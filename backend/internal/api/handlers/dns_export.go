package handlers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/database"
	"golang.org/x/crypto/argon2"
)

// --- Export types ---

type exportProvider struct {
	Provider  string `json:"provider"`
	Domain    string `json:"domain"`
	ZoneID    string `json:"zone_id"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret,omitempty"`
}

type exportHost struct {
	Domain        string              `json:"domain"`
	ProviderDomain string            `json:"provider_domain"`
	UpstreamURL   string              `json:"upstream_url"`
	SSL           bool                `json:"ssl"`
	WebSocket     bool                `json:"websocket"`
	Config        *HostAdvancedConfig `json:"config,omitempty"`
}

type exportCertificate struct {
	Domain         string  `json:"domain"`
	SAN            string  `json:"san,omitempty"`
	ProviderDomain string  `json:"provider_domain"`
	CertificatePEM string  `json:"certificate_pem"`
	PrivateKeyPEM  string  `json:"private_key_pem"`
	IssuerPEM      string  `json:"issuer_pem,omitempty"`
	CertURL        string  `json:"cert_url,omitempty"`
	ChallengeType  string  `json:"challenge_type"`
	IssuedAt       *string `json:"issued_at,omitempty"`
	ExpiresAt      *string `json:"expires_at,omitempty"`
}

type exportPayload struct {
	Version      int                 `json:"version"`
	ExportedAt   string              `json:"exported_at"`
	Providers    []exportProvider    `json:"providers"`
	Hosts        []exportHost        `json:"hosts,omitempty"`
	Certificates []exportCertificate `json:"certificates,omitempty"`
}

type exportFile struct {
	Version    int    `json:"version"`
	Salt       string `json:"salt"`
	Ciphertext string `json:"ciphertext"`
	Checksum   string `json:"checksum"`
}

// --- Password-based encryption helpers ---

func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
}

func encryptWithPassword(plaintext []byte, password string) (salt, ciphertext []byte, err error) {
	salt = make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = gcm.Seal(nonce, nonce, plaintext, nil)
	return salt, ciphertext, nil
}

func decryptWithPassword(ciphertext []byte, password string, salt []byte) ([]byte, error) {
	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("wrong password or corrupted backup")
	}

	return plaintext, nil
}

