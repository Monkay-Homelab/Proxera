package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	dns "github.com/proxera/backend/internal/dns"
	"github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/database"
)

type AddDNSProviderRequest struct {
	Provider  string `json:"provider"`
	ZoneID    string `json:"zone_id"`            // Cloudflare: required; IONOS/others: omit
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`          // Porkbun / GoDaddy; empty for others
	Domain    string `json:"domain"`              // IONOS / Hetzner: domain for zone UUID lookup
}

type DNSProviderResponse struct {
	ID        int       `json:"id"`
	Provider  string    `json:"provider"`
	ZoneID    string    `json:"zone_id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

func AddDNSProvider(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var req AddDNSProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Provider == "" || req.APIKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Provider and api_key are required"})
	}

	p, err := dns.Get(req.Provider)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	creds := dns.Credentials{
		ZoneID:    req.ZoneID,
		APIKey:    req.APIKey,
		APISecret: req.APISecret,
		Domain:    req.Domain,
	}
	domain, zoneID, err := p.VerifyZone(c.Context(), creds)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("%s verification failed: %s", req.Provider, err.Error()),
		})
	}

	// Encrypt credentials before storing. Use the zoneID returned by VerifyZone —
	// for providers like IONOS it's the UUID looked up by domain, not what the user typed.
	encZoneID, err := crypto.Encrypt(zoneID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encrypt credentials"})
	}
	encAPIKey, err := crypto.Encrypt(req.APIKey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encrypt credentials"})
	}
	encAPISecret := ""
	if req.APISecret != "" {
		encAPISecret, err = crypto.Encrypt(req.APISecret)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encrypt credentials"})
		}
	}

	var id int
	var createdAt time.Time
	err = database.DB.QueryRow(
		context.Background(),
		`INSERT INTO dns_providers (user_id, provider, zone_id, api_key, api_secret, domain)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		 RETURNING id, created_at`,
		userID, req.Provider, encZoneID, encAPIKey, encAPISecret, domain,
	).Scan(&id, &createdAt)
	if err != nil {
		slog.Error("DB insert failed for DNS provider", "component", "dns", "provider", req.Provider, "domain", domain, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save DNS provider"})
	}

	// Auto-sync DNS records in the background (best-effort)
	// Must update both Domain and ZoneID — VerifyZone may have resolved the zone UUID (e.g. IONOS).
	creds.ZoneID = zoneID
	creds.Domain = domain
	go func() {
		if _, err := syncRecordsForProvider(id, req.Provider, creds); err != nil {
			slog.Error("auto-sync failed after adding provider", "component", "dns", "provider_id", id, "error", err)
		}
	}()

	return c.Status(fiber.StatusCreated).JSON(DNSProviderResponse{
		ID:        id,
		Provider:  req.Provider,
		ZoneID:    zoneID,
		Domain:    domain,
		CreatedAt: createdAt,
	})
}

