package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/models"
	"github.com/proxera/backend/internal/settings"
)

// generateAgentID generates a unique agent identifier
func generateAgentID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("crypto/rand.Read failed: %w", err)
	}
	return "agent_" + hex.EncodeToString(bytes), nil
}

// generateAPIKey generates a secure API key
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("crypto/rand.Read failed: %w", err)
	}
	return "pk_" + hex.EncodeToString(bytes), nil
}

// HashAPIKey computes SHA-256 hash of an API key for secure storage.
// API keys are high-entropy (24+ random bytes), making SHA-256 appropriate
// and equivalent in security to bcrypt/argon2 for this use case.
func HashAPIKey(apiKey string) string {
	h := sha256.Sum256([]byte(apiKey)) // #nosec - SHA-256 is safe for high-entropy API keys
	return hex.EncodeToString(h[:])
}

// verifyAgentAccessByID checks that the authenticated user can access the given agent.
// Admins can access all agents. Owners and assigned users can access their agents.
func verifyAgentAccessByID(c *fiber.Ctx, agentID string, id *int) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	if role == "admin" {
		return database.DB.QueryRow(context.Background(),
			`SELECT id FROM agents WHERE agent_id = $1`, agentID).Scan(id)
	}
	return database.DB.QueryRow(context.Background(),
		`SELECT id FROM agents WHERE agent_id = $1 AND (user_id = $2 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $2))`,
		agentID, userID).Scan(id)
}

// markAgentOfflineIfStale returns "offline" if the agent hasn't sent a heartbeat in 90+ seconds
func markAgentOfflineIfStale(agent *models.Agent) {
	if time.Since(agent.LastSeen) > AgentOfflineThreshold {
		agent.Status = "offline"
	}
}

// RegisterAgent handles agent registration
func RegisterAgent(c *fiber.Ctx) error {
	// Get user ID from context (set by auth middleware)
	userID, _ := c.Locals("user_id").(int)

	var req models.RegisterAgentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Agent name is required",
		})
	}
	if len(req.Name) > 255 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Agent name must be 255 characters or less",
		})
	}

	// Generate agent credentials
	agentID, err := generateAgentID()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate agent credentials",
		})
	}
	apiKey, err := generateAPIKey()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate agent credentials",
		})
	}
	apiKeyHash := HashAPIKey(apiKey)

	// Insert agent into database (only store hash, never plaintext)
	query := `
		INSERT INTO agents (user_id, agent_id, name, api_key_hash, version, os, arch, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'offline')
		RETURNING id, created_at
	`

	var id int
	var createdAt time.Time
	err = database.DB.QueryRow(
		context.Background(),
		query,
		userID,
		agentID,
		req.Name,
		apiKeyHash,
		req.Version,
		req.OS,
		req.Arch,
	).Scan(&id, &createdAt)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to register agent",
		})
	}

	wsURL := settings.Get("PUBLIC_WS_URL", "wss://localhost/ws/agent")

	// Return agent credentials
	return c.Status(fiber.StatusCreated).JSON(models.RegisterAgentResponse{
		AgentID: agentID,
		APIKey:  apiKey,
		WSURL:   wsURL,
	})
}

