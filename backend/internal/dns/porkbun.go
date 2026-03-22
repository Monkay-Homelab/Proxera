package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var porkbunClient = &http.Client{Timeout: 15 * time.Second}

const porkbunBase = "https://api.porkbun.com/api/json/v3"

// PorkbunProvider implements Provider for Porkbun DNS.
//
// Quirks vs other providers:
//   - Auth credentials go in the POST body (not a header) on every request.
//   - All operations use POST, even reads.
//   - Record names are subdomain-only (e.g. "www"), not FQDN — we convert on the way in/out.
//   - TTL is returned as a string, not an integer.
//   - zoneID == domain name (no separate UUID).
type PorkbunProvider struct{}

// auth returns the base body map with credentials embedded.
func (p *PorkbunProvider) auth(creds Credentials) map[string]interface{} {
	return map[string]interface{}{
		"apikey":       creds.APIKey,
		"secretapikey": creds.APISecret,
	}
}

// post sends an authenticated POST request and returns the raw response body.
func (p *PorkbunProvider) post(ctx context.Context, url string, body map[string]interface{}) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := porkbunClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach Porkbun API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	slog.Info("API response received", "component", "dns", "provider", "porkbun", "status", resp.StatusCode, "response_body", string(respBody))

	var base struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &base); err != nil {
		return nil, fmt.Errorf("failed to parse Porkbun response: %w", err)
	}
	if base.Status != "SUCCESS" {
		if base.Message != "" {
			return nil, fmt.Errorf("%s", base.Message)
		}
		return nil, fmt.Errorf("porkbun API returned status %q", base.Status)
	}
	return respBody, nil
}

// toFQDN converts a Porkbun subdomain name to a fully-qualified domain name.
// Porkbun returns subdomain-only names (e.g. "www") for most records, but
// returns the full domain name for apex/NS records (e.g. "example.com").
// Guard against double-appending in that case.
func toFQDN(name, domain string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "@" {
		return domain
	}
	// Already FQDN — Porkbun returns full name for apex NS and similar records
	if strings.HasSuffix(strings.ToLower(name), strings.ToLower(domain)) {
		return name
	}
	return name + "." + domain
}

// toSubdomain strips the domain suffix from an FQDN to get the subdomain Porkbun expects.
func toSubdomain(fqdn, domain string) string {
	fqdn = strings.TrimSuffix(strings.TrimSpace(fqdn), ".")
	if fqdn == "@" {
		return ""
	}
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if strings.EqualFold(fqdn, domain) {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(fqdn, "."+domain), "."+strings.ToLower(domain))
}

func (p *PorkbunProvider) VerifyZone(ctx context.Context, creds Credentials) (string, string, error) {
	pingURL := porkbunBase + "/ping"
	slog.Info("pinging API for credential check", "component", "dns", "provider", "porkbun", "operation", "VerifyZone")

	if _, err := p.post(ctx, pingURL, p.auth(creds)); err != nil {
		slog.Error("VerifyZone ping failed", "component", "dns", "provider", "porkbun", "error", err)
		return "", "", fmt.Errorf("credential check failed: %w", err)
	}
	slog.Info("VerifyZone ping succeeded", "component", "dns", "provider", "porkbun")

	// Verify the domain exists in the account
	retrieveURL := fmt.Sprintf("%s/dns/retrieve/%s", porkbunBase, creds.Domain)
	slog.Info("verifying domain ownership", "component", "dns", "provider", "porkbun", "domain", creds.Domain)

	if _, err := p.post(ctx, retrieveURL, p.auth(creds)); err != nil {
		slog.Error("VerifyZone domain check failed", "component", "dns", "provider", "porkbun", "domain", creds.Domain, "error", err)
		return "", "", fmt.Errorf("domain %q not found in Porkbun account: %w", creds.Domain, err)
	}
	slog.Info("VerifyZone domain verified", "component", "dns", "provider", "porkbun", "domain", creds.Domain)

	// For Porkbun, zoneID == domain name (used in all API paths)
	return creds.Domain, creds.Domain, nil
}