func ListDNSProviders(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	var query string
	var args []interface{}
	if role == "admin" {
		query = `SELECT id, provider, zone_id, domain, created_at
			 FROM dns_providers ORDER BY created_at DESC`
	} else {
		query = `SELECT id, provider, zone_id, domain, created_at
			 FROM dns_providers WHERE user_id = $1
			 OR id IN (SELECT dns_provider_id FROM user_dns_providers WHERE user_id = $1)
			 ORDER BY created_at DESC`
		args = append(args, userID)
	}

	rows, err := database.DB.Query(context.Background(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch DNS providers"})
	}
	defer rows.Close()

	providers := []DNSProviderResponse{}
	for rows.Next() {
		var p DNSProviderResponse
		var encZoneID string
		var domain *string
		if err := rows.Scan(&p.ID, &p.Provider, &encZoneID, &domain, &p.CreatedAt); err != nil {
			continue
		}
		if decrypted, err := crypto.Decrypt(encZoneID); err == nil {
			p.ZoneID = decrypted
		}
		if domain != nil {
			p.Domain = *domain
		}
		providers = append(providers, p)
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating DNS providers", "component", "dns", "error", err)
	}

	return c.JSON(providers)
}

type DNSRecordResponse struct {
	ID         int       `json:"id"`
	ProviderID int       `json:"dns_provider_id"`
	CfRecordID string    `json:"cf_record_id"` // kept for API compatibility; equals provider_record_id
	Type       string    `json:"type"`
	Name       string    `json:"name"`
	Content    string    `json:"content"`
	TTL        int       `json:"ttl"`
	Proxied    bool      `json:"proxied"`
	LastSynced time.Time `json:"last_synced"`
	AgentID    *int      `json:"agent_id"`
	AgentName  *string   `json:"agent_name"`
}

// GetProviderCreds fetches and decrypts all credentials for a provider row.
func GetProviderCreds(providerID, userID int, role string) (providerType string, creds dns.Credentials, err error) {
	var encZoneID, encAPIKey string
	var encAPISecret, domain *string

	var query string
	var args []interface{}
	if role == "admin" {
		query = `SELECT provider, zone_id, api_key, api_secret, domain
			 FROM dns_providers WHERE id = $1`
		args = append(args, providerID)
	} else {
		query = `SELECT provider, zone_id, api_key, api_secret, domain
			 FROM dns_providers WHERE id = $1 AND (user_id = $2 OR id IN (SELECT dns_provider_id FROM user_dns_providers WHERE user_id = $2))`
		args = append(args, providerID, userID)
	}

	err = database.DB.QueryRow(context.Background(), query, args...).Scan(&providerType, &encZoneID, &encAPIKey, &encAPISecret, &domain)
	if err != nil {
		return "", dns.Credentials{}, fmt.Errorf("provider not found: %w", err)
	}

	creds.ZoneID, err = crypto.Decrypt(encZoneID)
	if err != nil {
		return "", dns.Credentials{}, fmt.Errorf("failed to decrypt zone_id: %w", err)
	}
	creds.APIKey, err = crypto.Decrypt(encAPIKey)
	if err != nil {
		return "", dns.Credentials{}, fmt.Errorf("failed to decrypt api_key: %w", err)
	}
	if encAPISecret != nil && *encAPISecret != "" {
		creds.APISecret, err = crypto.Decrypt(*encAPISecret)
		if err != nil {
			return "", dns.Credentials{}, fmt.Errorf("failed to decrypt api_secret: %w", err)
		}
	}
	if domain != nil {
		creds.Domain = *domain
	}

	return providerType, creds, nil
}

// syncRecordsForProvider fetches records from the provider and upserts them in the database,
// preserving agent_id assignments across syncs.
func syncRecordsForProvider(providerID int, providerType string, creds dns.Credentials) ([]DNSRecordResponse, error) {
	p, err := dns.Get(providerType)
	if err != nil {
		return nil, err
	}

	remoteRecords, err := p.ListRecords(context.Background(), creds)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	now := time.Now()

	remoteIDs := make([]string, 0, len(remoteRecords))
	records := make([]DNSRecordResponse, 0, len(remoteRecords))

	for _, r := range remoteRecords {
		remoteIDs = append(remoteIDs, r.ProviderID)
		var id int
		var agentID *int
		var agentName *string
		err = database.DB.QueryRow(ctx,
			`INSERT INTO dns_records (dns_provider_id, cf_record_id, provider_record_id, record_type, name, content, ttl, proxied, last_synced)
			 VALUES ($1, $2, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (dns_provider_id, provider_record_id) DO UPDATE SET
			   record_type = EXCLUDED.record_type, name = EXCLUDED.name, content = EXCLUDED.content,
			   ttl = EXCLUDED.ttl, proxied = EXCLUDED.proxied, last_synced = EXCLUDED.last_synced
			 RETURNING id, agent_id`,
			providerID, r.ProviderID, r.Type, r.Name, r.Content, r.TTL, r.Proxied, now,
		).Scan(&id, &agentID)
		if err != nil {
			slog.Error("failed to upsert DNS record", "component", "dns", "record_name", r.Name, "error", err)
			continue
		}
		if agentID != nil {
			_ = database.DB.QueryRow(ctx, `SELECT name FROM agents WHERE id = $1`, *agentID).Scan(&agentName)
		}
		records = append(records, DNSRecordResponse{
			ID:         id,
			ProviderID: providerID,
			CfRecordID: r.ProviderID,
			Type:       r.Type,
			Name:       r.Name,
			Content:    r.Content,
			TTL:        r.TTL,
			Proxied:    r.Proxied,
			LastSynced: now,
			AgentID:    agentID,
			AgentName:  agentName,
		})
	}

	// Delete stale records that no longer exist at the provider
	if len(remoteIDs) > 0 {
		_, err = database.DB.Exec(ctx,
			`DELETE FROM dns_records WHERE dns_provider_id = $1 AND provider_record_id != ALL($2)`,
			providerID, remoteIDs,
		)
		if err != nil {
			slog.Error("failed to clean stale DNS records", "component", "dns", "provider_id", providerID, "error", err)
		}
	}

	return records, nil
}

func SyncDNSRecords(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}

	providerType, creds, err := GetProviderCreds(providerID, userID, role)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	records, err := syncRecordsForProvider(providerID, providerType, creds)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to sync records: %s", err.Error()),
		})
	}

	return c.JSON(records)
}