// ListAgents returns all agents for the authenticated user (or all agents for admins)
// Automatically marks agents as offline if no heartbeat received in 90+ seconds
func ListAgents(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	var query string
	var args []interface{}

	if role == "admin" {
		query = `
			SELECT a.id, a.agent_id, a.name, COALESCE(a.status, 'offline'),
			       COALESCE(a.version, ''), COALESCE(a.os, ''), COALESCE(a.arch, ''),
			       COALESCE(a.last_seen, a.created_at),
			       COALESCE(a.ip_address, ''), COALESCE(a.lan_ip, ''), COALESCE(a.wan_ip, ''),
			       COALESCE(a.host_count, 0),
			       COALESCE(d.dns_count, 0),
			       COALESCE(a.metrics_interval, 300),
			       COALESCE(a.crowdsec_installed, false),
			       COALESCE(a.nginx_version, ''),
			       a.created_at, a.updated_at,
			       a.user_id
			FROM agents a
			LEFT JOIN (SELECT agent_id, COUNT(*) AS dns_count FROM dns_records GROUP BY agent_id) d ON d.agent_id = a.id
			ORDER BY a.created_at DESC
		`
	} else {
		query = `
			SELECT a.id, a.agent_id, a.name, COALESCE(a.status, 'offline'),
			       COALESCE(a.version, ''), COALESCE(a.os, ''), COALESCE(a.arch, ''),
			       COALESCE(a.last_seen, a.created_at),
			       COALESCE(a.ip_address, ''), COALESCE(a.lan_ip, ''), COALESCE(a.wan_ip, ''),
			       COALESCE(a.host_count, 0),
			       COALESCE(d.dns_count, 0),
			       COALESCE(a.metrics_interval, 300),
			       COALESCE(a.crowdsec_installed, false),
			       COALESCE(a.nginx_version, ''),
			       a.created_at, a.updated_at,
			       a.user_id
			FROM agents a
			LEFT JOIN (SELECT agent_id, COUNT(*) AS dns_count FROM dns_records GROUP BY agent_id) d ON d.agent_id = a.id
			WHERE a.user_id = $1 OR a.id IN (SELECT agent_id FROM user_agents WHERE user_id = $1)
			ORDER BY a.created_at DESC
		`
		args = append(args, userID)
	}

	rows, err := database.DB.Query(context.Background(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch agents",
		})
	}
	defer rows.Close()

	agents := []models.Agent{}
	for rows.Next() {
		var agent models.Agent
		err := rows.Scan(
			&agent.ID,
			&agent.AgentID,
			&agent.Name,
			&agent.Status,
			&agent.Version,
			&agent.OS,
			&agent.Arch,
			&agent.LastSeen,
			&agent.IPAddress,
			&agent.LanIP,
			&agent.WanIP,
			&agent.HostCount,
			&agent.DNSRecordCount,
			&agent.MetricsInterval,
			&agent.CrowdSecInstalled,
			&agent.NginxVersion,
			&agent.CreatedAt,
			&agent.UpdatedAt,
			&agent.UserID,
		)
		if err != nil {
			slog.Error("error scanning agent row", "component", "agents", "error", err)
			continue
		}
		markAgentOfflineIfStale(&agent)
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating agents", "component", "agents", "error", err)
	}

	return c.JSON(agents)
}

// GetAgent returns a specific agent by agent_id
// Automatically marks agent as offline if no heartbeat received in 90+ seconds
func GetAgent(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	agentID := c.Params("agentId")

	var query string
	var args []interface{}

	if role == "admin" {
		query = `
			SELECT a.id, a.agent_id, a.name, COALESCE(a.status, 'offline'),
			       COALESCE(a.version, ''), COALESCE(a.os, ''), COALESCE(a.arch, ''),
			       COALESCE(a.last_seen, a.created_at),
			       COALESCE(a.ip_address, ''), COALESCE(a.lan_ip, ''), COALESCE(a.wan_ip, ''),
			       COALESCE(a.host_count, 0),
			       (SELECT COUNT(*) FROM dns_records d WHERE d.agent_id = a.id),
			       COALESCE(a.metrics_interval, 300),
			       COALESCE(a.crowdsec_installed, false),
			       COALESCE(a.nginx_version, ''),
			       a.created_at, a.updated_at,
			       a.user_id
			FROM agents a
			WHERE a.agent_id = $1
		`
		args = append(args, agentID)
	} else {
		query = `
			SELECT a.id, a.agent_id, a.name, COALESCE(a.status, 'offline'),
			       COALESCE(a.version, ''), COALESCE(a.os, ''), COALESCE(a.arch, ''),
			       COALESCE(a.last_seen, a.created_at),
			       COALESCE(a.ip_address, ''), COALESCE(a.lan_ip, ''), COALESCE(a.wan_ip, ''),
			       COALESCE(a.host_count, 0),
			       (SELECT COUNT(*) FROM dns_records d WHERE d.agent_id = a.id),
			       COALESCE(a.metrics_interval, 300),
			       COALESCE(a.crowdsec_installed, false),
			       COALESCE(a.nginx_version, ''),
			       a.created_at, a.updated_at,
			       a.user_id
			FROM agents a
			WHERE a.agent_id = $1
			  AND (a.user_id = $2 OR a.id IN (SELECT agent_id FROM user_agents WHERE user_id = $2))
		`
		args = append(args, agentID, userID)
	}

	var agent models.Agent
	err := database.DB.QueryRow(context.Background(), query, args...).Scan(
		&agent.ID,
		&agent.AgentID,
		&agent.Name,
		&agent.Status,
		&agent.Version,
		&agent.OS,
		&agent.Arch,
		&agent.LastSeen,
		&agent.IPAddress,
		&agent.LanIP,
		&agent.WanIP,
		&agent.HostCount,
		&agent.DNSRecordCount,
		&agent.MetricsInterval,
		&agent.CrowdSecInstalled,
		&agent.NginxVersion,
		&agent.CreatedAt,
		&agent.UpdatedAt,
		&agent.UserID,
	)

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Agent not found",
		})
	}

	markAgentOfflineIfStale(&agent)

	return c.JSON(agent)
}

