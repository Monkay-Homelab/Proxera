package handlers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
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

type dnsExportProvider struct {
	Provider  string `json:"provider"`
	Domain    string `json:"domain"`
	ZoneID    string `json:"zone_id"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret,omitempty"`
}

type dnsExportPayload struct {
	Version   int                 `json:"version"`
	ExportedAt string            `json:"exported_at"`
	Providers []dnsExportProvider `json:"providers"`
}

type dnsExportFile struct {
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

// ExportDNSProviders handles POST /api/dns/export
func ExportDNSProviders(c *fiber.Ctx) error {
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

	// Fetch all accessible providers with encrypted credentials
	var query string
	var args []interface{}
	if role == "admin" {
		query = `SELECT provider, domain, zone_id, api_key, api_secret FROM dns_providers ORDER BY id`
	} else {
		query = `SELECT provider, domain, zone_id, api_key, api_secret FROM dns_providers
			WHERE user_id = $1 OR id IN (SELECT dns_provider_id FROM user_dns_providers WHERE user_id = $1)
			ORDER BY id`
		args = append(args, userID)
	}

	rows, err := database.DB.Query(context.Background(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch providers"})
	}
	defer rows.Close()

	var providers []dnsExportProvider
	for rows.Next() {
		var provider, encZoneID, encAPIKey string
		var domain, encAPISecret *string

		if err := rows.Scan(&provider, &domain, &encZoneID, &encAPIKey, &encAPISecret); err != nil {
			continue
		}

		ep := dnsExportProvider{Provider: provider}
		if domain != nil {
			ep.Domain = *domain
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No DNS providers to export"})
	}

	// Build plaintext payload
	payload := dnsExportPayload{
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Providers:  providers,
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
	hash := sha256.Sum256(ct)

	export := dnsExportFile{
		Version:    1,
		Salt:       hex.EncodeToString(salt),
		Ciphertext: hex.EncodeToString(ct),
		Checksum:   hex.EncodeToString(hash[:]),
	}

	return c.JSON(export)
}

// ImportDNSProviders handles POST /api/dns/import
func ImportDNSProviders(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var body struct {
		Password string        `json:"password"`
		Backup   dnsExportFile `json:"backup"`
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
	hash := sha256.Sum256(ct)
	if hex.EncodeToString(hash[:]) != body.Backup.Checksum {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Backup integrity check failed"})
	}

	// Decrypt with password
	plaintext, err := decryptWithPassword(ct, body.Password, salt)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	var payload dnsExportPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup data"})
	}

	if len(payload.Providers) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Backup contains no providers"})
	}

	// Import each provider — skip duplicates (same provider + domain)
	imported := 0
	skipped := 0
	for _, p := range payload.Providers {
		// Check for existing provider with same provider type and domain
		var exists bool
		err := database.DB.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM dns_providers WHERE provider = $1 AND domain = $2)`,
			p.Provider, p.Domain,
		).Scan(&exists)
		if err == nil && exists {
			skipped++
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

		_, err = database.DB.Exec(context.Background(),
			`INSERT INTO dns_providers (user_id, provider, zone_id, api_key, api_secret, domain, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
			userID, p.Provider, encZoneID, encAPIKey, encAPISecret, p.Domain,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("Failed to import provider %s: %v", p.Domain, err)})
		}
		imported++
	}

	return c.JSON(fiber.Map{
		"message":  fmt.Sprintf("Imported %d provider(s)", imported),
		"imported": imported,
		"skipped":  skipped,
	})
}
