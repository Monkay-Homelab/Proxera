package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/proxera/agent/pkg/version"
	"github.com/proxera/backend/internal/settings"
)

func getAgentDownloadURL() string {
	apiURL := settings.Get("PUBLIC_API_URL", "http://localhost:8080")
	return apiURL + "/api/agent/download/proxera-linux-amd64"
}

// GetAgentVersion returns the latest agent version information.
// Version comes from the shared version package, set at build time via git tags.
func GetAgentVersion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"latest_version": version.Version,
		"download_url":   getAgentDownloadURL(),
	})
}

// DownloadAgent serves the latest agent binary
func DownloadAgent(c *fiber.Ctx) error {
	filename := c.Params("filename")

	// Validate filename to prevent directory traversal
	if filename != "proxera-linux-amd64" && filename != "proxera-darwin-amd64" && filename != "proxera-windows-amd64.exe" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Binary not found",
		})
	}

	// Serve the binary file
	filepath := "./downloads/" + filename
	return c.Download(filepath, filename)
}