// RenameAgent handles PATCH /api/agents/:agentId — updates the agent's display name
func RenameAgent(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	agentID := c.Params("agentId")

	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name cannot be empty"})
	}
	if len(req.Name) > 255 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name must be 255 characters or less"})
	}

	var query string
	var args []interface{}
	if role == "admin" {
		query = `UPDATE agents SET name = $1, updated_at = NOW() WHERE agent_id = $2`
		args = []interface{}{req.Name, agentID}
	} else {
		query = `UPDATE agents SET name = $1, updated_at = NOW() WHERE agent_id = $2 AND (user_id = $3 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $3))`
		args = []interface{}{req.Name, agentID, userID}
	}

	result, err := database.DB.Exec(context.Background(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to rename agent"})
	}
	if result.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Agent not found"})
	}

	return c.JSON(fiber.Map{"message": "Agent renamed successfully"})
}

// DeleteAgent removes an agent
func DeleteAgent(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	agentID := c.Params("agentId")

	var query string
	var args []interface{}
	if role == "admin" {
		query = `DELETE FROM agents WHERE agent_id = $1`
		args = []interface{}{agentID}
	} else {
		query = `DELETE FROM agents WHERE user_id = $1 AND agent_id = $2`
		args = []interface{}{userID, agentID}
	}
	result, err := database.DB.Exec(context.Background(), query, args...)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete agent",
		})
	}

	if result.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Agent not found",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Agent deleted successfully",
	})
}

// UpdateAgentHeartbeat updates the agent's last seen timestamp, status, and all metadata
// This function is called when the agent sends a heartbeat with complete system information
// including IP addresses, version, OS, architecture, and host count
func UpdateAgentHeartbeat(agentID string, status string, hostCount int, ipAddress string, version string, os string, arch string, lanIP string, wanIP string, crowdsecInstalled bool, nginxVersion string) error {
	query := `
		UPDATE agents
		SET last_seen = $1, status = $2, host_count = $3, ip_address = $4,
		    version = $5, os = $6, arch = $7, lan_ip = $8, wan_ip = $9,
		    crowdsec_installed = $10, nginx_version = $11, updated_at = $1
		WHERE agent_id = $12
	`

	_, err := database.DB.Exec(
		context.Background(),
		query,
		time.Now(),
		status,
		hostCount,
		ipAddress,
		version,
		os,
		arch,
		lanIP,
		wanIP,
		crowdsecInstalled,
		nginxVersion,
		agentID,
	)

	return err
}

// UpdateAgentStatus updates only the agent's status and last seen time
// Used for registration/unregistration to avoid clearing IP data
func UpdateAgentStatus(agentID string, status string) error {
	query := `
		UPDATE agents
		SET last_seen = $1, status = $2, updated_at = $1
		WHERE agent_id = $3
	`

	_, err := database.DB.Exec(
		context.Background(),
		query,
		time.Now(),
		status,
		agentID,
	)

	return err
}

// UpdateAgent triggers an update for a specific agent via WebSocket command
// The agent will download and install the latest version, then automatically restart
func UpdateAgent(c *fiber.Ctx) error {
	agentID := c.Params("agentId")

	// Verify agent access
	var id int
	if err := verifyAgentAccessByID(c, agentID, &id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Agent not found",
		})
	}

	// Local agent updates with the control node binary
	if IsLocalAgent(agentID) {
		return c.JSON(fiber.Map{
			"message": "Local agent updates automatically with the control node",
		})
	}

	// Send update command to agent via WebSocket
	command := models.AgentCommand{
		Type: "update",
		Payload: map[string]interface{}{
			"version": "latest",
		},
	}

	if err := SendCommandToAgent(agentID, command); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Agent is not connected or command failed",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Update command sent to agent",
	})
}

