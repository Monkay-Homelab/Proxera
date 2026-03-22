package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/proxera/backend/internal/models"
)

var webhookClient = &http.Client{Timeout: 10 * time.Second}

// SendWebhook delivers an alert payload to a generic webhook URL.
func SendWebhook(url, method string, headers map[string]string, alert models.AlertPayload) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("webhook URL must use HTTPS")
	}
	if method == "" {
		method = "POST"
	}

	payload := map[string]any{
		"alert_type": alert.AlertType,
		"severity":   alert.Severity,
		"title":      alert.Title,
		"message":    alert.Message,
		"metadata":   alert.Metadata,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := webhookClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("Webhook non-2xx response", "component", "notifications", "channel_type", "webhook", "status_code", resp.StatusCode, "url", url)
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	return nil
}

// discordColor maps alert severity to Discord embed colors.
func discordColor(severity string) int {
	switch severity {
	case "critical":
		return 0xED4245 // Discord red
	case "warning":
		return 0xFEE75C // Discord yellow
	default:
		return 0x5865F2 // Discord blurple (info/test)
	}
}

// SendDiscordWebhook delivers an alert as a Discord embed.
func SendDiscordWebhook(webhookURL string, alert models.AlertPayload) error {
	if !strings.HasPrefix(webhookURL, "https://") {
		return fmt.Errorf("discord webhook URL must use HTTPS")
	}

	// Build embed fields from metadata
	fields := []map[string]any{
		{"name": "Type", "value": alert.AlertType, "inline": true},
		{"name": "Severity", "value": alert.Severity, "inline": true},
	}
	for k, v := range alert.Metadata {
		if k == "test" {
			continue
		}
		fields = append(fields, map[string]any{
			"name":   k,
			"value":  fmt.Sprintf("%v", v),
			"inline": true,
		})
	}

	payload := map[string]any{
		"embeds": []map[string]any{
			{
				"title":       alert.Title,
				"description": alert.Message,
				"color":       discordColor(alert.Severity),
				"fields":      fields,
				"footer":      map[string]any{"text": "Proxera Alerts"},
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := webhookClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()

	// Discord returns 204 No Content on success
	if resp.StatusCode != 204 && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		slog.Warn("Discord webhook non-success response", "component", "notifications", "channel_type", "discord", "status_code", resp.StatusCode)
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}

	return nil
}
