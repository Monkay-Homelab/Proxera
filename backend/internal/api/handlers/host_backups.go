package handlers

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/models"
)

// hostBackupInfo mirrors the BackupInfo struct from the agent.
type hostBackupInfo struct {
	Filename  string    `json:"filename"`
	Domain    string    `json:"domain"`
	Timestamp time.Time `json:"timestamp"`
	SizeBytes int64     `json:"size_bytes"`
}

// lookupHostForBackup returns the domain and agent string ID for a host owned by userID.
func lookupHostForBackup(hostID, userID int) (domain, agentStringID string, err error) {
	var agentDBID *int
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = database.DB.QueryRow(ctx,
		`SELECT domain, agent_id FROM hosts WHERE id = $1 AND user_id = $2`,
		hostID, userID,
	).Scan(&domain, &agentDBID)
	if err != nil {
		return "", "", fiber.NewError(fiber.StatusNotFound, "Host not found")
	}
	if agentDBID == nil {
		return "", "", fiber.NewError(fiber.StatusBadRequest, "Host has no agent assigned")
	}

	agentStringID, err = lookupAgentStringID(*agentDBID)
	if err != nil {
		return "", "", fiber.NewError(fiber.StatusBadRequest, "Agent not found")
	}
	return domain, agentStringID, nil
}

// ListHostBackups dispatches list_backups to the agent and returns the backup list.
// GET /api/hosts/:providerId/configs/:id/backups
func ListHostBackups(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	hostID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid host ID"})
	}

	domain, agentStringID, err := lookupHostForBackup(hostID, userID)
	if err != nil {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	command := models.AgentCommand{
		Type:    "list_backups",
		Payload: map[string]interface{}{"domain": domain},
	}

	response, err := SendCommandAndWaitForResponse(agentStringID, command, CmdTimeoutFast)
	if err != nil {
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{"error": "Agent did not respond: " + err.Error()})
	}

	var backups []hostBackupInfo
	if err := json.Unmarshal([]byte(response), &backups); err != nil {
		// Agent returned an empty list or non-JSON — treat as empty
		backups = []hostBackupInfo{}
	}
	if backups == nil {
		backups = []hostBackupInfo{}
	}

	return c.JSON(fiber.Map{"backups": backups})
}

// GetHostBackupContent dispatches get_backup to the agent and returns the raw config text.
// GET /api/hosts/:providerId/configs/:id/backups/:filename
func GetHostBackupContent(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	hostID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid host ID"})
	}

	filename := c.Params("filename")
	if filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "filename is required"})
	}

	domain, agentStringID, err := lookupHostForBackup(hostID, userID)
	if err != nil {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	command := models.AgentCommand{
		Type: "get_backup",
		Payload: map[string]interface{}{
			"domain":   domain,
			"filename": filename,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentStringID, command, CmdTimeoutFast)
	if err != nil {
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{"error": "Agent did not respond: " + err.Error()})
	}

	return c.JSON(fiber.Map{"content": response})
}

// RestoreHostBackup dispatches restore_backup to the agent for the requested filename.
// POST /api/hosts/:providerId/configs/:id/backups/restore
// Body: { "filename": "proxera_example_com_20240101_120000.conf" }
func RestoreHostBackup(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	hostID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid host ID"})
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := c.BodyParser(&req); err != nil || req.Filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "filename is required"})
	}

	domain, agentStringID, err := lookupHostForBackup(hostID, userID)
	if err != nil {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	command := models.AgentCommand{
		Type: "restore_backup",
		Payload: map[string]interface{}{
			"domain":   domain,
			"filename": req.Filename,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentStringID, command, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{"error": "Agent did not respond: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": response})
}
