package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/email"
	"github.com/proxera/backend/internal/models"
	"github.com/proxera/backend/internal/notifications"
)

// --- Alert Rules ---

// ListAlertRules returns the user's alert rules with linked channel IDs.
func ListAlertRules(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, alert_type, name, config, enabled, cooldown_minutes, last_triggered_at, created_at, updated_at
		FROM alert_rules
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch alert rules"})
	}
	defer rows.Close()

	var rules []models.AlertRule
	for rows.Next() {
		var r models.AlertRule
		r.UserID = userID
		if err := rows.Scan(&r.ID, &r.AlertType, &r.Name, &r.Config, &r.Enabled,
			&r.CooldownMinutes, &r.LastTriggeredAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}

		// Fetch linked channel IDs
		chRows, err := database.DB.Query(ctx,
			`SELECT channel_id FROM alert_rule_channels WHERE rule_id = $1`, r.ID)
		if err == nil {
			for chRows.Next() {
				var chID int
				if chRows.Scan(&chID) == nil {
					r.ChannelIDs = append(r.ChannelIDs, chID)
				}
			}
			chRows.Close()
		}
		if r.ChannelIDs == nil {
			r.ChannelIDs = []int{}
		}
		rules = append(rules, r)
	}
	if rules == nil {
		rules = []models.AlertRule{}
	}

	return c.JSON(fiber.Map{"rules": rules})
}

// CreateAlertRule creates a new alert rule.
func CreateAlertRule(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var body struct {
		AlertType       string          `json:"alert_type"`
		Name            string          `json:"name"`
		Config          json.RawMessage `json:"config"`
		CooldownMinutes *int            `json:"cooldown_minutes"`
		ChannelIDs      []int           `json:"channel_ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	validTypes := map[string]bool{
		"agent_offline": true, "cert_expiry": true,
		"cert_renewal_failed": true, "error_rate": true,
		"high_latency": true, "traffic_spike": true,
		"host_down": true, "bandwidth_threshold": true,
	}
	if !validTypes[body.AlertType] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid alert_type"})
	}
	if body.Name == "" || len(body.Name) > 255 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name is required (max 255 chars)"})
	}
	if body.Config == nil {
		body.Config = json.RawMessage(`{}`)
	}
	cooldown := 5
	if body.CooldownMinutes != nil {
		cooldown = *body.CooldownMinutes
	}
	if cooldown < 1 {
		cooldown = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify channel IDs belong to this user
	if len(body.ChannelIDs) > 0 {
		var count int
		database.DB.QueryRow(ctx,
			`SELECT COUNT(*) FROM notification_channels WHERE user_id = $1 AND id = ANY($2)`,
			userID, body.ChannelIDs).Scan(&count)
		if count != len(body.ChannelIDs) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "One or more channel IDs are invalid"})
		}
	}

	var ruleID int
	err := database.DB.QueryRow(ctx, `
		INSERT INTO alert_rules (user_id, alert_type, name, config, cooldown_minutes)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, userID, body.AlertType, body.Name, body.Config, cooldown).Scan(&ruleID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create alert rule"})
	}

	// Link channels
	for _, chID := range body.ChannelIDs {
		database.DB.Exec(ctx,
			`INSERT INTO alert_rule_channels (rule_id, channel_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			ruleID, chID)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": ruleID, "message": "Alert rule created"})
}

// UpdateAlertRule updates an existing alert rule.
func UpdateAlertRule(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	ruleID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid rule ID"})
	}

	var body struct {
		Name            *string          `json:"name"`
		Config          *json.RawMessage `json:"config"`
		Enabled         *bool            `json:"enabled"`
		CooldownMinutes *int             `json:"cooldown_minutes"`
		ChannelIDs      *[]int           `json:"channel_ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify ownership
	var ownerID int
	err = database.DB.QueryRow(ctx, `SELECT user_id FROM alert_rules WHERE id = $1`, ruleID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Alert rule not found"})
	}

	// Build a single UPDATE query for all changed fields
	setClauses := []string{"updated_at = NOW()"}
	updateArgs := []any{}
	updateIdx := 1

	if body.Name != nil {
		if *body.Name == "" || len(*body.Name) > 255 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name must be 1-255 chars"})
		}
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", updateIdx))
		updateArgs = append(updateArgs, *body.Name)
		updateIdx++
	}
	if body.Config != nil {
		setClauses = append(setClauses, fmt.Sprintf("config = $%d", updateIdx))
		updateArgs = append(updateArgs, *body.Config)
		updateIdx++
	}
	if body.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", updateIdx))
		updateArgs = append(updateArgs, *body.Enabled)
		updateIdx++
	}
	if body.CooldownMinutes != nil {
		cd := *body.CooldownMinutes
		if cd < 1 {
			cd = 1
		}
		setClauses = append(setClauses, fmt.Sprintf("cooldown_minutes = $%d", updateIdx))
		updateArgs = append(updateArgs, cd)
		updateIdx++
	}

	if len(updateArgs) > 0 {
		updateArgs = append(updateArgs, ruleID)
		updateQuery := fmt.Sprintf("UPDATE alert_rules SET %s WHERE id = $%d",
			strings.Join(setClauses, ", "), updateIdx)
		if _, err := database.DB.Exec(ctx, updateQuery, updateArgs...); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update alert rule"})
		}
	}

	if body.ChannelIDs != nil {
		// Verify channel IDs belong to user
		if len(*body.ChannelIDs) > 0 {
			var count int
			if err := database.DB.QueryRow(ctx,
				`SELECT COUNT(*) FROM notification_channels WHERE user_id = $1 AND id = ANY($2)`,
				userID, *body.ChannelIDs).Scan(&count); err != nil || count != len(*body.ChannelIDs) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "One or more channel IDs are invalid"})
			}
		}
		if _, err := database.DB.Exec(ctx, `DELETE FROM alert_rule_channels WHERE rule_id = $1`, ruleID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update channel links"})
		}
		for _, chID := range *body.ChannelIDs {
			database.DB.Exec(ctx,
				`INSERT INTO alert_rule_channels (rule_id, channel_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				ruleID, chID)
		}
	}

	return c.JSON(fiber.Map{"message": "Alert rule updated"})
}

// DeleteAlertRule deletes an alert rule.
func DeleteAlertRule(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	ruleID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid rule ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := database.DB.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1 AND user_id = $2`, ruleID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Alert rule not found"})
	}

	return c.JSON(fiber.Map{"message": "Alert rule deleted"})
}

