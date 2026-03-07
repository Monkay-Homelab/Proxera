package deploy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/proxera/agent/pkg/nginx"
	"github.com/proxera/agent/pkg/types"
	"golang.org/x/crypto/bcrypt"
)

// Deployer handles deploying host configurations to nginx
type Deployer struct {
	manager     *nginx.Manager
	configPath  string
	enabledPath string
}

// EnabledPath returns the nginx sites-enabled path
func (d *Deployer) EnabledPath() string {
	return d.enabledPath
}

// NewDeployer creates a new Deployer
func NewDeployer(manager *nginx.Manager, configPath, enabledPath string) *Deployer {
	return &Deployer{
		manager:     manager,
		configPath:  configPath,
		enabledPath: enabledPath,
	}
}

// ensureHTPasswd creates or removes htpasswd file based on config
func (d *Deployer) ensureHTPasswd(host types.Host) error {
	safeDomain := nginx.SanitizeDomain(host.Domain)
	htpasswdPath := fmt.Sprintf("/etc/nginx/.htpasswd_%s", safeDomain)

	// Determine basic auth credentials from config or legacy field
	var username, password string
	if host.Config != nil && host.Config.BasicAuth != nil && host.Config.BasicAuth.Username != "" {
		username = host.Config.BasicAuth.Username
		password = host.Config.BasicAuth.Password
	} else if host.BasicAuth != nil && host.BasicAuth.Username != "" {
		username = host.BasicAuth.Username
		password = host.BasicAuth.Password
	}

	if username == "" {
		// No basic auth - remove htpasswd file if exists
		os.Remove(htpasswdPath)
		return nil
	}

	// Generate bcrypt hash
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	content := fmt.Sprintf("%s:%s\n", username, string(hash))
	if err := os.WriteFile(htpasswdPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write htpasswd file: %w", err)
	}

	return nil
}

// removeHTPasswd removes the htpasswd file for a domain
func (d *Deployer) removeHTPasswd(domain string) {
	safeDomain := nginx.SanitizeDomain(domain)
	htpasswdPath := fmt.Sprintf("/etc/nginx/.htpasswd_%s", safeDomain)
	os.Remove(htpasswdPath)
}

// validateSSLCerts validates PEM format, key-cert matching, and expiry of SSL certificates.
// Returns an error if validation fails. Logs warnings for non-fatal issues (e.g., near expiry).
func validateSSLCerts(certPEM, keyPEM, issuerPEM, domain string) error {
	// Validate cert PEM format
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return fmt.Errorf("invalid certificate PEM: failed to decode PEM block")
	}

	// Validate key PEM format
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return fmt.Errorf("invalid private key PEM: failed to decode PEM block")
	}

	// Validate issuer PEM format if provided
	if issuerPEM != "" {
		issuerBlock, _ := pem.Decode([]byte(issuerPEM))
		if issuerBlock == nil {
			return fmt.Errorf("invalid issuer PEM: failed to decode PEM block")
		}
	}

	// Build fullchain for key-pair validation
	fullchain := certPEM
	if issuerPEM != "" {
		fullchain = certPEM + "\n" + issuerPEM
	}

	// Validate key-cert matching via tls.X509KeyPair
	_, err := tls.X509KeyPair([]byte(fullchain), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("certificate and private key do not match: %w", err)
	}

	// Parse certificate to check expiry and domain SANs
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check expiry
	now := time.Now()
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate expired on %s", cert.NotAfter.Format(time.RFC3339))
	}
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate not yet valid until %s", cert.NotBefore.Format(time.RFC3339))
	}

	// Warn if expiring within 7 days
	if time.Until(cert.NotAfter) < 7*24*time.Hour {
		log.Printf("Warning: certificate for %s expires in %s", domain, time.Until(cert.NotAfter).Round(time.Hour))
	}

	// Validate domain SAN matching
	if domain != "" {
		if err := cert.VerifyHostname(domain); err != nil {
			log.Printf("Warning: certificate does not match domain %s: %v", domain, err)
		}
	}

	return nil
}

