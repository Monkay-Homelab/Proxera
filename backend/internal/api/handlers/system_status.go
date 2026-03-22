package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/localagent"
)

// GetSystemStatus returns the current system status including OS, architecture,
// nginx version, and CrowdSec status. Requires authentication but not admin role.
func GetSystemStatus(c *fiber.Ctx) error {
	return c.JSON(localagent.DetectSystem())
}