// --- Notification Channels ---

// ListChannels returns the user's notification channels.
func ListChannels(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, name, channel_type, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch channels"})
	}
	defer rows.Close()

	var channels []models.NotificationChannel
	for rows.Next() {
		var ch models.NotificationChannel
		ch.UserID = userID
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.ChannelType, &ch.Config, &ch.Enabled,
			&ch.CreatedAt, &ch.UpdatedAt); err != nil {
			continue
		}
		channels = append(channels, ch)
	}
	if channels == nil {
		channels = []models.NotificationChannel{}
	}

	return c.JSON(fiber.Map{"channels": channels})
}

// CreateChannel creates a new notification channel.
func CreateChannel(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var body struct {
		Name        string          `json:"name"`
		ChannelType string          `json:"channel_type"`
		Config      json.RawMessage `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if body.Name == "" || len(body.Name) > 255 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name is required (max 255 chars)"})
	}
	if body.ChannelType != "email" && body.ChannelType != "webhook" && body.ChannelType != "discord" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "channel_type must be 'email', 'webhook', or 'discord'"})
	}
	if body.Config == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "config is required"})
	}

	// Validate config
	switch body.ChannelType {
	case "email":
		var cfg struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(body.Config, &cfg); err != nil || cfg.Address == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email channel requires config.address"})
		}
	case "webhook":
		var cfg struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body.Config, &cfg); err != nil || cfg.URL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Webhook channel requires config.url"})
		}
	case "discord":
		var cfg struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body.Config, &cfg); err != nil || cfg.URL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Discord channel requires config.url"})
		}
		if !strings.HasPrefix(cfg.URL, "https://discord.com/api/webhooks/") && !strings.HasPrefix(cfg.URL, "https://canary.discord.com/api/webhooks/") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Discord webhook URL"})
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var channelID int
	err := database.DB.QueryRow(ctx, `
		INSERT INTO notification_channels (user_id, name, channel_type, config)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, userID, body.Name, body.ChannelType, body.Config).Scan(&channelID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create channel"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": channelID, "message": "Channel created"})
}

