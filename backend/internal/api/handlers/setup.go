package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/settings"
)

// SetupStatus returns the current setup state.
// Used after first admin registration to check if additional setup steps are needed.
func SetupStatus(c *fiber.Ctx) error {
	crowdsecEulaRequired := false

	// Check if CrowdSec is running on the local agent but EULA hasn't been accepted
	if settings.Get("crowdsec_eula_accepted", "") != "true" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var csInstalled bool
		err := database.DB.QueryRow(ctx,
			`SELECT crowdsec_installed FROM agents WHERE is_local = true LIMIT 1`,
		).Scan(&csInstalled)
		if err == nil && csInstalled {
			crowdsecEulaRequired = true
		}
	}

	return c.JSON(fiber.Map{
		"crowdsec_eula_required": crowdsecEulaRequired,
	})
}

// AcceptCrowdSecEULA records the admin's acceptance of the CrowdSec EULA.
func AcceptCrowdSecEULA(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := database.DB.Exec(ctx,
		`INSERT INTO system_settings (key, value, updated_at) VALUES ('crowdsec_eula_accepted', 'true', NOW())
		 ON CONFLICT (key) DO UPDATE SET value = 'true', updated_at = NOW()`,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save EULA acceptance",
		})
	}

	return c.JSON(fiber.Map{"accepted": true})
}
