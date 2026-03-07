package middleware

import "github.com/gofiber/fiber/v2"

// RejectViewer blocks write operations for users with the "viewer" role.
// Must be chained after Auth middleware which sets "user_role".
func RejectViewer(c *fiber.Ctx) error {
	role, _ := c.Locals("user_role").(string)
	if role == "viewer" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Viewer accounts have read-only access",
		})
	}
	return c.Next()
}
