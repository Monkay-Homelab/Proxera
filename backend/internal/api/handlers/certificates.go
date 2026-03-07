package handlers

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/acme"
	"github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/database"
)

type CertificateResponse struct {
	ID             int        `json:"id"`
	ProviderID     int        `json:"provider_id"`
	Domain         string     `json:"domain"`
	SAN            string     `json:"san"`
	CertificatePEM string     `json:"certificate_pem,omitempty"`
	PrivateKeyPEM  string     `json:"private_key_pem,omitempty"`
	IssuerPEM      string     `json:"issuer_pem,omitempty"`
	CertURL        string     `json:"cert_url"`
	Status         string     `json:"status"`
	ChallengeType  string     `json:"challenge_type"`
	IssuedAt       *time.Time `json:"issued_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type IssueCertificateRequest struct {
	ProviderID int      `json:"provider_id"`
	Domains    []string `json:"domains"`
}

// computeStatus returns the display status based on expiry
func computeStatus(dbStatus string, expiresAt *time.Time) string {
	if dbStatus == "error" || dbStatus == "pending" {
		return dbStatus
	}
	if expiresAt == nil {
		return dbStatus
	}
	now := time.Now()
	if expiresAt.Before(now) {
		return "expired"
	}
	if expiresAt.Before(now.Add(CertExpiryWarning)) {
		return "expiring"
	}
	return "active"
}

func ListCertificates(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	rows, err := database.DB.Query(
		context.Background(),
		`SELECT id, provider_id, domain, COALESCE(san, ''), cert_url, status, challenge_type,
		        issued_at, expires_at, created_at, updated_at
		 FROM certificates WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch certificates",
		})
	}
	defer rows.Close()

	certs := []CertificateResponse{}
	for rows.Next() {
		var cert CertificateResponse
		var dbStatus string
		var certURL *string
		if err := rows.Scan(
			&cert.ID, &cert.ProviderID, &cert.Domain, &cert.SAN, &certURL,
			&dbStatus, &cert.ChallengeType, &cert.IssuedAt, &cert.ExpiresAt,
			&cert.CreatedAt, &cert.UpdatedAt,
		); err != nil {
			continue
		}
		if certURL != nil {
			cert.CertURL = *certURL
		}
		cert.Status = computeStatus(dbStatus, cert.ExpiresAt)
		certs = append(certs, cert)
	}
	if err := rows.Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error reading certificates",
		})
	}

	return c.JSON(certs)
}

