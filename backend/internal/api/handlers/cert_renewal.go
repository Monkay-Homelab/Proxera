package handlers

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/proxera/backend/internal/acme"
	"github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/database"
	dnspkg "github.com/proxera/backend/internal/dns"
	"github.com/proxera/backend/internal/notifications"
)

type expiringCert struct {
	ID           int
	UserID       int
	ProviderID   int
	Domain       string
	SAN          string
	Email        string
	ProviderName string
	APIKey       string // encrypted
	APISecret    string // encrypted (Porkbun / GoDaddy)
	ZoneID       string // encrypted
}

// StartCertRenewalJob runs certificate renewal checks on startup and every 24 hours.
func StartCertRenewalJob() {
	renewExpiring()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		renewExpiring()
	}
}

func renewExpiring() {
	certs, err := findExpiringCerts()
	if err != nil {
		slog.Error("failed to find expiring certs", "component", "cert-renewal", "error", err)
		return
	}

	if len(certs) == 0 {
		slog.Info("no certificates need renewal", "component", "cert-renewal")
		return
	}

	slog.Info("found certificates to renew", "component", "cert-renewal", "count", len(certs))

	for _, cert := range certs {
		if err := renewCert(cert); err != nil {
			slog.Error("failed to renew cert", "component", "cert-renewal", "cert_id", cert.ID, "domain", cert.Domain, "error", err)
			markCertError(cert.ID, err.Error())

			// Trigger cert renewal failure alert
			var expiresAt time.Time
			_ = database.DB.QueryRow(context.Background(),
				`SELECT COALESCE(expires_at, '0001-01-01') FROM certificates WHERE id = $1`, cert.ID,
			).Scan(&expiresAt)
			go triggerCertRenewalFailedAlert(cert.UserID, cert.ID, cert.Domain, expiresAt, err.Error())

			continue
		}
		slog.Info("successfully renewed cert", "component", "cert-renewal", "cert_id", cert.ID, "domain", cert.Domain)

		// Resolve any open cert alerts for this domain
		go func(c expiringCert) {
			ctx := context.Background()
			notifications.Resolve(ctx, c.UserID, "cert_expiry", "domain", c.Domain)
			notifications.Resolve(ctx, c.UserID, "cert_renewal_failed", "domain", c.Domain)
		}(cert)

		redeployHostsForCert(cert)
	}
}