// ExportBackup handles POST /api/dns/export
func ExportBackup(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	var body struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil || body.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Password is required"})
	}
	if len(body.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Password must be at least 8 characters"})
	}

	// --- Fetch providers ---
	var providerQuery string
	var providerArgs []interface{}
	if role == "admin" {
		providerQuery = `SELECT id, provider, domain, zone_id, api_key, api_secret FROM dns_providers ORDER BY id`
	} else {
		providerQuery = `SELECT id, provider, domain, zone_id, api_key, api_secret FROM dns_providers
			WHERE user_id = $1 OR id IN (SELECT dns_provider_id FROM user_dns_providers WHERE user_id = $1)
			ORDER BY id`
		providerArgs = append(providerArgs, userID)
	}

	rows, err := database.DB.Query(context.Background(), providerQuery, providerArgs...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch providers"})
	}
	defer rows.Close()

	var providers []exportProvider
	providerIDToDomain := map[int]string{} // map provider DB id → domain for host/cert linking
	for rows.Next() {
		var id int
		var provider, encZoneID, encAPIKey string
		var domain, encAPISecret *string

		if err := rows.Scan(&id, &provider, &domain, &encZoneID, &encAPIKey, &encAPISecret); err != nil {
			continue
		}

		ep := exportProvider{Provider: provider}
		if domain != nil {
			ep.Domain = *domain
			providerIDToDomain[id] = *domain
		}
		if v, err := crypto.Decrypt(encZoneID); err == nil {
			ep.ZoneID = v
		}
		if v, err := crypto.Decrypt(encAPIKey); err == nil {
			ep.APIKey = v
		}
		if encAPISecret != nil && *encAPISecret != "" {
			if v, err := crypto.Decrypt(*encAPISecret); err == nil {
				ep.APISecret = v
			}
		}

		providers = append(providers, ep)
	}

	if len(providers) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No data to export"})
	}

	// --- Fetch hosts ---
	var hostQuery string
	var hostArgs []interface{}
	if role == "admin" {
		hostQuery = `SELECT h.domain, h.upstream_url, h.ssl, h.websocket, h.config, h.provider_id
			FROM hosts h ORDER BY h.id`
	} else {
		hostQuery = `SELECT h.domain, h.upstream_url, h.ssl, h.websocket, h.config, h.provider_id
			FROM hosts h WHERE h.user_id = $1 ORDER BY h.id`
		hostArgs = append(hostArgs, userID)
	}

	hostRows, err := database.DB.Query(context.Background(), hostQuery, hostArgs...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch hosts"})
	}
	defer hostRows.Close()

	var hosts []exportHost
	for hostRows.Next() {
		var domain, upstreamURL string
		var ssl, websocket bool
		var configBytes []byte
		var providerID int

		if err := hostRows.Scan(&domain, &upstreamURL, &ssl, &websocket, &configBytes, &providerID); err != nil {
			continue
		}

		eh := exportHost{
			Domain:         domain,
			ProviderDomain: providerIDToDomain[providerID],
			UpstreamURL:    upstreamURL,
			SSL:            ssl,
			WebSocket:      websocket,
			Config:         scanHostConfig(configBytes),
		}
		hosts = append(hosts, eh)
	}

	// --- Fetch certificates ---
	var certQuery string
	var certArgs []interface{}
	if role == "admin" {
		certQuery = `SELECT c.domain, COALESCE(c.san, ''), c.certificate_pem, c.private_key_pem, COALESCE(c.issuer_pem, ''),
			COALESCE(c.cert_url, ''), c.challenge_type, c.issued_at, c.expires_at, c.provider_id
			FROM certificates c WHERE c.status = 'active' ORDER BY c.id`
	} else {
		certQuery = `SELECT c.domain, COALESCE(c.san, ''), c.certificate_pem, c.private_key_pem, COALESCE(c.issuer_pem, ''),
			COALESCE(c.cert_url, ''), c.challenge_type, c.issued_at, c.expires_at, c.provider_id
			FROM certificates c WHERE c.user_id = $1 AND c.status = 'active' ORDER BY c.id`
		certArgs = append(certArgs, userID)
	}

	certRows, err := database.DB.Query(context.Background(), certQuery, certArgs...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch certificates"})
	}
	defer certRows.Close()

	var certs []exportCertificate
	for certRows.Next() {
		var domain, san, certPEM, encPrivateKey, issuerPEM, certURL, challengeType string
		var issuedAt, expiresAt *time.Time
		var providerID int

		if err := certRows.Scan(&domain, &san, &certPEM, &encPrivateKey, &issuerPEM,
			&certURL, &challengeType, &issuedAt, &expiresAt, &providerID); err != nil {
			continue
		}

		// Decrypt private key
		privateKey, err := crypto.Decrypt(encPrivateKey)
		if err != nil {
			continue
		}

		ec := exportCertificate{
			Domain:         domain,
			SAN:            san,
			ProviderDomain: providerIDToDomain[providerID],
			CertificatePEM: certPEM,
			PrivateKeyPEM:  privateKey,
			IssuerPEM:      issuerPEM,
			CertURL:        certURL,
			ChallengeType:  challengeType,
		}
		if issuedAt != nil {
			s := issuedAt.UTC().Format(time.RFC3339)
			ec.IssuedAt = &s
		}
		if expiresAt != nil {
			s := expiresAt.UTC().Format(time.RFC3339)
			ec.ExpiresAt = &s
		}
		certs = append(certs, ec)
	}

	// Build plaintext payload
	payload := exportPayload{
		Version:      2,
		ExportedAt:   time.Now().UTC().Format(time.RFC3339),
		Providers:    providers,
		Hosts:        hosts,
		Certificates: certs,
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to serialize export"})
	}

	// Encrypt with user password
	salt, ct, err := encryptWithPassword(plaintext, body.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Encryption failed"})
	}

	// Checksum of ciphertext for integrity verification
	hash := sha512.Sum512(ct)

	export := exportFile{
		Version:    2,
		Salt:       hex.EncodeToString(salt),
		Ciphertext: hex.EncodeToString(ct),
		Checksum:   hex.EncodeToString(hash[:]),
	}

	return c.JSON(export)
}