// UpdateChannel updates a notification channel.
func UpdateChannel(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	channelID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid channel ID"})
	}

	var body struct {
		Name    *string          `json:"name"`
		Config  *json.RawMessage `json:"config"`
		Enabled *bool            `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var ownerID int
	err = database.DB.QueryRow(ctx, `SELECT user_id FROM notification_channels WHERE id = $1`, channelID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Channel not found"})
	}

	if body.Name != nil {
		database.DB.Exec(ctx, `UPDATE notification_channels SET name = $1, updated_at = NOW() WHERE id = $2`, *body.Name, channelID)
	}
	if body.Config != nil {
		database.DB.Exec(ctx, `UPDATE notification_channels SET config = $1, updated_at = NOW() WHERE id = $2`, *body.Config, channelID)
	}
	if body.Enabled != nil {
		database.DB.Exec(ctx, `UPDATE notification_channels SET enabled = $1, updated_at = NOW() WHERE id = $2`, *body.Enabled, channelID)
	}

	return c.JSON(fiber.Map{"message": "Channel updated"})
}

// DeleteChannel deletes a notification channel.
func DeleteChannel(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	channelID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid channel ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := database.DB.Exec(ctx, `DELETE FROM notification_channels WHERE id = $1 AND user_id = $2`, channelID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Channel not found"})
	}

	return c.JSON(fiber.Map{"message": "Channel deleted"})
}

// TestChannel sends a test notification through a channel.
func TestChannel(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	channelID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid channel ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var chType string
	var chConfig json.RawMessage
	var ownerID int
	err = database.DB.QueryRow(ctx,
		`SELECT user_id, channel_type, config FROM notification_channels WHERE id = $1`, channelID,
	).Scan(&ownerID, &chType, &chConfig)
	if err != nil || ownerID != userID {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Channel not found"})
	}

	testAlert := models.AlertPayload{
		AlertType: "test",
		Severity:  "info",
		Title:     "Test Notification from Proxera",
		Message:   "This is a test notification. Your channel is configured correctly.",
		Metadata:  map[string]any{"test": true},
	}

	switch chType {
	case "email":
		var cfg struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(chConfig, &cfg); err != nil || cfg.Address == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid email channel config"})
		}
		if err := email.SendAlertEmail(cfg.Address, testAlert); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test email: " + err.Error()})
		}

	case "webhook":
		var cfg struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
		}
		if err := json.Unmarshal(chConfig, &cfg); err != nil || cfg.URL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid webhook channel config"})
		}
		if err := notifications.SendWebhook(cfg.URL, cfg.Method, cfg.Headers, testAlert); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Webhook test failed: " + err.Error()})
		}

	case "discord":
		var cfg struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(chConfig, &cfg); err != nil || cfg.URL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid discord channel config"})
		}
		if err := notifications.SendDiscordWebhook(cfg.URL, testAlert); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Discord webhook test failed: " + err.Error()})
		}
	}

	return c.JSON(fiber.Map{"message": "Test notification sent"})
}

// --- Alert History ---

// ListAlertHistory returns paginated alert history for the user.
func ListAlertHistory(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	alertType := c.Query("type", "")
	severity := c.Query("severity", "")
	resolvedParam := c.Query("resolved", "")

	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	whereClause := ` WHERE user_id = $1`
	args := []any{userID}
	argIdx := 2

	if alertType != "" {
		whereClause += ` AND alert_type = $` + strconv.Itoa(argIdx)
		args = append(args, alertType)
		argIdx++
	}
	if severity != "" {
		whereClause += ` AND severity = $` + strconv.Itoa(argIdx)
		args = append(args, severity)
		argIdx++
	}
	if resolvedParam == "true" {
		whereClause += ` AND resolved = true`
	} else if resolvedParam == "false" {
		whereClause += ` AND resolved = false`
	}

	// Get total count with same filters
	var total int
	countQuery := `SELECT COUNT(*) FROM alert_history` + whereClause
	database.DB.QueryRow(ctx, countQuery, args...).Scan(&total)

	query := `SELECT id, rule_id, alert_type, severity, title, message, metadata, resolved, resolved_at, created_at
		FROM alert_history` + whereClause +
		` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := database.DB.Query(ctx, query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch alert history"})
	}
	defer rows.Close()

	var entries []models.AlertHistoryEntry
	for rows.Next() {
		var e models.AlertHistoryEntry
		e.UserID = userID
		if err := rows.Scan(&e.ID, &e.RuleID, &e.AlertType, &e.Severity, &e.Title,
			&e.Message, &e.Metadata, &e.Resolved, &e.ResolvedAt, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []models.AlertHistoryEntry{}
	}

	return c.JSON(fiber.Map{
		"alerts": entries,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ResolveAlert manually resolves an alert.
func ResolveAlert(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	alertID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid alert ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := database.DB.Exec(ctx, `
		UPDATE alert_history SET resolved = true, resolved_at = NOW()
		WHERE id = $1 AND user_id = $2 AND resolved = false
	`, alertID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Alert not found or already resolved"})
	}

	return c.JSON(fiber.Map{"message": "Alert resolved"})
}

// --- Quick Setup ---

// QuickSetupAlerts creates default alert rules (and an email channel if none exist) for the user.
// Skips any alert types that already have rules configured.
func QuickSetupAlerts(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find existing rule types so we don't duplicate
	existingTypes := make(map[string]bool)
	rows, err := database.DB.Query(ctx, `SELECT alert_type FROM alert_rules WHERE user_id = $1`, userID)
	if err == nil {
		for rows.Next() {
			var t string
			if rows.Scan(&t) == nil {
				existingTypes[t] = true
			}
		}
		rows.Close()
	}

	// Get or create a channel to link rules to
	var channelIDs []int
	chRows, err := database.DB.Query(ctx, `SELECT id FROM notification_channels WHERE user_id = $1 AND enabled = true ORDER BY created_at LIMIT 5`, userID)
	if err == nil {
		for chRows.Next() {
			var id int
			if chRows.Scan(&id) == nil {
				channelIDs = append(channelIDs, id)
			}
		}
		chRows.Close()
	}

	// If no channels exist, create an email channel
	if len(channelIDs) == 0 {
		var userEmail string
		err := database.DB.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&userEmail)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch user email"})
		}

		emailConfig, _ := json.Marshal(map[string]string{"address": userEmail})
		var channelID int
		err = database.DB.QueryRow(ctx, `
			INSERT INTO notification_channels (user_id, name, channel_type, config)
			VALUES ($1, 'Email Alerts', 'email', $2) RETURNING id
		`, userID, emailConfig).Scan(&channelID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create email channel"})
		}
		channelIDs = []int{channelID}
	}

	// Create default rules (skip types that already exist)
	defaultRules := []struct {
		alertType string
		name      string
		config    string
	}{
		{"agent_offline", "Agent offline alert", `{"agent_ids":["all"]}`},
		{"cert_expiry", "Certificate expiry alert", `{"warn_days":[30,7,1]}`},
		{"cert_renewal_failed", "Certificate renewal failure", `{}`},
		{"error_rate", "High error rate alert", `{"threshold_pct":5,"domains":["all"],"window_minutes":5}`},
		{"high_latency", "High latency alert", `{"threshold_ms":500,"domains":["all"],"window_minutes":1}`},
		{"traffic_spike", "Traffic spike alert", `{"multiplier":3,"baseline_minutes":60,"domains":["all"]}`},
		{"host_down", "Host down alert", `{"domains":["all"],"window_minutes":1}`},
		{"bandwidth_threshold", "Bandwidth threshold alert", `{"threshold_gb":10,"period_hours":1,"domains":["all"]}`},
	}

	created := 0
	for _, dr := range defaultRules {
		if existingTypes[dr.alertType] {
			continue
		}
		var ruleID int
		err := database.DB.QueryRow(ctx, `
			INSERT INTO alert_rules (user_id, alert_type, name, config, cooldown_minutes)
			VALUES ($1, $2, $3, $4, 5) RETURNING id
		`, userID, dr.alertType, dr.name, json.RawMessage(dr.config)).Scan(&ruleID)
		if err != nil {
			continue
		}
		for _, chID := range channelIDs {
			database.DB.Exec(ctx,
				`INSERT INTO alert_rule_channels (rule_id, channel_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				ruleID, chID)
		}
		created++
	}

	if created == 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "All alert types already have rules configured"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": fmt.Sprintf("Created %d alert rules", created),
	})
}
