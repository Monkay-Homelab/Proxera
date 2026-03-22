package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/settings"
)

// allowedSettingsKeys defines the allowlist of valid system settings keys.
// Any key not in this map will be rejected by AdminUpdateSettings.
var allowedSettingsKeys = map[string]bool{
	"registration_mode":         true,
	"invite_code":               true,
	"PUBLIC_SITE_URL":           true,
	"PUBLIC_API_URL":            true,
	"PUBLIC_WS_URL":             true,
	"SMTP_HOST":                 true,
	"SMTP_PORT":                 true,
	"SMTP_USER":                 true,
	"SMTP_PASSWORD":             true,
	"SMTP_FROM_EMAIL":           true,
	"ENABLE_EMAIL_VERIFICATION": true,
	"ACME_STAGING":              true,
	"crowdsec_eula_accepted":    true,
}

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
		// Mask SMTP_PASSWORD so the actual (encrypted) value is never exposed via API
		if k == "SMTP_PASSWORD" && v != "" {
			v = "********"
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

	// Validate all keys before writing any (atomic: all valid or none written)
	for k := range body {
		if !allowedSettingsKeys[k] {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Unknown setting key: %s", k),
			})
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for k, v := range body {
		storeValue := v

		// Encrypt SMTP_PASSWORD before storing. Skip if the value is the mask
		// placeholder (user did not change the password) or empty.
		if k == "SMTP_PASSWORD" && v != "" && v != "********" {
			encrypted, err := crypto.Encrypt(v)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Failed to encrypt SMTP password",
				})
			}
			storeValue = encrypted
		}

		// If the user submitted the mask placeholder, skip writing so the
		// existing encrypted value is preserved.
		if k == "SMTP_PASSWORD" && v == "********" {
			continue
		}

		_, err := database.DB.Exec(ctx,
			`INSERT INTO system_settings (key, value, updated_at) VALUES ($1, $2, NOW())
			 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`,
			k, storeValue,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update settings"})
		}
		settings.Invalidate(k)
	}

	return AdminGetSettings(c)
}
