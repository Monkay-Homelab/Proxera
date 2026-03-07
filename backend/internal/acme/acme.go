package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"

	proxeraCrypto "github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/database"
	dns "github.com/proxera/backend/internal/dns"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	legoCloudflare "github.com/go-acme/lego/v4/providers/dns/cloudflare"
	legoIONOS "github.com/go-acme/lego/v4/providers/dns/ionos"
	legoPorkbun "github.com/go-acme/lego/v4/providers/dns/porkbun"
	"github.com/go-acme/lego/v4/registration"
)

// acmeUser implements registration.User for lego
type acmeUser struct {
	Email        string
	Registration *registration.Resource
	Key          crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.Email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.Key }

// GetOrCreateAccountKey loads the ACME account key for a user, or generates a new one.
func GetOrCreateAccountKey(userID int) (crypto.PrivateKey, error) {
	ctx := context.Background()

	// Try to load existing key
	var encKey *string
	err := database.DB.QueryRow(ctx, `SELECT acme_key FROM users WHERE id = $1`, userID).Scan(&encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if encKey != nil && *encKey != "" {
		pemStr, err := proxeraCrypto.Decrypt(*encKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt ACME key: %w", err)
		}
		block, rest := pem.Decode([]byte(pemStr))
		if block == nil {
			return nil, fmt.Errorf("failed to decode ACME key PEM")
		}
		if len(rest) > 0 {
			log.Printf("[ACME] Warning: trailing data after PEM block (%d bytes)", len(rest))
		}
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ACME key: %w", err)
		}
		return key, nil
	}

	// Generate new ECDSA P-256 key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ACME key: %w", err)
	}

	// Encode to PEM
	derBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ACME key: %w", err)
	}
	pemBlock := &pem.Block{Type: "EC PRIVATE KEY", Bytes: derBytes}
	pemStr := string(pem.EncodeToMemory(pemBlock))

	// Encrypt and store
	encrypted, err := proxeraCrypto.Encrypt(pemStr)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt ACME key: %w", err)
	}

	_, err = database.DB.Exec(ctx, `UPDATE users SET acme_key = $1 WHERE id = $2`, encrypted, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to store ACME key: %w", err)
	}

	log.Printf("[ACME] Generated new account key for user %d", userID)
	return key, nil
}

// IssueCertificate obtains a certificate from Let's Encrypt using DNS-01 via the given provider.
func IssueCertificate(email string, accountKey crypto.PrivateKey, domains []string, providerType string, creds dns.Credentials) (*certificate.Resource, error) {
	user := &acmeUser{
		Email: email,
		Key:   accountKey,
	}

	config := lego.NewConfig(user)

	// Use staging if env var is set
	if os.Getenv("ACME_STAGING") == "true" {
		config.CADirURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
		log.Println("[ACME] Using staging environment")
	}

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Configure the DNS-01 provider based on the provider type
	log.Printf("[ACME] Configuring DNS-01 challenge via %s", providerType)
	switch providerType {
	case "cloudflare":
		cfg := legoCloudflare.NewDefaultConfig()
		cfg.AuthToken = creds.APIKey
		p, err := legoCloudflare.NewDNSProviderConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create Cloudflare DNS provider: %w", err)
		}
		if err := client.Challenge.SetDNS01Provider(p); err != nil {
			return nil, fmt.Errorf("failed to set DNS-01 provider: %w", err)
		}

	case "porkbun":
		cfg := legoPorkbun.NewDefaultConfig()
		cfg.APIKey = creds.APIKey
		cfg.SecretAPIKey = creds.APISecret
		p, err := legoPorkbun.NewDNSProviderConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create Porkbun DNS provider: %w", err)
		}
		if err := client.Challenge.SetDNS01Provider(p); err != nil {
			return nil, fmt.Errorf("failed to set DNS-01 provider: %w", err)
		}

	case "ionos":
		cfg := legoIONOS.NewDefaultConfig()
		cfg.APIKey = creds.APIKey
		p, err := legoIONOS.NewDNSProviderConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create IONOS DNS provider: %w", err)
		}
		if err := client.Challenge.SetDNS01Provider(p); err != nil {
			return nil, fmt.Errorf("failed to set DNS-01 provider: %w", err)
		}

	default:
		return nil, fmt.Errorf("DNS-01 certificate issuance is not supported for provider %q", providerType)
	}

	// Register account (or retrieve existing)
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("failed to register ACME account: %w", err)
	}
	user.Registration = reg

	// Request certificate
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	cert, err := client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain certificate: %w", err)
	}

	return cert, nil
}
