package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/api/handlers"
)

// AgentAuth middleware for agent authentication via API key
func AgentAuth(c *fiber.Ctx) error {
	// Get API key from Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	// Extract API key from "Bearer <key>" format
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid authorization format",
		})
	}

	apiKey := parts[1]

	// Authenticate agent
	agent, err := handlers.AuthenticateAgent(apiKey)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid API key",
		})
	}

	// Store agent info in context
	c.Locals("agent_id", agent.AgentID)
	c.Locals("agent", agent)

	return c.Next()
}
