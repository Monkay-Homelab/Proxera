package models

import (
	"encoding/json"
	"time"
)

type AlertRule struct {
	ID              int             `json:"id"`
	UserID          int             `json:"user_id"`
	AlertType       string          `json:"alert_type"`
	Name            string          `json:"name"`
	Config          json.RawMessage `json:"config"`
	Enabled         bool            `json:"enabled"`
	CooldownMinutes int             `json:"cooldown_minutes"`
	LastTriggeredAt *time.Time      `json:"last_triggered_at"`
	ChannelIDs      []int           `json:"channel_ids,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type NotificationChannel struct {
	ID          int             `json:"id"`
	UserID      int             `json:"user_id"`
	Name        string          `json:"name"`
	ChannelType string          `json:"channel_type"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type AlertHistoryEntry struct {
	ID         int64           `json:"id"`
	UserID     int             `json:"user_id"`
	RuleID     *int            `json:"rule_id"`
	AlertType  string          `json:"alert_type"`
	Severity   string          `json:"severity"`
	Title      string          `json:"title"`
	Message    string          `json:"message"`
	Metadata   json.RawMessage `json:"metadata"`
	Resolved   bool            `json:"resolved"`
	ResolvedAt *time.Time      `json:"resolved_at"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AlertPayload is the internal representation passed to the notification dispatcher.
type AlertPayload struct {
	AlertType string         `json:"alert_type"`
	Severity  string         `json:"severity"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
}