// ImportBackup handles POST /api/dns/import
func ImportBackup(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var body struct {
		Password string     `json:"password"`
		Backup   exportFile `json:"backup"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if body.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Password is required"})
	}
	if body.Backup.Ciphertext == "" || body.Backup.Salt == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup file"})
	}

	// Decode hex fields
	salt, err := hex.DecodeString(body.Backup.Salt)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup: bad salt"})
	}
	ct, err := hex.DecodeString(body.Backup.Ciphertext)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup: bad ciphertext"})
	}

	// Verify checksum
	hash := sha512.Sum512(ct)
	if hex.EncodeToString(hash[:]) != body.Backup.Checksum {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Backup integrity check failed"})
	}

	// Decrypt with password
	plaintext, err := decryptWithPassword(ct, body.Password, salt)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	// Support both v1 (providers-only) and v2 (full backup)
	var payload exportPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup data"})
	}

	if len(payload.Providers) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Backup contains no providers"})
	}

	result := fiber.Map{}

	// --- Import providers ---
	providersImported := 0
	providersSkipped := 0
	// Track provider domain → new DB id for host/cert linking
	providerDomainToID := map[string]int{}

	for _, p := range payload.Providers {
		// Check for existing provider with same provider type and domain
		var existingID int
		err := database.DB.QueryRow(context.Background(),
			`SELECT id FROM dns_providers WHERE provider = $1 AND domain = $2`,
			p.Provider, p.Domain,
		).Scan(&existingID)
		if err == nil {
			providerDomainToID[p.Domain] = existingID
			providersSkipped++
			continue
		}

		// Re-encrypt credentials with system encryption key
		encZoneID, err := crypto.Encrypt(p.ZoneID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("Failed to encrypt credentials for %s", p.Domain)})
		}
		encAPIKey, err := crypto.Encrypt(p.APIKey)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("Failed to encrypt credentials for %s", p.Domain)})
		}

		var encAPISecret *string
		if p.APISecret != "" {
			v, err := crypto.Encrypt(p.APISecret)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("Failed to encrypt credentials for %s", p.Domain)})
			}
			encAPISecret = &v
		}

		var newID int
		err = database.DB.QueryRow(context.Background(),
			`INSERT INTO dns_providers (user_id, provider, zone_id, api_key, api_secret, domain, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW()) RETURNING id`,
			userID, p.Provider, encZoneID, encAPIKey, encAPISecret, p.Domain,
		).Scan(&newID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("Failed to import provider %s: %v", p.Domain, err)})
		}
		providerDomainToID[p.Domain] = newID
		providersImported++
	}
	result["providers_imported"] = providersImported
	result["providers_skipped"] = providersSkipped

	// --- Import hosts ---
	hostsImported := 0
	hostsSkipped := 0
	for _, h := range payload.Hosts {
		providerID, ok := providerDomainToID[h.ProviderDomain]
		if !ok {
			hostsSkipped++
			continue
		}

		// Skip duplicate hosts (same domain under same provider)
		var exists bool
		err := database.DB.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM hosts WHERE domain = $1 AND provider_id = $2)`,
			h.Domain, providerID,
		).Scan(&exists)
		if err == nil && exists {
			hostsSkipped++
			continue
		}

		configJSON, _ := json.Marshal(h.Config)
		if h.Config == nil {
			configJSON = []byte("{}")
		}

		_, err = database.DB.Exec(context.Background(),
			`INSERT INTO hosts (user_id, provider_id, domain, upstream_url, ssl, websocket, config, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`,
			userID, providerID, h.Domain, h.UpstreamURL, h.SSL, h.WebSocket, configJSON,
		)
		if err != nil {
			hostsSkipped++
			continue
		}
		hostsImported++
	}
	result["hosts_imported"] = hostsImported
	result["hosts_skipped"] = hostsSkipped

	// --- Import certificates ---
	certsImported := 0
	certsSkipped := 0
	for _, cert := range payload.Certificates {
		providerID, ok := providerDomainToID[cert.ProviderDomain]
		if !ok {
			certsSkipped++
			continue
		}

		// Skip duplicate certs (same domain under same provider)
		var exists bool
		err := database.DB.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM certificates WHERE domain = $1 AND provider_id = $2 AND status = 'active')`,
			cert.Domain, providerID,
		).Scan(&exists)
		if err == nil && exists {
			certsSkipped++
			continue
		}

		// Re-encrypt private key with system key
		encPrivateKey, err := crypto.Encrypt(cert.PrivateKeyPEM)
		if err != nil {
			certsSkipped++
			continue
		}

		var issuedAt, expiresAt *time.Time
		if cert.IssuedAt != nil {
			if t, err := time.Parse(time.RFC3339, *cert.IssuedAt); err == nil {
				issuedAt = &t
			}
		}
		if cert.ExpiresAt != nil {
			if t, err := time.Parse(time.RFC3339, *cert.ExpiresAt); err == nil {
				expiresAt = &t
			}
		}

		_, err = database.DB.Exec(context.Background(),
			`INSERT INTO certificates (user_id, provider_id, domain, san, certificate_pem, private_key_pem, issuer_pem, cert_url, status, challenge_type, issued_at, expires_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10, $11, NOW(), NOW())`,
			userID, providerID, cert.Domain, cert.SAN, cert.CertificatePEM, encPrivateKey,
			cert.IssuerPEM, cert.CertURL, cert.ChallengeType, issuedAt, expiresAt,
		)
		if err != nil {
			certsSkipped++
			continue
		}
		certsImported++
	}
	result["certs_imported"] = certsImported
	result["certs_skipped"] = certsSkipped

	total := providersImported + hostsImported + certsImported
	result["message"] = fmt.Sprintf("Imported %d item(s)", total)

	return c.JSON(result)
}
