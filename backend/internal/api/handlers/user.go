package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// GetNavContext returns counts needed for progressive nav unlocking
func GetNavContext(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	ctx := context.Background()
	var agentCount, dnsZoneCount, hostCount int
	_ = database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM agents WHERE user_id = $1", userID).Scan(&agentCount)
	_ = database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM dns_providers WHERE user_id = $1", userID).Scan(&dnsZoneCount)
	_ = database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM hosts WHERE user_id = $1", userID).Scan(&hostCount)
	return c.JSON(fiber.Map{
		"agent_count":    agentCount,
		"dns_zone_count": dnsZoneCount,
		"host_count":     hostCount,
	})
}

// ChangePassword verifies the current password then sets a new one
func ChangePassword(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Current and new password are required"})
	}
	if len(req.NewPassword) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "New password must be at least 8 characters"})
	}

	ctx := context.Background()
	var hashedPassword string
	err := database.DB.QueryRow(ctx, "SELECT password FROM users WHERE id = $1", userID).Scan(&hashedPassword)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to verify password"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.CurrentPassword)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Current password is incorrect"})
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	_, err = database.DB.Exec(ctx,
		"UPDATE users SET password = $1, password_changed_at = NOW(), updated_at = NOW() WHERE id = $2",
		string(newHash), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update password"})
	}

	return c.JSON(fiber.Map{"message": "Password updated successfully. Please log in again."})
}

// GetCurrentUser returns the authenticated user's information
func GetCurrentUser(c *fiber.Ctx) error {
	// Get user ID from context (set by auth middleware)
	userID, _ := c.Locals("user_id").(int)

	var user models.User
	query := `SELECT id, email, name, COALESCE(role, 'member'), COALESCE(email_verified, false), created_at, updated_at FROM users WHERE id = $1`

	err := database.DB.QueryRow(context.Background(), query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Role,
		&user.EmailVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"role":           user.Role,
		"email_verified": user.EmailVerified,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
	})
}
