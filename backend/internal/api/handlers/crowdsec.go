package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/models"
)

// verifyAgentOwnership checks that the agent belongs to the authenticated user
func verifyAgentOwnership(c *fiber.Ctx) (string, error) {
	userID, _ := c.Locals("user_id").(int)
	agentID := c.Params("agentId")

	var id int
	err := database.DB.QueryRow(
		context.Background(),
		`SELECT id FROM agents WHERE user_id = $1 AND agent_id = $2`,
		userID, agentID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("agent not found")
	}
	return agentID, nil
}

// CrowdSecStatus handles GET /api/agents/:agentId/crowdsec/status
func CrowdSecStatus(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		result, err := localAgent.CrowdSecStatus()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_status", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendString(response)
}

// CrowdSecInstall handles POST /api/agents/:agentId/crowdsec/install
func CrowdSecInstall(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var body struct {
		EnrollmentKey string `json:"enrollment_key"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecInstall(body.EnrollmentKey); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "CrowdSec installed successfully"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_install", Payload: map[string]interface{}{"enrollment_key": body.EnrollmentKey},
	}, CmdTimeoutLong)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecUninstall handles POST /api/agents/:agentId/crowdsec/uninstall
func CrowdSecUninstall(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecUninstall(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "CrowdSec uninstalled successfully"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_uninstall", Payload: map[string]interface{}{},
	}, CmdTimeoutSlow)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecListDecisions handles GET /api/agents/:agentId/crowdsec/decisions
func CrowdSecListDecisions(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		result, err := localAgent.CrowdSecListDecisions()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_decisions_list", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendString(response)
}

// CrowdSecAddDecision handles POST /api/agents/:agentId/crowdsec/decisions
func CrowdSecAddDecision(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var body struct {
		IP       string `json:"ip"`
		Duration string `json:"duration"`
		Reason   string `json:"reason"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if body.IP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "IP is required"})
	}
	if body.Duration == "" {
		body.Duration = "24h"
	}
	if body.Reason == "" {
		body.Reason = "Manual ban"
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecAddDecision(body.IP, body.Duration, body.Reason); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Decision added"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_decisions_add", Payload: map[string]interface{}{
			"ip": body.IP, "duration": body.Duration, "reason": body.Reason,
		},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecDeleteDecision handles DELETE /api/agents/:agentId/crowdsec/decisions/:decisionId
func CrowdSecDeleteDecision(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	decisionID, err := strconv.Atoi(c.Params("decisionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid decision ID"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecDeleteDecision(decisionID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Decision deleted"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_decisions_delete", Payload: map[string]interface{}{"id": decisionID},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecListAlerts handles GET /api/agents/:agentId/crowdsec/alerts
func CrowdSecListAlerts(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		result, err := localAgent.CrowdSecListAlerts()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_alerts_list", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendString(response)
}

// CrowdSecDeleteAlert handles DELETE /api/agents/:agentId/crowdsec/alerts/:alertId
func CrowdSecDeleteAlert(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	alertID, err := strconv.Atoi(c.Params("alertId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid alert ID"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecDeleteAlert(alertID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Alert deleted"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_alerts_delete", Payload: map[string]interface{}{"id": alertID},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecListCollections handles GET /api/agents/:agentId/crowdsec/collections
func CrowdSecListCollections(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		result, err := localAgent.CrowdSecListCollections()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_collections_list", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendString(response)
}

// CrowdSecInstallCollection handles POST /api/agents/:agentId/crowdsec/collections
func CrowdSecInstallCollection(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil || body.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Collection name is required"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecInstallCollection(body.Name); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Collection installed"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_collections_install", Payload: map[string]interface{}{"name": body.Name},
	}, CmdTimeoutSlow)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecRemoveCollection handles DELETE /api/agents/:agentId/crowdsec/collections/:name
func CrowdSecRemoveCollection(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	name := c.Params("*")
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Collection name is required"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecRemoveCollection(name); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Collection removed"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_collections_remove", Payload: map[string]interface{}{"name": name},
	}, CmdTimeoutMedium)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecListBouncers handles GET /api/agents/:agentId/crowdsec/bouncers
func CrowdSecListBouncers(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		result, err := localAgent.CrowdSecListBouncers()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_bouncers_list", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendString(response)
}

// CrowdSecInstallBouncer handles POST /api/agents/:agentId/crowdsec/bouncers
func CrowdSecInstallBouncer(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var body struct {
		Package string `json:"package"`
	}
	if err := c.BodyParser(&body); err != nil || body.Package == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Package name is required"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecInstallBouncer(body.Package); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Bouncer installed"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_bouncer_install", Payload: map[string]interface{}{"package": body.Package},
	}, CmdTimeoutLong)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecRemoveBouncer handles DELETE /api/agents/:agentId/crowdsec/bouncers/:name
func CrowdSecRemoveBouncer(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	name := c.Params("name")
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Package name is required"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecRemoveBouncer(name); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Bouncer removed"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_bouncer_remove", Payload: map[string]interface{}{"package": name},
	}, CmdTimeoutSlow)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecGetMetrics handles GET /api/agents/:agentId/crowdsec/metrics
func CrowdSecGetMetrics(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		result, err := localAgent.CrowdSecGetMetrics()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendString(result)
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_metrics", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendString(response)
}

// CrowdSecListWhitelist handles GET /api/agents/:agentId/crowdsec/whitelist
func CrowdSecListWhitelist(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		result, err := localAgent.CrowdSecListWhitelists()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_whitelist_list", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendString(response)
}

// CrowdSecAddWhitelist handles POST /api/agents/:agentId/crowdsec/whitelist
func CrowdSecAddWhitelist(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var body struct {
		IP          string `json:"ip"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&body); err != nil || body.IP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "IP is required"})
	}
	if net.ParseIP(body.IP) == nil {
		if _, _, err := net.ParseCIDR(body.IP); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid IP address or CIDR"})
		}
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecAddWhitelist(body.IP, body.Description); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "IP whitelisted"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_whitelist_add", Payload: map[string]interface{}{
			"ip": body.IP, "description": body.Description,
		},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecRemoveWhitelist handles DELETE /api/agents/:agentId/crowdsec/whitelist/:ip
func CrowdSecRemoveWhitelist(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	ip := c.Params("*")
	if ip == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "IP is required"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecRemoveWhitelist(ip); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "IP removed from whitelist"})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_whitelist_remove", Payload: map[string]interface{}{"ip": ip},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response})
}

// CrowdSecGetBanDuration handles GET /api/agents/:agentId/crowdsec/ban-duration
func CrowdSecGetBanDuration(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if IsLocalAgent(agentID) {
		duration, err := localAgent.CrowdSecGetBanDuration()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"duration": duration})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_get_ban_duration", Payload: map[string]interface{}{},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"duration": response})
}

// CrowdSecSetBanDuration handles PUT /api/agents/:agentId/crowdsec/ban-duration
func CrowdSecSetBanDuration(c *fiber.Ctx) error {
	agentID, err := verifyAgentOwnership(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var body struct {
		Duration string `json:"duration"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if body.Duration == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Duration is required"})
	}

	if IsLocalAgent(agentID) {
		if err := localAgent.CrowdSecSetBanDuration(body.Duration); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "Ban duration updated", "duration": body.Duration})
	}

	response, err := SendCommandAndWaitForResponse(agentID, models.AgentCommand{
		Type: "crowdsec_set_ban_duration", Payload: map[string]interface{}{
			"duration": body.Duration,
		},
	}, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": response, "duration": body.Duration})
}

// jsonStr marshals a value to a JSON string for WebSocket responses
func jsonStr(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
