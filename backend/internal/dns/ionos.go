package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	log.Printf("[IONOS] GET %s (VerifyZone domain=%q)", url, creds.Domain)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		log.Printf("[IONOS] VerifyZone request failed: %v", err)
		return "", "", fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[IONOS] VerifyZone response: %d", resp.StatusCode)

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
	log.Printf("[IONOS] VerifyZone: found %d zones in account", len(zones))

	want := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(creds.Domain), "."))
	for _, z := range zones {
		if strings.ToLower(z.Name) == want {
			log.Printf("[IONOS] VerifyZone: matched zone %q -> id=%s", z.Name, z.ID)
			return z.Name, z.ID, nil
		}
	}
	return "", "", fmt.Errorf("domain %q not found in your IONOS account", creds.Domain)
}

func (p *IONOSProvider) ListRecords(ctx context.Context, creds Credentials) ([]Record, error) {
	url := fmt.Sprintf("%s/zones/%s", ionosBase, creds.ZoneID)
	log.Printf("[IONOS] GET %s (ListRecords)", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		log.Printf("[IONOS] ListRecords request failed: %v", err)
		return nil, fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[IONOS] ListRecords response: %d", resp.StatusCode)

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
	log.Printf("[IONOS] ListRecords: returned %d records (of %d total, disabled excluded)", len(records), len(zone.Records))
	return records, nil
}

func (p *IONOSProvider) CreateRecord(ctx context.Context, creds Credentials, r RecordInput) (Record, error) {
	url := fmt.Sprintf("%s/zones/%s/records", ionosBase, creds.ZoneID)
	log.Printf("[IONOS] POST %s (CreateRecord type=%s name=%s content=%s)", url, r.Type, r.Name, r.Content)

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
		log.Printf("[IONOS] CreateRecord request failed: %v", err)
		return Record{}, fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[IONOS] CreateRecord response: %d", resp.StatusCode)

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		errBody, _ := io.ReadAll(resp.Body)
		log.Printf("[IONOS] CreateRecord error body: %s", string(errBody))
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
	log.Printf("[IONOS] CreateRecord: created record id=%s name=%s", created[0].ID, created[0].Name)
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
	log.Printf("[IONOS] PUT %s (UpdateRecord type=%s name=%s content=%s)", url, r.Type, r.Name, r.Content)

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
		log.Printf("[IONOS] UpdateRecord request failed: %v", err)
		return Record{}, fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[IONOS] UpdateRecord response: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		log.Printf("[IONOS] UpdateRecord error body: %s", string(errBody))
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
		log.Printf("[IONOS] UpdateRecord: empty response body, using input values")
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
	log.Printf("[IONOS] GET %s (PatchContent pre-fetch for record %s)", zoneURL, providerID)

	req, err := http.NewRequestWithContext(ctx, "GET", zoneURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := ionosClient.Do(req)
	if err != nil {
		log.Printf("[IONOS] PatchContent pre-fetch failed: %v", err)
		return fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[IONOS] PatchContent pre-fetch response: %d", resp.StatusCode)

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
			log.Printf("[IONOS] PatchContent: updating record %s (%s %s) content %q -> %q", providerID, r.Type, r.Name, r.Content, content)
			_, err = p.UpdateRecord(ctx, creds, providerID, RecordInput{
				Name:    r.Name,
				Type:    r.Type,
				Content: content,
				TTL:     r.TTL,
			})
			return err
		}
	}
	log.Printf("[IONOS] PatchContent: record %s not found in zone (zone has %d records)", providerID, len(zone.Records))
	return fmt.Errorf("record %s not found in IONOS zone", providerID)
}

func (p *IONOSProvider) DeleteRecord(ctx context.Context, creds Credentials, providerID string) error {
	url := fmt.Sprintf("%s/zones/%s/records/%s", ionosBase, creds.ZoneID, providerID)
	log.Printf("[IONOS] DELETE %s (DeleteRecord)", url)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-Key", creds.APIKey)

	resp, err := ionosClient.Do(req)
	if err != nil {
		log.Printf("[IONOS] DeleteRecord request failed: %v", err)
		return fmt.Errorf("failed to reach IONOS API: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[IONOS] DeleteRecord response: %d", resp.StatusCode)

	if resp.StatusCode == 404 {
		// Already deleted at the provider — treat as success (idempotent)
		log.Printf("[IONOS] DeleteRecord: record %s already gone (404), treating as success", providerID)
		return nil
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("IONOS API returned status %d", resp.StatusCode)
	}
	return nil
}
