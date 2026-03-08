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

	// Insert pending record immediately
	primaryDomain := req.Domains[0]
	san := ""
	if len(req.Domains) > 1 {
		san = strings.Join(req.Domains[1:], ",")
	}

	now := time.Now()
	var certID int
	err = database.DB.QueryRow(
		context.Background(),
		`INSERT INTO certificates (user_id, provider_id, domain, san, certificate_pem, private_key_pem, issuer_pem, cert_url, status, challenge_type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, '', '', '', '', 'pending', 'dns-01', $5, $5)
		 RETURNING id`,
		userID, req.ProviderID, primaryDomain, san, now,
	).Scan(&certID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create certificate record",
		})
	}

	// Return the pending record immediately
	resp := CertificateResponse{
		ID:            certID,
		ProviderID:    req.ProviderID,
		Domain:        primaryDomain,
		SAN:           san,
		Status:        "pending",
		ChallengeType: "dns-01",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Issue certificate in the background
	go issueCertInBackground(certID, userID, role, req)

	return c.Status(fiber.StatusCreated).JSON(resp)
}

// issueCertInBackground performs the ACME challenge and updates the DB record
func issueCertInBackground(certID, userID int, role string, req IssueCertificateRequest) {
	setError := func(msg string) {
		log.Printf("[ACME] Certificate %d failed: %s", certID, msg)
		database.DB.Exec(context.Background(),
			`UPDATE certificates SET status = 'error', updated_at = NOW() WHERE id = $1`,
			certID,
		)
	}

	providerType, creds, err := GetProviderCreds(req.ProviderID, userID, role)
	if err != nil {
		setError("Failed to get provider credentials")
		return
	}

	var email string
	err = database.DB.QueryRow(context.Background(),
		`SELECT email FROM users WHERE id = $1`, userID,
	).Scan(&email)
	if err != nil {
		setError("Failed to get user email")
		return
	}

	accountKey, err := acme.GetOrCreateAccountKey(userID)
	if err != nil {
		setError(fmt.Sprintf("Failed to get ACME account key: %s", err.Error()))
		return
	}

	log.Printf("[ACME] Issuing certificate %d for domains: %v (user %d)", certID, req.Domains, userID)

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
			setError(fmt.Sprintf("ACME challenge failed: %s", result.err.Error()))
			return
		}
		cert = result.cert
	case <-time.After(3 * time.Minute):
		setError("Certificate issuance timed out (3 minutes)")
		return
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
		setError("Failed to encrypt private key")
		return
	}

	now := time.Now()
	_, err = database.DB.Exec(context.Background(),
		`UPDATE certificates SET certificate_pem = $1, private_key_pem = $2, issuer_pem = $3,
		 cert_url = $4, status = 'active', issued_at = $5, expires_at = $6, updated_at = $5
		 WHERE id = $7`,
		string(cert.Certificate), encPrivateKey, string(cert.IssuerCertificate),
		cert.CertURL, now, expiresAt, certID,
	)
	if err != nil {
		log.Printf("[ACME] Certificate %d issued but failed to store: %v", certID, err)
		return
	}

	log.Printf("[ACME] Certificate issued and stored: id=%d domain=%s expires=%s", certID, req.Domains[0], expiresAt)
}

// RetryCertificate handles POST /api/certificates/:id/retry
func RetryCertificate(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	certID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid certificate ID"})
	}

	// Fetch the failed cert record
	var providerID int
	var domain, san, status string
	err = database.DB.QueryRow(context.Background(),
		`SELECT provider_id, domain, COALESCE(san, ''), status FROM certificates WHERE id = $1 AND user_id = $2`,
		certID, userID,
	).Scan(&providerID, &domain, &san, &status)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Certificate not found"})
	}

	if status != "error" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Only failed certificates can be retried"})
	}

	// Verify provider still exists
	exists, err := verifyProviderOwnership(providerID, userID, role)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	// Rebuild domain list from domain + SAN
	domains := []string{domain}
	if san != "" {
		domains = append(domains, strings.Split(san, ",")...)
	}

	// Reset to pending
	database.DB.Exec(context.Background(),
		`UPDATE certificates SET status = 'pending', updated_at = NOW() WHERE id = $1`,
		certID,
	)

	// Re-issue in background
	go issueCertInBackground(certID, userID, role, IssueCertificateRequest{
		ProviderID: providerID,
		Domains:    domains,
	})

	return c.JSON(fiber.Map{"status": "pending", "id": certID})
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
