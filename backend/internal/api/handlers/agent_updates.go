package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/agent/pkg/version"
	"github.com/proxera/backend/internal/settings"
)

func getAgentDownloadURL() string {
	apiURL := settings.Get("PUBLIC_API_URL", "http://localhost:8080")
	return apiURL + "/api/agent/download/proxera-linux-amd64"
}

// validAgentFilename returns true if the filename is an allowed agent binary name.
func validAgentFilename(filename string) bool {
	return filename == "proxera-linux-amd64" ||
		filename == "proxera-linux-arm64" ||
		filename == "proxera-darwin-amd64" ||
		filename == "proxera-windows-amd64.exe"
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
	if !validAgentFilename(filename) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Binary not found",
		})
	}

	// Serve the binary file
	filepath := "./downloads/" + filename
	return c.Download(filepath, filename)
}

// GetAgentChecksum computes and returns the SHA-256 checksum of the requested agent binary.
func GetAgentChecksum(c *fiber.Ctx) error {
	filename := c.Params("filename")

	if !validAgentFilename(filename) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Binary not found",
		})
	}

	filepath := "./downloads/" + filename
	f, err := os.Open(filepath) //nolint:gosec // filename is validated above via allowedBinaries map
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Binary not found",
		})
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to compute checksum",
		})
	}

	checksum := hex.EncodeToString(h.Sum(nil))

	return c.JSON(fiber.Map{
		"filename": filename,
		"sha256":   checksum,
	})
}
