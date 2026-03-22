package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var cfClient = &http.Client{Timeout: 15 * time.Second}

// CloudflareProvider implements Provider for Cloudflare DNS.
type CloudflareProvider struct{}

func (cf *CloudflareProvider) VerifyZone(ctx context.Context, creds Credentials) (string, string, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s", creds.ZoneID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cfClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to reach Cloudflare API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var cfResp struct {
		Success bool `json:"success"`
		Result  struct {
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfResp); err != nil {
		return "", "", fmt.Errorf("failed to parse Cloudflare response: %w", err)
	}
	if !cfResp.Success {
		if len(cfResp.Errors) > 0 {
			return "", "", fmt.Errorf("%s", cfResp.Errors[0].Message)
		}
		return "", "", fmt.Errorf("invalid Zone ID or API key")
	}
	if cfResp.Result.Name == "" {
		return "", "", fmt.Errorf("zone has no domain name")
	}
	return cfResp.Result.Name, creds.ZoneID, nil
}

func (cf *CloudflareProvider) ListRecords(ctx context.Context, creds Credentials) ([]Record, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?per_page=5000", creds.ZoneID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cfClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach Cloudflare API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var cfResp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
			TTL     int    `json:"ttl"`
			Proxied bool   `json:"proxied"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfResp); err != nil {
		return nil, fmt.Errorf("failed to parse Cloudflare response: %w", err)
	}
	if !cfResp.Success {
		if len(cfResp.Errors) > 0 {
			return nil, fmt.Errorf("%s", cfResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare API returned an error")
	}

	records := make([]Record, 0, len(cfResp.Result))
	for _, r := range cfResp.Result {
		records = append(records, Record{
			ProviderID: r.ID,
			Type:       r.Type,
			Name:       r.Name,
			Content:    r.Content,
			TTL:        r.TTL,
			Proxied:    r.Proxied,
		})
	}
	return records, nil
}

func (cf *CloudflareProvider) CreateRecord(ctx context.Context, creds Credentials, r RecordInput) (Record, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", creds.ZoneID)
	body := map[string]interface{}{
		"type":    r.Type,
		"name":    r.Name,
		"content": r.Content,
		"ttl":     r.TTL,
		"proxied": r.Proxied,
	}

	result, err := cf.request(ctx, "POST", url, creds.APIKey, body)
	if err != nil {
		return Record{}, err
	}

	var cfRec struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
		Proxied bool   `json:"proxied"`
	}
	if err := json.Unmarshal(result, &cfRec); err != nil {
		return Record{}, fmt.Errorf("failed to parse created record: %w", err)
	}
	return Record{
		ProviderID: cfRec.ID,
		Type:       cfRec.Type,
		Name:       cfRec.Name,
		Content:    cfRec.Content,
		TTL:        cfRec.TTL,
		Proxied:    cfRec.Proxied,
	}, nil
}

func (cf *CloudflareProvider) UpdateRecord(ctx context.Context, creds Credentials, providerID string, r RecordInput) (Record, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", creds.ZoneID, providerID)
	body := map[string]interface{}{
		"type":    r.Type,
		"name":    r.Name,
		"content": r.Content,
		"ttl":     r.TTL,
		"proxied": r.Proxied,
	}

	result, err := cf.request(ctx, "PATCH", url, creds.APIKey, body)
	if err != nil {
		return Record{}, err
	}

	var cfRec struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
		Proxied bool   `json:"proxied"`
	}
	if err := json.Unmarshal(result, &cfRec); err != nil {
		return Record{}, fmt.Errorf("failed to parse updated record: %w", err)
	}
	return Record{
		ProviderID: providerID,
		Type:       cfRec.Type,
		Name:       cfRec.Name,
		Content:    cfRec.Content,
		TTL:        cfRec.TTL,
		Proxied:    cfRec.Proxied,
	}, nil
}

func (cf *CloudflareProvider) PatchContent(ctx context.Context, creds Credentials, providerID string, content string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", creds.ZoneID, providerID)
	_, err := cf.request(ctx, "PATCH", url, creds.APIKey, map[string]interface{}{"content": content})
	return err
}

func (cf *CloudflareProvider) DeleteRecord(ctx context.Context, creds Credentials, providerID string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", creds.ZoneID, providerID)
	_, err := cf.request(ctx, "DELETE", url, creds.APIKey, nil)
	return err
}

// request makes an authenticated API call and returns the parsed result field.
func (cf *CloudflareProvider) request(ctx context.Context, method, url, apiKey string, body interface{}) (json.RawMessage, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cfClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var cfResp struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &cfResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if !cfResp.Success {
		if len(cfResp.Errors) > 0 {
			return nil, fmt.Errorf("%s", cfResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare API returned an error")
	}
	return cfResp.Result, nil
}
