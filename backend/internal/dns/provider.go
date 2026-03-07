package dns

import "context"

// Credentials holds decrypted credentials for a DNS provider.
// Only the fields relevant to the specific provider will be populated.
type Credentials struct {
	ZoneID    string // Cloudflare Zone ID / Hetzner Zone ID
	APIKey    string // Primary API token or key (all providers)
	APISecret string // Secondary secret — Porkbun, GoDaddy; empty for others
	Domain    string // Canonical zone domain name (cached)
}

// Record is a normalised DNS record returned by any provider.
type Record struct {
	ProviderID string // Provider's own record identifier
	Type       string // A, AAAA, CNAME, MX, TXT, …
	Name       string // Always FQDN (e.g. "www.example.com")
	Content    string // IP address, target hostname, or text value
	TTL        int    // Seconds; 1 means "auto/default" for the provider
	Proxied    bool   // Cloudflare only; always false for other providers
}

// RecordInput is used for create and full-update operations.
type RecordInput struct {
	Type    string
	Name    string
	Content string
	TTL     int
	Proxied bool // Ignored for non-Cloudflare providers
}

// Provider is the interface every DNS provider must implement.
type Provider interface {
	// VerifyZone validates credentials and returns the canonical domain name and the
	// zone ID to store. For providers like Cloudflare, zoneID == creds.ZoneID.
	// For providers like IONOS/Hetzner where the zone ID is an opaque UUID, VerifyZone
	// looks it up by domain name and returns it so it can be persisted.
	VerifyZone(ctx context.Context, creds Credentials) (domain, zoneID string, err error)

	// ListRecords returns all DNS records for the zone.
	ListRecords(ctx context.Context, creds Credentials) ([]Record, error)

	// CreateRecord creates a new record and returns it with ProviderID populated.
	CreateRecord(ctx context.Context, creds Credentials, r RecordInput) (Record, error)

	// UpdateRecord does a full replacement of a record's fields.
	UpdateRecord(ctx context.Context, creds Credentials, providerID string, r RecordInput) (Record, error)

	// PatchContent updates only the content (IP) of a record — used for DDNS.
	PatchContent(ctx context.Context, creds Credentials, providerID string, content string) error

	// DeleteRecord removes the record.
	DeleteRecord(ctx context.Context, creds Credentials, providerID string) error
}