// UpgradeAgentNginx sends an upgrade_nginx command to a specific agent
func UpgradeAgentNginx(c *fiber.Ctx) error {
	agentID := c.Params("agentId")

	// Verify agent access
	var id int
	if err := verifyAgentAccessByID(c, agentID, &id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Agent not found",
		})
	}

	// Local agent: nginx is managed externally (e.g. Docker container)
	if IsLocalAgent(agentID) {
		return c.JSON(fiber.Map{
			"message": "Local agent nginx is managed externally",
		})
	}

	command := models.AgentCommand{
		Type:    "upgrade_nginx",
		Payload: map[string]interface{}{},
	}

	if err := SendCommandToAgent(agentID, command); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Agent is not connected or command failed",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Nginx upgrade command sent to agent",
	})
}

// GetAgentLogs requests logs from a specific agent via WebSocket command
// Waits up to 10 seconds for the agent to respond with log data
func GetAgentLogs(c *fiber.Ctx) error {
	agentID := c.Params("agentId")

	// Verify agent access
	var id int
	if err := verifyAgentAccessByID(c, agentID, &id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Agent not found",
		})
	}

	// Local agent: get logs directly
	if IsLocalAgent(agentID) {
		logs, err := localAgent.GetNginxLogs(100)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(fiber.Map{
			"logs": logs,
		})
	}

	// Send get_logs command to agent via WebSocket
	command := models.AgentCommand{
		Type:    "get_logs",
		Payload: map[string]interface{}{},
	}

	// Try to get logs from agent with timeout
	logs, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutFast)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"logs": logs,
	})
}

// AuthenticateAgent verifies the agent's API key using SHA-256 hash lookup
func AuthenticateAgent(apiKey string) (*models.Agent, error) {
	keyHash := HashAPIKey(apiKey)

	query := `
		SELECT id, user_id, agent_id, name, status,
		       COALESCE(version, ''), COALESCE(os, ''), COALESCE(arch, ''),
		       last_seen, COALESCE(ip_address, ''), COALESCE(lan_ip, ''), COALESCE(wan_ip, ''),
		       COALESCE(host_count, 0),
		       (SELECT COUNT(*) FROM dns_records d WHERE d.agent_id = agents.id),
		       COALESCE(metrics_interval, 300), COALESCE(crowdsec_installed, false),
		       COALESCE(nginx_version, ''),
		       created_at, updated_at
		FROM agents
		WHERE api_key_hash = $1
	`

	var agent models.Agent
	err := database.DB.QueryRow(context.Background(), query, keyHash).Scan(
		&agent.ID,
		&agent.UserID,
		&agent.AgentID,
		&agent.Name,
		&agent.Status,
		&agent.Version,
		&agent.OS,
		&agent.Arch,
		&agent.LastSeen,
		&agent.IPAddress,
		&agent.LanIP,
		&agent.WanIP,
		&agent.HostCount,
		&agent.DNSRecordCount,
		&agent.MetricsInterval,
		&agent.CrowdSecInstalled,
		&agent.NginxVersion,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	return &agent, nil
}

// SetMetricsInterval sends a set_metrics_interval command to an agent
func SetMetricsInterval(c *fiber.Ctx) error {
	agentID := c.Params("agentId")

	// Verify agent access
	var id int
	if err := verifyAgentAccessByID(c, agentID, &id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Agent not found"})
	}

	var body struct {
		Interval int `json:"interval"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if body.Interval < 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "interval must be at least 10 seconds"})
	}

	command := models.AgentCommand{
		Type: "set_metrics_interval",
		Payload: map[string]interface{}{
			"interval": body.Interval,
		},
	}

	// Save to database so it persists across refreshes and restarts
	if _, err := database.DB.Exec(context.Background(),
		`UPDATE agents SET metrics_interval = $1 WHERE agent_id = $2`, body.Interval, agentID,
	); err != nil {
		slog.Error("failed to save metrics_interval to DB", "component", "agents", "agent_id", agentID, "error", err)
	}

	// Local agent doesn't use WebSocket — interval is read from DB on next cycle
	if !IsLocalAgent(agentID) {
		if err := SendCommandToAgent(agentID, command); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to send command: %v", err),
			})
		}
	}

	return c.JSON(fiber.Map{"message": fmt.Sprintf("Metrics interval set to %d seconds", body.Interval)})
}