// ApplyHost deploys a single host configuration
func (d *Deployer) ApplyHost(host types.Host) error {
	// Ensure Cloudflare real IP config exists
	if err := d.EnsureCloudflareRealIP(); err != nil {
		log.Printf("Warning: failed to ensure Cloudflare real IP config: %v", err)
	}

	// Ensure metrics log format exists
	if err := d.EnsureMetricsLogFormat(); err != nil {
		log.Printf("Warning: failed to ensure metrics log format: %v", err)
	}

	// Ensure log rotation is configured
	if err := d.EnsureLogRotation(); err != nil {
		log.Printf("Warning: failed to ensure log rotation: %v", err)
	}

	// Ensure ACME challenge directory exists for SSL hosts
	if host.SSL {
		os.MkdirAll("/var/www/proxera-acme/.well-known/acme-challenge", 0755)
	}

	// Write SSL certs if provided
	if host.SSL && host.CertPEM != "" && host.KeyPEM != "" {
		// Validate PEM format, key-cert matching, and expiry before writing
		if err := validateSSLCerts(host.CertPEM, host.KeyPEM, host.IssuerPEM, host.Domain); err != nil {
			return fmt.Errorf("SSL certificate validation failed for %s: %w", host.Domain, err)
		}

		certDir := d.sslDir(host.Domain)
		if err := os.MkdirAll(certDir, 0700); err != nil {
			return fmt.Errorf("failed to create SSL directory: %w", err)
		}

		fullchain := host.CertPEM
		if host.IssuerPEM != "" {
			fullchain = host.CertPEM + "\n" + host.IssuerPEM
		}

		if err := os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte(fullchain), 0600); err != nil {
			return fmt.Errorf("failed to write fullchain.pem: %w", err)
		}
		if err := os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte(host.KeyPEM), 0600); err != nil {
			return fmt.Errorf("failed to write privkey.pem: %w", err)
		}

		// Set cert paths on the host for config generation
		host.CertPath = filepath.Join(certDir, "fullchain.pem")
		host.KeyPath = filepath.Join(certDir, "privkey.pem")
	}

	// Handle htpasswd file for basic auth
	if err := d.ensureHTPasswd(host); err != nil {
		log.Printf("Warning: failed to create htpasswd for %s: %v", host.Domain, err)
	}

	// Generate nginx config
	if err := nginx.GenerateConfig(host, d.configPath); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	// Enable config
	if err := nginx.EnableConfig(host.Domain, d.configPath, d.enabledPath); err != nil {
		return fmt.Errorf("failed to enable config: %w", err)
	}

	// Apply with rollback (backup, test, reload)
	if err := d.manager.ApplyWithRollback(host.Domain); err != nil {
		return fmt.Errorf("failed to apply config: %w", err)
	}

	log.Printf("Successfully deployed host: %s", host.Domain)
	return nil
}

// RemoveHost removes a host configuration from nginx
func (d *Deployer) RemoveHost(domain string) error {
	// Backup current config before removing
	if err := d.manager.BackupConfig(domain); err != nil {
		log.Printf("Warning: failed to backup config for %s: %v", domain, err)
	}

	// Disable config (remove symlink)
	if err := nginx.DisableConfig(domain, d.enabledPath); err != nil {
		return fmt.Errorf("failed to disable config: %w", err)
	}

	// Remove config file
	safeDomain := nginx.SanitizeDomain(domain)
	configFile := filepath.Join(d.configPath, fmt.Sprintf("proxera_%s.conf", safeDomain))
	if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove config file: %w", err)
	}

	// Test nginx config
	if err := d.manager.Test(); err != nil {
		// Rollback: restore backup and re-enable
		log.Printf("Nginx test failed after removing %s, rolling back...", domain)
		if restoreErr := d.manager.RestoreBackup(domain); restoreErr != nil {
			return fmt.Errorf("test failed and rollback failed: %v (original: %v)", restoreErr, err)
		}
		if err := nginx.EnableConfig(domain, d.configPath, d.enabledPath); err != nil {
			return fmt.Errorf("failed to re-enable config during rollback: %w", err)
		}
		return fmt.Errorf("nginx test failed, rolled back: %v", err)
	}

	// Reload nginx
	if err := d.manager.Reload(); err != nil {
		return fmt.Errorf("failed to reload nginx: %w", err)
	}

	// Remove SSL directory
	sslDir := d.sslDir(domain)
	if err := os.RemoveAll(sslDir); err != nil {
		log.Printf("Warning: failed to remove SSL dir for %s: %v", domain, err)
	}

	// Remove htpasswd file
	d.removeHTPasswd(domain)

	log.Printf("Successfully removed host: %s", domain)
	return nil
}

