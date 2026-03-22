package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"golang.org/x/crypto/bcrypt"
)

// DeleteAccount deletes the user account and all associated data.
// Endpoint: DELETE /api/user/me
// Body: { "password": "..." }
func DeleteAccount(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var req struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	if req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password is required",
		})
	}

	// Use a 60-second timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Verify password
	var hashedPassword string
	err := database.DB.QueryRow(ctx,
		`SELECT password FROM users WHERE id = $1`, userID,
	).Scan(&hashedPassword)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Incorrect password",
		})
	}

	// Collect agent_id strings for metrics/visitor_ips cleanup
	agentIDs, err := collectAgentIDs(ctx, userID)
	if err != nil {
		slog.Error("failed to collect agent IDs for account deletion", "component", "auth", "user_id", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Account deletion failed",
		})
	}

	// Delete metrics and visitor_ips (no FK, manual cleanup)
	if len(agentIDs) > 0 {
		if _, err := database.DB.Exec(ctx,
			`DELETE FROM metrics WHERE agent_id = ANY($1)`, agentIDs,
		); err != nil {
			slog.Error("metrics delete failed for account deletion", "component", "auth", "user_id", userID, "error", err)
		}
		if _, err := database.DB.Exec(ctx,
			`DELETE FROM visitor_ips WHERE agent_id = ANY($1)`, agentIDs,
		); err != nil {
			slog.Error("visitor_ips delete failed for account deletion", "component", "auth", "user_id", userID, "error", err)
		}
	}

	// Delete user (CASCADE handles agents, dns_providers, dns_records, hosts, certificates)
	_, err = database.DB.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		slog.Error("user delete failed for account deletion", "component", "auth", "user_id", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Account deletion failed",
		})
	}

	slog.Info("account deleted", "component", "auth", "user_id", userID)
	return c.JSON(fiber.Map{
		"message": "Account deleted",
	})
}

func collectAgentIDs(ctx context.Context, userID int) ([]string, error) {
	rows, err := database.DB.Query(ctx,
		`SELECT agent_id FROM agents WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