func ListDNSRecords(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}

	var exists bool
	if role == "admin" {
		err = database.DB.QueryRow(
			context.Background(),
			`SELECT EXISTS(SELECT 1 FROM dns_providers WHERE id = $1)`,
			providerID,
		).Scan(&exists)
	} else {
		err = database.DB.QueryRow(
			context.Background(),
			`SELECT EXISTS(SELECT 1 FROM dns_providers WHERE id = $1 AND (user_id = $2 OR id IN (SELECT dns_provider_id FROM user_dns_providers WHERE user_id = $2)))`,
			providerID, userID,
		).Scan(&exists)
	}
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var count int
	var lastSynced *time.Time
	_ = database.DB.QueryRow(
		context.Background(),
		`SELECT COUNT(*), MAX(last_synced) FROM dns_records WHERE dns_provider_id = $1`,
		providerID,
	).Scan(&count, &lastSynced)

	needsSync := count == 0 || lastSynced == nil || time.Since(*lastSynced) > 24*time.Hour
	if needsSync {
		providerType, creds, err := GetProviderCreds(providerID, userID, role)
		if err == nil {
			if synced, err := syncRecordsForProvider(providerID, providerType, creds); err == nil {
				return c.JSON(synced)
			}
			slog.Error("auto-sync failed for provider", "component", "dns", "provider_id", providerID, "error", err)
		}
	}

	rows, err := database.DB.Query(
		context.Background(),
		`SELECT r.id, r.dns_provider_id, r.provider_record_id, r.record_type, r.name, r.content, r.ttl, r.proxied, r.last_synced, r.agent_id, a.name
		 FROM dns_records r
		 LEFT JOIN agents a ON r.agent_id = a.id
		 WHERE r.dns_provider_id = $1 ORDER BY r.record_type, r.name`,
		providerID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch DNS records"})
	}
	defer rows.Close()

	records := []DNSRecordResponse{}
	for rows.Next() {
		var r DNSRecordResponse
		if err := rows.Scan(&r.ID, &r.ProviderID, &r.CfRecordID, &r.Type, &r.Name, &r.Content, &r.TTL, &r.Proxied, &r.LastSynced, &r.AgentID, &r.AgentName); err != nil {
			continue
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating DNS records", "component", "dns", "error", err)
	}

	return c.JSON(records)
}

type DNSRecordRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func CreateDNSRecord(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}

	var req DNSRecordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.Type == "" || req.Name == "" || req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Type, name, and content are required"})
	}

	providerType, creds, err := GetProviderCreds(providerID, userID, role)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	p, err := dns.Get(providerType)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	created, err := p.CreateRecord(c.Context(), creds, dns.RecordInput{
		Type:    req.Type,
		Name:    req.Name,
		Content: req.Content,
		TTL:     req.TTL,
		Proxied: req.Proxied,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("%s error: %s", providerType, err.Error())})
	}

	now := time.Now()
	var id int
	err = database.DB.QueryRow(
		context.Background(),
		`INSERT INTO dns_records (dns_provider_id, cf_record_id, provider_record_id, record_type, name, content, ttl, proxied, last_synced)
		 VALUES ($1, $2, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		providerID, created.ProviderID, created.Type, created.Name, created.Content, created.TTL, created.Proxied, now,
	).Scan(&id)
	if err != nil {
		slog.Error("failed to cache new DNS record", "component", "dns", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Record created at provider but failed to cache locally"})
	}

	return c.Status(fiber.StatusCreated).JSON(DNSRecordResponse{
		ID:         id,
		ProviderID: providerID,
		CfRecordID: created.ProviderID,
		Type:       created.Type,
		Name:       created.Name,
		Content:    created.Content,
		TTL:        created.TTL,
		Proxied:    created.Proxied,
		LastSynced: now,
	})
}

func UpdateDNSRecord(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}
	recordID, err := strconv.Atoi(c.Params("recordId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid record ID"})
	}

	var req DNSRecordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.Type == "" || req.Name == "" || req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Type, name, and content are required"})
	}

	providerType, creds, err := GetProviderCreds(providerID, userID, role)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var providerRecordID string
	err = database.DB.QueryRow(
		context.Background(),
		`SELECT provider_record_id FROM dns_records WHERE id = $1 AND dns_provider_id = $2`,
		recordID, providerID,
	).Scan(&providerRecordID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS record not found"})
	}

	p, err := dns.Get(providerType)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	updated, err := p.UpdateRecord(c.Context(), creds, providerRecordID, dns.RecordInput{
		Type:    req.Type,
		Name:    req.Name,
		Content: req.Content,
		TTL:     req.TTL,
		Proxied: req.Proxied,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("%s error: %s", providerType, err.Error())})
	}

	now := time.Now()
	_, err = database.DB.Exec(
		context.Background(),
		`UPDATE dns_records SET record_type = $1, name = $2, content = $3, ttl = $4, proxied = $5, last_synced = $6
		 WHERE id = $7`,
		updated.Type, updated.Name, updated.Content, updated.TTL, updated.Proxied, now, recordID,
	)
	if err != nil {
		slog.Error("failed to update cached DNS record", "component", "dns", "error", err)
	}

	return c.JSON(DNSRecordResponse{
		ID:         recordID,
		ProviderID: providerID,
		CfRecordID: providerRecordID,
		Type:       updated.Type,
		Name:       updated.Name,
		Content:    updated.Content,
		TTL:        updated.TTL,
		Proxied:    updated.Proxied,
		LastSynced: now,
	})
}

func DeleteDNSRecord(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}
	recordID, err := strconv.Atoi(c.Params("recordId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid record ID"})
	}

	providerType, creds, err := GetProviderCreds(providerID, userID, role)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var providerRecordID string
	err = database.DB.QueryRow(
		context.Background(),
		`SELECT provider_record_id FROM dns_records WHERE id = $1 AND dns_provider_id = $2`,
		recordID, providerID,
	).Scan(&providerRecordID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS record not found"})
	}

	p, err := dns.Get(providerType)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := p.DeleteRecord(c.Context(), creds, providerRecordID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("%s error: %s", providerType, err.Error())})
	}

	_, err = database.DB.Exec(context.Background(), `DELETE FROM dns_records WHERE id = $1`, recordID)
	if err != nil {
		slog.Error("failed to delete cached DNS record", "component", "dns", "error", err)
	}

	return c.JSON(fiber.Map{"message": "DNS record deleted"})
}

func DeleteDNSProvider(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	id := c.Params("id")

	var query string
	var args []interface{}
	if role == "admin" {
		query = `DELETE FROM dns_providers WHERE id = $1`
		args = append(args, id)
	} else {
		query = `DELETE FROM dns_providers WHERE id = $1 AND user_id = $2`
		args = append(args, id, userID)
	}

	result, err := database.DB.Exec(context.Background(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete DNS provider"})
	}
	if result.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	return c.JSON(fiber.Map{"message": "DNS provider deleted"})
}

func AssignDNSRecordAgent(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	providerID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}
	recordID, err := strconv.Atoi(c.Params("recordId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid record ID"})
	}

	var exists bool
	err = database.DB.QueryRow(
		context.Background(),
		`SELECT EXISTS(SELECT 1 FROM dns_providers WHERE id = $1 AND user_id = $2)`,
		providerID, userID,
	).Scan(&exists)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var body struct {
		AgentID *int `json:"agent_id"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if body.AgentID != nil {
		var agentExists bool
		err = database.DB.QueryRow(
			context.Background(),
			`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND user_id = $2)`,
			*body.AgentID, userID,
		).Scan(&agentExists)
		if err != nil || !agentExists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Agent not found"})
		}
	}

	result, err := database.DB.Exec(
		context.Background(),
		`UPDATE dns_records SET agent_id = $1 WHERE id = $2 AND dns_provider_id = $3`,
		body.AgentID, recordID, providerID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign agent"})
	}
	if result.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS record not found"})
	}

	// Do NOT auto-sync on assignment — only the explicit force-sync button or a heartbeat WAN IP
	// change should update provider records. Auto-syncing on assignment silently overwrites the
	// existing record content, which may be intentionally different from the agent's WAN IP.

	var r DNSRecordResponse
	err = database.DB.QueryRow(
		context.Background(),
		`SELECT r.id, r.dns_provider_id, r.provider_record_id, r.record_type, r.name, r.content, r.ttl, r.proxied, r.last_synced, r.agent_id, a.name
		 FROM dns_records r
		 LEFT JOIN agents a ON r.agent_id = a.id
		 WHERE r.id = $1`,
		recordID,
	).Scan(&r.ID, &r.ProviderID, &r.CfRecordID, &r.Type, &r.Name, &r.Content, &r.TTL, &r.Proxied, &r.LastSynced, &r.AgentID, &r.AgentName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch updated record"})
	}

	return c.JSON(r)
}

