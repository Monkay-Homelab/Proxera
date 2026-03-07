package models

import (
	"time"
)

// Agent represents a registered agent
type Agent struct {
	ID          int       `json:"id"`
	UserID      int       `json:"-"`
	AgentID     string    `json:"agent_id"`     // Unique identifier for the agent
	Name        string    `json:"name"`         // Friendly name set by user
	APIKey      string    `json:"-"`            // Authentication key (only exposed during registration)
	Status      string    `json:"status"`       // online, offline, error
	Version     string    `json:"version"`      // Agent version
	OS          string    `json:"os"`           // Operating system
	Arch        string    `json:"arch"`         // Architecture
	LastSeen    time.Time `json:"last_seen"`    // Last heartbeat
	IPAddress   string    `json:"ip_address"`   // Agent IP address (for backward compatibility)
	LanIP       string    `json:"lan_ip"`       // Local/LAN IP address
	WanIP       string    `json:"wan_ip"`       // Public/WAN IP address
	HostCount         int       `json:"host_count"`          // Number of hosts managed
	DNSRecordCount    int       `json:"dns_record_count"`    // Number of DNS records assigned
	MetricsInterval   int       `json:"metrics_interval"`    // Metrics collection interval in seconds
	CrowdSecInstalled bool      `json:"crowdsec_installed"`  // Whether CrowdSec is installed
	NginxVersion      string    `json:"nginx_version"`       // Nginx version (e.g. "1.28.2")
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// RegisterAgentRequest is the request to register a new agent
type RegisterAgentRequest struct {
	Name    string `json:"name"`    // Friendly name for the agent
	Version string `json:"version"` // Agent version
	OS      string `json:"os"`      // Operating system
	Arch    string `json:"arch"`    // Architecture
}

// RegisterAgentResponse is returned when registering an agent
type RegisterAgentResponse struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
	WSURL   string `json:"ws_url"`
}

// AgentCommand represents a command sent to an agent
type AgentCommand struct {
	Type    string                 `json:"type"`    // apply, reload, update, etc.
	Payload map[string]interface{} `json:"payload"` // Command-specific data
}

// AgentResponse is sent by the agent in response to commands
type AgentResponse struct {
	CommandType string `json:"command_type"`
	Success     bool   `json:"success"`
	Message     string `json:"message,omitempty"`
	Error       string `json:"error,omitempty"`
}
