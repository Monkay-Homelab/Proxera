package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var ionosClient = &http.Client{Timeout: 15 * time.Second}

const ionosBase = "https://api.hosting.ionos.com/dns/v1"

// IONOSProvider implements Provider for IONOS Managed DNS.
type IONOSProvider struct{}

func (p *IONOSProvider) VerifyZone(ctx context.Context, creds Credentials) (string, string, error) {
	url := ionosBase + "/zones"
	slog.Info("fetching zones", "component", "dns", "provider", "ionos", "operation", "VerifyZone", "domain", creds.Domain)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		slog.Error("VerifyZone request failed", "component", "dns", "provider", "ionos", "error", err)
		return "", "", fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info("VerifyZone response received", "component", "dns", "provider", "ionos", "status", resp.StatusCode)

	if resp.StatusCode == 401 {
		return "", "", fmt.Errorf("invalid API key")
	}
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("IONOS API returned status %d", resp.StatusCode)
	}

	var zones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&zones); err != nil {
		return "", "", fmt.Errorf("failed to parse IONOS response: %w", err)
	}
	slog.Info("VerifyZone found zones", "component", "dns", "provider", "ionos", "zone_count", len(zones))

	want := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(creds.Domain), "."))
	for _, z := range zones {
		if strings.ToLower(z.Name) == want {
			slog.Info("VerifyZone matched zone", "component", "dns", "provider", "ionos", "zone_name", z.Name, "zone_id", z.ID)
			return z.Name, z.ID, nil
		}
	}
	return "", "", fmt.Errorf("domain %q not found in your IONOS account", creds.Domain)
}