// DDNSSyncRecord manually triggers a DDNS update for all records assigned to the agent on a given record.
// POST /api/dns/providers/:id/records/:recordId/ddns-sync
func DDNSSyncRecord(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	providerID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}
	recordID, err := strconv.Atoi(c.Params("recordId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid record ID"})
	}

	var exists bool
	err = database.DB.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM dns_providers WHERE id = $1 AND user_id = $2)`,
		providerID, userID,
	).Scan(&exists)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var agentID int
	err = database.DB.QueryRow(context.Background(),
		`SELECT COALESCE(agent_id, 0) FROM dns_records WHERE id = $1 AND dns_provider_id = $2`,
		recordID, providerID,
	).Scan(&agentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS record not found"})
	}
	if agentID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "No agent assigned to this record"})
	}

	var wanIP string
	err = database.DB.QueryRow(context.Background(),
		`SELECT COALESCE(wan_ip, '') FROM agents WHERE id = $1 AND user_id = $2`,
		agentID, userID,
	).Scan(&wanIP)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Agent not found"})
	}
	if wanIP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Agent WAN IP not yet known — wait for the next heartbeat"})
	}

	UpdateDDNSForAgent(agentID, userID, wanIP)

	var r DNSRecordResponse
	err = database.DB.QueryRow(context.Background(),
		`SELECT r.id, r.dns_provider_id, r.provider_record_id, r.record_type, r.name, r.content, r.ttl, r.proxied, r.last_synced, r.agent_id, a.name
		 FROM dns_records r
		 LEFT JOIN agents a ON r.agent_id = a.id
		 WHERE r.id = $1`,
		recordID,
	).Scan(&r.ID, &r.ProviderID, &r.CfRecordID, &r.Type, &r.Name, &r.Content, &r.TTL, &r.Proxied, &r.LastSynced, &r.AgentID, &r.AgentName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch updated record"})
	}

	return c.JSON(r)
}

// UpdateDDNSForAgent updates all DNS records assigned to an agent when its WAN IP changes.
// Called as a goroutine from the heartbeat handler — runs in the background and logs results.
func UpdateDDNSForAgent(agentDBID int, agentUserID int, newWanIP string) {
	ctx := context.Background()

	rows, err := database.DB.Query(ctx,
		`SELECT r.id, r.provider_record_id, r.record_type, r.name, r.content,
		        p.id, p.provider, p.zone_id, p.api_key, COALESCE(p.api_secret, ''), COALESCE(p.domain, '')
		 FROM dns_records r
		 JOIN dns_providers p ON r.dns_provider_id = p.id
		 WHERE r.agent_id = $1 AND p.user_id = $2
		   AND r.record_type IN ('A', 'AAAA')`,
		agentDBID, agentUserID,
	)
	if err != nil {
		slog.Error("failed to query records for agent", "component", "dns", "operation", "ddns", "agent_id", agentDBID, "error", err)
		return
	}
	defer rows.Close()

	type ddnsRecord struct {
		id               int
		providerRecordID string
		recordType       string
		name             string
		content          string
		providerID       int
		providerType     string
		encZoneID        string
		encAPIKey        string
		encAPISecret     string
		domain           string
	}

	var records []ddnsRecord
	for rows.Next() {
		var r ddnsRecord
		if err := rows.Scan(&r.id, &r.providerRecordID, &r.recordType, &r.name, &r.content,
			&r.providerID, &r.providerType, &r.encZoneID, &r.encAPIKey, &r.encAPISecret, &r.domain); err != nil {
			slog.Error("failed to scan DDNS record", "component", "dns", "operation", "ddns", "error", err)
			continue
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating DDNS records", "component", "dns", "operation", "ddns", "agent_id", agentDBID, "error", err)
	}
	if len(records) == 0 {
		return
	}

	isIPv6 := strings.Contains(newWanIP, ":")

	// Cache decrypted credentials per provider to avoid redundant decryption
	type cachedCreds struct {
		providerType string
		creds        dns.Credentials
	}
	credsCache := make(map[int]*cachedCreds)

	for _, r := range records {
		if r.recordType == "A" && isIPv6 {
			continue
		}
		if r.recordType == "AAAA" && !isIPv6 {
			continue
		}
		if r.content == newWanIP {
			continue
		}

		cc, ok := credsCache[r.providerID]
		if !ok {
			zoneID, err := crypto.Decrypt(r.encZoneID)
			if err != nil {
				slog.Error("failed to decrypt zone_id", "component", "dns", "operation", "ddns", "provider_id", r.providerID, "error", err)
				continue
			}
			apiKey, err := crypto.Decrypt(r.encAPIKey)
			if err != nil {
				slog.Error("failed to decrypt api_key", "component", "dns", "operation", "ddns", "provider_id", r.providerID, "error", err)
				continue
			}
			apiSecret := ""
			if r.encAPISecret != "" {
				apiSecret, err = crypto.Decrypt(r.encAPISecret)
				if err != nil {
					slog.Error("failed to decrypt api_secret", "component", "dns", "operation", "ddns", "provider_id", r.providerID, "error", err)
					continue
				}
			}
			cc = &cachedCreds{
				providerType: r.providerType,
				creds:        dns.Credentials{ZoneID: zoneID, APIKey: apiKey, APISecret: apiSecret, Domain: r.domain},
			}
			credsCache[r.providerID] = cc
		}

		p, err := dns.Get(cc.providerType)
		if err != nil {
			slog.Error("unknown provider for DDNS record", "component", "dns", "operation", "ddns", "provider", cc.providerType, "record_name", r.name, "error", err)
			continue
		}

		if err := p.PatchContent(ctx, cc.creds, r.providerRecordID, newWanIP); err != nil {
			slog.Error("failed to update DDNS record at provider", "component", "dns", "operation", "ddns", "record_name", r.name, "provider_record_id", r.providerRecordID, "error", err)
			continue
		}

		_, err = database.DB.Exec(ctx,
			`UPDATE dns_records SET content = $1, last_synced = NOW() WHERE id = $2`,
			newWanIP, r.id,
		)
		if err != nil {
			slog.Error("failed to update local DDNS record", "component", "dns", "operation", "ddns", "record_id", r.id, "error", err)
		}

		slog.Info("DDNS record updated", "component", "dns", "operation", "ddns", "record_name", r.name, "old_ip", r.content, "new_ip", newWanIP)
	}
}