func (p *PorkbunProvider) ListRecords(ctx context.Context, creds Credentials) ([]Record, error) {
	url := fmt.Sprintf("%s/dns/retrieve/%s", porkbunBase, creds.Domain)
	slog.Info("listing records", "component", "dns", "provider", "porkbun", "domain", creds.Domain)

	raw, err := p.post(ctx, url, p.auth(creds))
	if err != nil {
		slog.Error("ListRecords failed", "component", "dns", "provider", "porkbun", "domain", creds.Domain, "error", err)
		return nil, err
	}

	var result struct {
		Records []struct {
			ID      string `json:"id"`
			Name    string `json:"name"` // subdomain only
			Type    string `json:"type"`
			Content string `json:"content"`
			TTL     string `json:"ttl"` // Porkbun returns TTL as string
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Porkbun records: %w", err)
	}

	records := make([]Record, 0, len(result.Records))
	for _, r := range result.Records {
		ttl, _ := strconv.Atoi(r.TTL)
		records = append(records, Record{
			ProviderID: r.ID,
			Type:       r.Type,
			Name:       toFQDN(r.Name, creds.Domain),
			Content:    r.Content,
			TTL:        ttl,
		})
	}
	slog.Info("ListRecords completed", "component", "dns", "provider", "porkbun", "record_count", len(records))
	return records, nil
}

func (p *PorkbunProvider) CreateRecord(ctx context.Context, creds Credentials, r RecordInput) (Record, error) {
	url := fmt.Sprintf("%s/dns/create/%s", porkbunBase, creds.Domain)
	ttl := r.TTL
	if ttl < 600 {
		ttl = 600 // Porkbun minimum TTL
	}
	slog.Info("creating record", "component", "dns", "provider", "porkbun", "record_type", r.Type, "name", r.Name, "content", r.Content, "ttl", ttl)

	body := p.auth(creds)
	body["name"] = toSubdomain(r.Name, creds.Domain)
	body["type"] = r.Type
	body["content"] = r.Content
	body["ttl"] = strconv.Itoa(ttl)

	raw, err := p.post(ctx, url, body)
	if err != nil {
		slog.Error("CreateRecord failed", "component", "dns", "provider", "porkbun", "error", err)
		return Record{}, err
	}

	// Porkbun returns "id" as either a string or an integer depending on the API version.
	var result struct {
		ID interface{} `json:"id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return Record{}, fmt.Errorf("failed to parse create response: %w", err)
	}
	var recordID string
	switch v := result.ID.(type) {
	case string:
		recordID = v
	case float64:
		recordID = strconv.FormatInt(int64(v), 10)
	default:
		return Record{}, fmt.Errorf("unexpected id type in Porkbun create response: %T", result.ID)
	}
	slog.Info("CreateRecord completed", "component", "dns", "provider", "porkbun", "record_id", recordID)

	return Record{
		ProviderID: recordID,
		Type:       r.Type,
		Name:       toFQDN(toSubdomain(r.Name, creds.Domain), creds.Domain),
		Content:    r.Content,
		TTL:        ttl,
	}, nil
}

func (p *PorkbunProvider) UpdateRecord(ctx context.Context, creds Credentials, providerID string, r RecordInput) (Record, error) {
	url := fmt.Sprintf("%s/dns/edit/%s/%s", porkbunBase, creds.Domain, providerID)
	ttl := r.TTL
	if ttl < 600 {
		ttl = 600 // Porkbun minimum TTL
	}
	slog.Info("updating record", "component", "dns", "provider", "porkbun", "record_id", providerID, "record_type", r.Type, "name", r.Name, "content", r.Content, "ttl", ttl)

	body := p.auth(creds)
	body["name"] = toSubdomain(r.Name, creds.Domain)
	body["type"] = r.Type
	body["content"] = r.Content
	body["ttl"] = strconv.Itoa(ttl)

	if _, err := p.post(ctx, url, body); err != nil {
		slog.Error("UpdateRecord failed", "component", "dns", "provider", "porkbun", "record_id", providerID, "error", err)
		return Record{}, err
	}
	slog.Info("UpdateRecord completed", "component", "dns", "provider", "porkbun", "record_id", providerID)

	return Record{
		ProviderID: providerID,
		Type:       r.Type,
		Name:       toFQDN(toSubdomain(r.Name, creds.Domain), creds.Domain),
		Content:    r.Content,
		TTL:        ttl,
	}, nil
}

func (p *PorkbunProvider) PatchContent(ctx context.Context, creds Credentials, providerID string, content string) error {
	// Fetch current record to get name/type/TTL required for the edit call.
	listURL := fmt.Sprintf("%s/dns/retrieve/%s", porkbunBase, creds.Domain)
	slog.Info("fetching records for PatchContent", "component", "dns", "provider", "porkbun", "record_id", providerID)

	raw, err := p.post(ctx, listURL, p.auth(creds))
	if err != nil {
		slog.Error("PatchContent pre-fetch failed", "component", "dns", "provider", "porkbun", "record_id", providerID, "error", err)
		return err
	}

	var result struct {
		Records []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Content string `json:"content"`
			TTL     string `json:"ttl"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("failed to parse Porkbun records: %w", err)
	}

	for _, r := range result.Records {
		if r.ID == providerID {
			ttl, _ := strconv.Atoi(r.TTL)
			slog.Info("PatchContent updating record", "component", "dns", "provider", "porkbun", "record_id", providerID, "record_type", r.Type, "name", r.Name, "old_content", r.Content, "new_content", content)
			_, err = p.UpdateRecord(ctx, creds, providerID, RecordInput{
				Name:    toFQDN(r.Name, creds.Domain),
				Type:    r.Type,
				Content: content,
				TTL:     ttl,
			})
			return err
		}
	}
	slog.Warn("PatchContent record not found", "component", "dns", "provider", "porkbun", "record_id", providerID, "zone_record_count", len(result.Records))
	return fmt.Errorf("record %s not found in Porkbun zone", providerID)
}

func (p *PorkbunProvider) DeleteRecord(ctx context.Context, creds Credentials, providerID string) error {
	url := fmt.Sprintf("%s/dns/delete/%s/%s", porkbunBase, creds.Domain, providerID)
	slog.Info("deleting record", "component", "dns", "provider", "porkbun", "record_id", providerID)

	if _, err := p.post(ctx, url, p.auth(creds)); err != nil {
		slog.Error("DeleteRecord failed", "component", "dns", "provider", "porkbun", "record_id", providerID, "error", err)
		return err
	}
	slog.Info("DeleteRecord completed", "component", "dns", "provider", "porkbun", "record_id", providerID)
	return nil
}