// ApplyAll performs a full state sync of all hosts
func (d *Deployer) ApplyAll(hosts []types.Host) (int, error) {
	// Ensure Cloudflare real IP config exists
	if err := d.EnsureCloudflareRealIP(); err != nil {
		log.Printf("Warning: failed to ensure Cloudflare real IP config: %v", err)
	}

	// Ensure metrics log format exists
	if err := d.EnsureMetricsLogFormat(); err != nil {
		log.Printf("Warning: failed to ensure metrics log format: %v", err)
	}

	// Ensure log rotation is configured
	if err := d.EnsureLogRotation(); err != nil {
		log.Printf("Warning: failed to ensure log rotation: %v", err)
	}

	// Track which proxera configs should exist
	expected := make(map[string]bool)

	applied := 0
	var errors []string

	for _, host := range hosts {
		safeDomain := nginx.SanitizeDomain(host.Domain)
		expected[fmt.Sprintf("proxera_%s.conf", safeDomain)] = true

		// Write SSL certs if provided
		if host.SSL && host.CertPEM != "" && host.KeyPEM != "" {
			// Validate PEM format, key-cert matching, and expiry before writing
			if err := validateSSLCerts(host.CertPEM, host.KeyPEM, host.IssuerPEM, host.Domain); err != nil {
				errors = append(errors, fmt.Sprintf("%s: SSL validation failed: %v", host.Domain, err))
				continue
			}

			certDir := d.sslDir(host.Domain)
			if err := os.MkdirAll(certDir, 0700); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to create SSL dir: %v", host.Domain, err))
				continue
			}

			fullchain := host.CertPEM
			if host.IssuerPEM != "" {
				fullchain = host.CertPEM + "\n" + host.IssuerPEM
			}

			if err := os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte(fullchain), 0600); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to write cert: %v", host.Domain, err))
				continue
			}
			if err := os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte(host.KeyPEM), 0600); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to write key: %v", host.Domain, err))
				continue
			}

			host.CertPath = filepath.Join(certDir, "fullchain.pem")
			host.KeyPath = filepath.Join(certDir, "privkey.pem")
		}

		// Handle htpasswd file for basic auth
		if err := d.ensureHTPasswd(host); err != nil {
			log.Printf("Warning: failed to create htpasswd for %s: %v", host.Domain, err)
		}

		// Generate config
		if err := nginx.GenerateConfig(host, d.configPath); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", host.Domain, err))
			continue
		}

		// Enable config
		if err := nginx.EnableConfig(host.Domain, d.configPath, d.enabledPath); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", host.Domain, err))
			continue
		}

		applied++
	}

	// Clean stale proxera configs not in the expected set
	entries, err := os.ReadDir(d.enabledPath)
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, "proxera_") && strings.HasSuffix(name, ".conf") {
				if !expected[name] {
					target := filepath.Join(d.enabledPath, name)
					os.Remove(target)
					// Also remove from sites-available
					source := filepath.Join(d.configPath, name)
					os.Remove(source)
					log.Printf("Cleaned stale config: %s", name)
				}
			}
		}
	}

	// Test and reload once for all changes
	if err := d.manager.Test(); err != nil {
		return applied, fmt.Errorf("nginx test failed after applying %d hosts: %v", applied, err)
	}

	if err := d.manager.Reload(); err != nil {
		return applied, fmt.Errorf("nginx reload failed after applying %d hosts: %v", applied, err)
	}

	if len(errors) > 0 {
		return applied, fmt.Errorf("applied %d hosts with %d errors: %s", applied, len(errors), strings.Join(errors, "; "))
	}

	log.Printf("Successfully applied all %d hosts", applied)
	return applied, nil
}

// sslDir returns the SSL certificate directory for a domain
func (d *Deployer) sslDir(domain string) string {
	safeDomain := nginx.SanitizeDomain(domain)
	return filepath.Join("/etc/nginx/ssl", fmt.Sprintf("proxera_%s", safeDomain))
}
