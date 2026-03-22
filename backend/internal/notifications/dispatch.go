package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/email"
	"github.com/proxera/backend/internal/models"
)

// Dispatch sends an alert through all channels linked to a rule, respecting cooldown.
func Dispatch(ctx context.Context, userID int, ruleID int, cooldownMinutes int, alert models.AlertPayload) {
	// Build cooldown key from metadata qualifier
	// For crowdsec_ban, use IP as qualifier so each banned IP gets its own alert
	qualifier := ""
	metaKey := ""
	if v, ok := alert.Metadata["ip"]; ok {
		qualifier, _ = v.(string)
		metaKey = "ip"
	} else if v, ok := alert.Metadata["agent_id"]; ok {
		qualifier, _ = v.(string)
		metaKey = "agent_id"
	} else if v, ok := alert.Metadata["domain"]; ok {
		qualifier, _ = v.(string)
		metaKey = "domain"
	} else if v, ok := alert.Metadata["cert_id"]; ok {
		qualifier = fmt.Sprintf("%v", v)
		metaKey = "cert_id"
	}

	// Skip if there's already an unresolved alert for the same type+qualifier
	if qualifier != "" {
		if metaKey != "" {
			var existing int
			database.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM alert_history
				WHERE user_id = $1 AND alert_type = $2 AND resolved = false
				  AND metadata->>$3 = $4
			`, userID, alert.AlertType, metaKey, qualifier).Scan(&existing)
			if existing > 0 {
				return
			}
		}
	}

	key := CooldownKey(userID, alert.AlertType, qualifier)
	if CheckAndSetCooldown(key, cooldownMinutes) {
		return
	}

	// Record in alert history
	metadataJSON, _ := json.Marshal(alert.Metadata)
	var historyID int64
	err := database.DB.QueryRow(ctx, `
		INSERT INTO alert_history (user_id, rule_id, alert_type, severity, title, message, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, userID, ruleID, alert.AlertType, alert.Severity, alert.Title, alert.Message, metadataJSON,
	).Scan(&historyID)
	if err != nil {
		slog.Error("Failed to insert alert history", "component", "notifications", "alert_type", alert.AlertType, "user_id", userID, "error", err)
		return
	}

	// Update last_triggered_at on the rule
	database.DB.Exec(ctx, `UPDATE alert_rules SET last_triggered_at = $1 WHERE id = $2`, time.Now(), ruleID)

	// Fetch linked channels
	rows, err := database.DB.Query(ctx, `
		SELECT nc.id, nc.channel_type, nc.config
		FROM notification_channels nc
		JOIN alert_rule_channels arc ON arc.channel_id = nc.id
		WHERE arc.rule_id = $1 AND nc.enabled = true
	`, ruleID)
	if err != nil {
		slog.Error("Failed to fetch channels for rule", "component", "notifications", "rule_id", ruleID, "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var chID int
		var chType string
		var chConfig json.RawMessage
		if err := rows.Scan(&chID, &chType, &chConfig); err != nil {
			slog.Error("Failed to scan channel", "component", "notifications", "error", err)
			continue
		}

		switch chType {
		case "email":
			var cfg struct {
				Address string `json:"address"`
			}
			if err := json.Unmarshal(chConfig, &cfg); err != nil || cfg.Address == "" {
				slog.Warn("Invalid email channel config", "component", "notifications", "channel_type", "email", "channel_id", chID)
				continue
			}
			if err := email.SendAlertEmail(cfg.Address, alert); err != nil {
				slog.Error("Failed to send alert email", "component", "notifications", "channel_type", "email", "address", cfg.Address, "error", err)
			}

		case "webhook":
			var cfg struct {
				URL     string            `json:"url"`
				Method  string            `json:"method"`
				Headers map[string]string `json:"headers"`
			}
			if err := json.Unmarshal(chConfig, &cfg); err != nil || cfg.URL == "" {
				slog.Warn("Invalid webhook channel config", "component", "notifications", "channel_type", "webhook", "channel_id", chID)
				continue
			}
			if err := SendWebhook(cfg.URL, cfg.Method, cfg.Headers, alert); err != nil {
				slog.Error("Failed to send webhook", "component", "notifications", "channel_type", "webhook", "url", cfg.URL, "error", err)
			}

		case "discord":
			var cfg struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(chConfig, &cfg); err != nil || cfg.URL == "" {
				slog.Warn("Invalid discord channel config", "component", "notifications", "channel_type", "discord", "channel_id", chID)
				continue
			}
			if err := SendDiscordWebhook(cfg.URL, alert); err != nil {
				slog.Error("Failed to send discord webhook", "component", "notifications", "channel_type", "discord", "url", cfg.URL, "error", err)
			}
		}
	}

	slog.Info("Alert dispatched", "component", "notifications", "alert_type", alert.AlertType, "severity", alert.Severity, "title", alert.Title, "user_id", userID, "history_id", historyID)
}

// Resolve marks open alerts as resolved.
func Resolve(ctx context.Context, userID int, alertType, metadataKey, metadataValue string) {
	_, err := database.DB.Exec(ctx, `
		UPDATE alert_history SET resolved = true, resolved_at = NOW()
		WHERE user_id = $1 AND alert_type = $2 AND resolved = false
		  AND metadata->>$3 = $4
	`, userID, alertType, metadataKey, metadataValue)
	if err != nil {
		slog.Error("Failed to resolve alerts", "component", "notifications", "alert_type", alertType, "user_id", userID, "error", err)
	}
}
