package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
)

// AdminGetSettings returns all system settings.
func AdminGetSettings(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `SELECT key, value FROM system_settings ORDER BY key`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch settings"})
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		settings[k] = v
	}

	return c.JSON(fiber.Map{"settings": settings})
}

// AdminUpdateSettings updates system settings.
func AdminUpdateSettings(c *fiber.Ctx) error {
	var body map[string]string
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for k, v := range body {
		_, err := database.DB.Exec(ctx,
			`INSERT INTO system_settings (key, value, updated_at) VALUES ($1, $2, NOW())
			 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`,
			k, v,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update settings"})
		}
	}

	return AdminGetSettings(c)
}