func findExpiringCerts() ([]expiringCert, error) {
	rows, err := database.DB.Query(context.Background(), `
		SELECT c.id, c.user_id, c.provider_id, c.domain, COALESCE(c.san, ''),
		       u.email,
		       dp.provider, dp.api_key, COALESCE(dp.api_secret, ''), COALESCE(dp.zone_id, '')
		FROM certificates c
		JOIN users u ON u.id = c.user_id
		JOIN dns_providers dp ON dp.id = c.provider_id
		WHERE c.status != 'error'
		  AND c.expires_at IS NOT NULL
		  AND c.expires_at <= NOW() + INTERVAL '30 days'
		  AND c.certificate_pem IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []expiringCert
	for rows.Next() {
		var c expiringCert
		if err := rows.Scan(&c.ID, &c.UserID, &c.ProviderID, &c.Domain, &c.SAN,
			&c.Email, &c.ProviderName, &c.APIKey, &c.APISecret, &c.ZoneID); err != nil {
			slog.Error("failed to scan cert row", "component", "cert-renewal", "error", err)
			continue
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

func renewCert(cert expiringCert) error {
	accountKey, err := acme.GetOrCreateAccountKey(cert.UserID)
	if err != nil {
		return err
	}

	apiKey, err := crypto.Decrypt(cert.APIKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt API key: %w", err)
	}
	apiSecret := ""
	if cert.APISecret != "" {
		apiSecret, err = crypto.Decrypt(cert.APISecret)
		if err != nil {
			return fmt.Errorf("failed to decrypt API secret: %w", err)
		}
	}
	zoneID := ""
	if cert.ZoneID != "" {
		zoneID, err = crypto.Decrypt(cert.ZoneID)
		if err != nil {
			return fmt.Errorf("failed to decrypt zone ID: %w", err)
		}
	}

	creds := dnspkg.Credentials{
		APIKey:    apiKey,
		APISecret: apiSecret,
		ZoneID:    zoneID,
	}

	// Build domain list: primary + SANs
	domains := []string{cert.Domain}
	if cert.SAN != "" {
		for _, san := range strings.Split(cert.SAN, ",") {
			san = strings.TrimSpace(san)
			if san != "" {
				domains = append(domains, san)
			}
		}
	}

	// Issue with 3-minute timeout
	type acmeResult struct {
		cert *certificate.Resource
		err  error
	}
	resultCh := make(chan acmeResult, 1)
	go func() {
		c, e := acme.IssueCertificate(cert.Email, accountKey, domains, cert.ProviderName, creds)
		resultCh <- acmeResult{cert: c, err: e}
	}()

	var newCert *certificate.Resource
	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}
		newCert = result.cert
	case <-time.After(3 * time.Minute):
		return errCertTimeout
	}

	// Parse expiry from new certificate
	var expiresAt time.Time
	block, _ := pem.Decode(newCert.Certificate)
	if block != nil {
		parsed, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			expiresAt = parsed.NotAfter
		}
	}

	// Encrypt new private key
	encPrivateKey, err := crypto.Encrypt(string(newCert.PrivateKey))
	if err != nil {
		return err
	}

	// Update certificate in database
	now := time.Now()
	_, err = database.DB.Exec(context.Background(), `
		UPDATE certificates
		SET certificate_pem = $1,
		    private_key_pem = $2,
		    issuer_pem = $3,
		    cert_url = $4,
		    status = 'active',
		    issued_at = $5,
		    expires_at = $6,
		    updated_at = $7
		WHERE id = $8`,
		string(newCert.Certificate), encPrivateKey, string(newCert.IssuerCertificate),
		newCert.CertURL, now, expiresAt, now, cert.ID,
	)
	return err
}

var errCertTimeout = &certTimeoutError{}

type certTimeoutError struct{}

func (e *certTimeoutError) Error() string {
	return "certificate issuance timed out (3 minutes)"
}

func redeployHostsForCert(cert expiringCert) {
	rows, err := database.DB.Query(context.Background(), `
		SELECT h.domain, h.upstream_url, h.ssl, h.websocket, h.certificate_id,
		       h.config, h.agent_id
		FROM hosts h
		WHERE h.certificate_id = $1
		  AND h.agent_id IS NOT NULL`, cert.ID)
	if err != nil {
		slog.Error("failed to query hosts for cert", "component", "cert-renewal", "cert_id", cert.ID, "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var domain, upstreamURL string
		var ssl, ws bool
		var certID *int
		var configBytes []byte
		var agentDBID int
		if err := rows.Scan(&domain, &upstreamURL, &ssl, &ws, &certID, &configBytes, &agentDBID); err != nil {
			slog.Error("failed to scan host row", "component", "cert-renewal", "error", err)
			continue
		}

		var config *HostAdvancedConfig
		if len(configBytes) > 0 && string(configBytes) != "{}" {
			config = &HostAdvancedConfig{}
			if err := json.Unmarshal(configBytes, config); err != nil {
				config = nil
			}
		}

		if err := deployHostToAgent(cert.UserID, agentDBID, domain, upstreamURL, ssl, ws, certID, config); err != nil {
			slog.Error("failed to redeploy host after cert renewal", "component", "cert-renewal", "domain", domain, "error", err)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating hosts for cert", "component", "cert-renewal", "cert_id", cert.ID, "error", err)
	}
}

func markCertError(certID int, reason string) {
	if _, err := database.DB.Exec(context.Background(),
		`UPDATE certificates SET status = 'error', updated_at = NOW() WHERE id = $1`,
		certID); err != nil {
		slog.Error("failed to mark cert as error", "component", "cert-renewal", "cert_id", certID, "error", err)
	}
}