func (p *IONOSProvider) ListRecords(ctx context.Context, creds Credentials) ([]Record, error) {
	url := fmt.Sprintf("%s/zones/%s", ionosBase, creds.ZoneID)
	slog.Info("listing records", "component", "dns", "provider", "ionos", "operation", "ListRecords", "zone_id", creds.ZoneID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		slog.Error("ListRecords request failed", "component", "dns", "provider", "ionos", "error", err)
		return nil, fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info("ListRecords response received", "component", "dns", "provider", "ionos", "status", resp.StatusCode)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("IONOS API returned status %d", resp.StatusCode)
	}

	var zone struct {
		Records []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			Content  string `json:"content"`
			TTL      int    `json:"ttl"`
			Disabled bool   `json:"disabled"`
		} `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&zone); err != nil {
		return nil, fmt.Errorf("failed to parse IONOS response: %w", err)
	}

	records := make([]Record, 0, len(zone.Records))
	for _, r := range zone.Records {
		if r.Disabled {
			continue
		}
		records = append(records, Record{
			ProviderID: r.ID,
			Type:       r.Type,
			Name:       r.Name,
			Content:    r.Content,
			TTL:        r.TTL,
		})
	}
	slog.Info("ListRecords completed", "component", "dns", "provider", "ionos", "returned", len(records), "total", len(zone.Records))
	return records, nil
}

func (p *IONOSProvider) CreateRecord(ctx context.Context, creds Credentials, r RecordInput) (Record, error) {
	url := fmt.Sprintf("%s/zones/%s/records", ionosBase, creds.ZoneID)
	slog.Info("creating record", "component", "dns", "provider", "ionos", "record_type", r.Type, "name", r.Name, "content", r.Content)

	// IONOS takes an array body
	body := []map[string]interface{}{
		{
			"name":     r.Name,
			"type":     r.Type,
			"content":  r.Content,
			"ttl":      r.TTL,
			"disabled": false,
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return Record{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return Record{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		slog.Error("CreateRecord request failed", "component", "dns", "provider", "ionos", "error", err)
		return Record{}, fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info("CreateRecord response received", "component", "dns", "provider", "ionos", "status", resp.StatusCode)

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		errBody, _ := io.ReadAll(resp.Body)
		slog.Error("CreateRecord failed", "component", "dns", "provider", "ionos", "status", resp.StatusCode, "response_body", string(errBody))
		return Record{}, fmt.Errorf("IONOS API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	var created []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return Record{}, fmt.Errorf("failed to parse IONOS response: %w", err)
	}
	if len(created) == 0 {
		return Record{}, fmt.Errorf("IONOS returned no records after creation")
	}
	slog.Info("CreateRecord completed", "component", "dns", "provider", "ionos", "record_id", created[0].ID, "name", created[0].Name)
	return Record{
		ProviderID: created[0].ID,
		Type:       created[0].Type,
		Name:       created[0].Name,
		Content:    created[0].Content,
		TTL:        created[0].TTL,
	}, nil
}

func (p *IONOSProvider) UpdateRecord(ctx context.Context, creds Credentials, providerID string, r RecordInput) (Record, error) {
	url := fmt.Sprintf("%s/zones/%s/records/%s", ionosBase, creds.ZoneID, providerID)
	slog.Info("updating record", "component", "dns", "provider", "ionos", "record_id", providerID, "record_type", r.Type, "name", r.Name, "content", r.Content)

	body := map[string]interface{}{
		"name":     r.Name,
		"type":     r.Type,
		"content":  r.Content,
		"ttl":      r.TTL,
		"disabled": false,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return Record{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(data))
	if err != nil {
		return Record{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		slog.Error("UpdateRecord request failed", "component", "dns", "provider", "ionos", "error", err)
		return Record{}, fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info("UpdateRecord response received", "component", "dns", "provider", "ionos", "status", resp.StatusCode)

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		slog.Error("UpdateRecord failed", "component", "dns", "provider", "ionos", "status", resp.StatusCode, "response_body", string(errBody))
		return Record{}, fmt.Errorf("IONOS API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	var updated struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		// PUT may return 200 with empty body — fall back to input values
		slog.Warn("UpdateRecord returned empty body, using input values", "component", "dns", "provider", "ionos", "record_id", providerID)
		return Record{ProviderID: providerID, Type: r.Type, Name: r.Name, Content: r.Content, TTL: r.TTL}, nil
	}
	return Record{
		ProviderID: providerID,
		Type:       updated.Type,
		Name:       updated.Name,
		Content:    updated.Content,
		TTL:        updated.TTL,
	}, nil
}

func (p *IONOSProvider) PatchContent(ctx context.Context, creds Credentials, providerID string, content string) error {
	// IONOS has no PATCH — we need the current record's name/type/TTL to issue a full PUT.
	// Fetch the zone and find the record by ID.
	zoneURL := fmt.Sprintf("%s/zones/%s", ionosBase, creds.ZoneID)
	slog.Info("fetching zone for PatchContent", "component", "dns", "provider", "ionos", "record_id", providerID)

	req, err := http.NewRequestWithContext(ctx, "GET", zoneURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		slog.Error("PatchContent pre-fetch failed", "component", "dns", "provider", "ionos", "error", err)
		return fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info("PatchContent pre-fetch response received", "component", "dns", "provider", "ionos", "status", resp.StatusCode)

	var zone struct {
		Records []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Content string `json:"content"`
			TTL     int    `json:"ttl"`
		} `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&zone); err != nil {
		return fmt.Errorf("failed to parse IONOS zone: %w", err)
	}

	for _, r := range zone.Records {
		if r.ID == providerID {
			slog.Info("PatchContent updating record", "component", "dns", "provider", "ionos", "record_id", providerID, "record_type", r.Type, "name", r.Name, "old_content", r.Content, "new_content", content)
			_, err = p.UpdateRecord(ctx, creds, providerID, RecordInput{
				Name:    r.Name,
				Type:    r.Type,
				Content: content,
				TTL:     r.TTL,
			})
			return err
		}
	}
	slog.Warn("PatchContent record not found in zone", "component", "dns", "provider", "ionos", "record_id", providerID, "zone_record_count", len(zone.Records))
	return fmt.Errorf("record %s not found in IONOS zone", providerID)
}

func (p *IONOSProvider) DeleteRecord(ctx context.Context, creds Credentials, providerID string) error {
	url := fmt.Sprintf("%s/zones/%s/records/%s", ionosBase, creds.ZoneID, providerID)
	slog.Info("deleting record", "component", "dns", "provider", "ionos", "record_id", providerID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)

	resp, err := ionosClient.Do(req)
	if err != nil {
		slog.Error("DeleteRecord request failed", "component", "dns", "provider", "ionos", "error", err)
		return fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info("DeleteRecord response received", "component", "dns", "provider", "ionos", "status", resp.StatusCode)

	if resp.StatusCode == 404 {
		// Already deleted at the provider — treat as success (idempotent)
		slog.Warn("DeleteRecord record already gone, treating as success", "component", "dns", "provider", "ionos", "record_id", providerID)
		return nil
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("IONOS API returned status %d", resp.StatusCode)
	}
	return nil
}
