package middleware

import "github.com/gofiber/fiber/v2"

// AdminOnly restricts access to users with role "admin".
// Must be chained after Auth middleware which sets "user_role".
func AdminOnly(c *fiber.Ctx) error {
	role, _ := c.Locals("user_role").(string)
	if role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Admin access required",
		})
	}
	return c.Next()
}
