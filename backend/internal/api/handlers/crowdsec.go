package handlers

import (
	"context"
	"fmt"
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

	command := models.AgentCommand{
		Type:    "crowdsec_status",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type: "crowdsec_install",
		Payload: map[string]interface{}{
			"enrollment_key": body.EnrollmentKey,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutLong)
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

	command := models.AgentCommand{
		Type:    "crowdsec_uninstall",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutSlow)
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

	command := models.AgentCommand{
		Type:    "crowdsec_decisions_list",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type: "crowdsec_decisions_add",
		Payload: map[string]interface{}{
			"ip":       body.IP,
			"duration": body.Duration,
			"reason":   body.Reason,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type: "crowdsec_decisions_delete",
		Payload: map[string]interface{}{
			"id": decisionID,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type:    "crowdsec_alerts_list",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type: "crowdsec_alerts_delete",
		Payload: map[string]interface{}{
			"id": alertID,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type:    "crowdsec_collections_list",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type: "crowdsec_collections_install",
		Payload: map[string]interface{}{
			"name": body.Name,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutSlow)
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

	name := c.Params("name")
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Collection name is required"})
	}

	command := models.AgentCommand{
		Type: "crowdsec_collections_remove",
		Payload: map[string]interface{}{
			"name": name,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutMedium)
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

	command := models.AgentCommand{
		Type:    "crowdsec_bouncers_list",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type: "crowdsec_bouncer_install",
		Payload: map[string]interface{}{
			"package": body.Package,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutLong)
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

	command := models.AgentCommand{
		Type: "crowdsec_bouncer_remove",
		Payload: map[string]interface{}{
			"package": name,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutSlow)
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

	command := models.AgentCommand{
		Type:    "crowdsec_metrics",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type:    "crowdsec_whitelist_list",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	command := models.AgentCommand{
		Type: "crowdsec_whitelist_add",
		Payload: map[string]interface{}{
			"ip":          body.IP,
			"description": body.Description,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
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

	ip := c.Params("ip")
	if ip == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "IP is required"})
	}

	command := models.AgentCommand{
		Type: "crowdsec_whitelist_remove",
		Payload: map[string]interface{}{
			"ip": ip,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": response})
}