func IssueCertificate(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	var req IssueCertificateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.ProviderID == 0 || len(req.Domains) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "provider_id and domains are required",
		})
	}

	// Verify provider ownership
	exists, err := verifyProviderOwnership(req.ProviderID, userID, role)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "DNS provider not found",
		})
	}

	// Get provider type and credentials for DNS-01 challenge
	providerType, creds, err := GetProviderCreds(req.ProviderID, userID, role)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get provider credentials",
		})
	}

	// Get user email for ACME registration
	var email string
	err = database.DB.QueryRow(
		context.Background(),
		`SELECT email FROM users WHERE id = $1`,
		userID,
	).Scan(&email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get user email",
		})
	}

	// Get or create ACME account key
	accountKey, err := acme.GetOrCreateAccountKey(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get ACME account key: %s", err.Error()),
		})
	}

	// Issue certificate with 3-minute timeout
	log.Printf("[ACME] Issuing certificate for domains: %v (user %d)", req.Domains, userID)

	type acmeResult struct {
		cert *certificate.Resource
		err  error
	}
	resultCh := make(chan acmeResult, 1)
	go func() {
		c, e := acme.IssueCertificate(email, accountKey, req.Domains, providerType, creds)
		resultCh <- acmeResult{cert: c, err: e}
	}()

	var cert *certificate.Resource
	select {
	case result := <-resultCh:
		if result.err != nil {
			log.Printf("[ACME] Failed to issue certificate: %v", result.err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to issue certificate: %s", result.err.Error()),
			})
		}
		cert = result.cert
	case <-time.After(3 * time.Minute):
		log.Printf("[ACME] Certificate issuance timed out for domains: %v", req.Domains)
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
			"error": "Certificate issuance timed out (3 minutes). The ACME provider may be slow — try again later.",
		})
	}

	// Parse certificate to get expiry date
	var expiresAt time.Time
	block, _ := pem.Decode(cert.Certificate)
	if block != nil {
		parsed, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			expiresAt = parsed.NotAfter
		}
	}

	// Encrypt private key before storing
	encPrivateKey, err := crypto.Encrypt(string(cert.PrivateKey))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to encrypt private key",
		})
	}

	// Store in database
	primaryDomain := req.Domains[0]
	san := ""
	if len(req.Domains) > 1 {
		san = strings.Join(req.Domains[1:], ",")
	}

	now := time.Now()
	var certID int
	err = database.DB.QueryRow(
		context.Background(),
		`INSERT INTO certificates (user_id, provider_id, domain, san, certificate_pem, private_key_pem, issuer_pem, cert_url, status, issued_at, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10, $11, $11)
		 RETURNING id`,
		userID, req.ProviderID, primaryDomain, san,
		string(cert.Certificate), encPrivateKey, string(cert.IssuerCertificate),
		cert.CertURL, now, expiresAt, now,
	).Scan(&certID)
	if err != nil {
		log.Printf("[ACME] Failed to store certificate: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Certificate issued but failed to store",
		})
	}

	log.Printf("[ACME] Certificate issued and stored: id=%d domain=%s expires=%s", certID, primaryDomain, expiresAt)

	return c.Status(fiber.StatusCreated).JSON(CertificateResponse{
		ID:            certID,
		ProviderID:    req.ProviderID,
		Domain:        primaryDomain,
		SAN:           san,
		CertURL:       cert.CertURL,
		Status:        "active",
		ChallengeType: "dns-01",
		IssuedAt:      &now,
		ExpiresAt:     &expiresAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

func GetCertificate(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	certID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid certificate ID",
		})
	}

	var cert CertificateResponse
	var dbStatus string
	var encPrivateKey *string
	var certURL *string
	var certPEM, issuerPEM *string

	err = database.DB.QueryRow(
		context.Background(),
		`SELECT id, provider_id, domain, COALESCE(san, ''), certificate_pem, private_key_pem, issuer_pem,
		        cert_url, status, challenge_type, issued_at, expires_at, created_at, updated_at
		 FROM certificates WHERE id = $1 AND user_id = $2`,
		certID, userID,
	).Scan(
		&cert.ID, &cert.ProviderID, &cert.Domain, &cert.SAN,
		&certPEM, &encPrivateKey, &issuerPEM, &certURL,
		&dbStatus, &cert.ChallengeType, &cert.IssuedAt, &cert.ExpiresAt,
		&cert.CreatedAt, &cert.UpdatedAt,
	)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Certificate not found",
		})
	}

	if certPEM != nil {
		cert.CertificatePEM = *certPEM
	}
	if issuerPEM != nil {
		cert.IssuerPEM = *issuerPEM
	}
	if certURL != nil {
		cert.CertURL = *certURL
	}

	// Only return private key when explicitly requested
	if c.Query("include_key") == "true" {
		if encPrivateKey != nil && *encPrivateKey != "" {
			decrypted, err := crypto.Decrypt(*encPrivateKey)
			if err == nil {
				cert.PrivateKeyPEM = decrypted
			}
		}
	}

	cert.Status = computeStatus(dbStatus, cert.ExpiresAt)

	return c.JSON(cert)
}

func DeleteCertificate(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	certID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid certificate ID",
		})
	}

	result, err := database.DB.Exec(
		context.Background(),
		`DELETE FROM certificates WHERE id = $1 AND user_id = $2`,
		certID, userID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete certificate",
		})
	}

	if result.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Certificate not found",
		})
	}

	return c.JSON(fiber.Map{"message": "Certificate deleted"})
}
