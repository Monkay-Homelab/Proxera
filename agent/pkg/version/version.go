package version

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

// Version is the current agent version — overridden at build time via:
//
//	go build -ldflags "-X github.com/proxera/agent/pkg/version.Version=$AGENT_VERSION"
var Version = "0.6.7"

// UpdateCheckURL is the endpoint used to check for agent updates.
// Set at build time via: -ldflags "-X github.com/proxera/agent/pkg/version.UpdateCheckURL=<url>"
// When connecting via WebSocket the backend sends the URL directly; this variable
// is only used by the standalone -check-update / -update CLI flags.
var UpdateCheckURL = ""

// Info represents version information
type Info struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	BuildDate string `json:"build_date,omitempty"`
}

// VersionResponse from update server
type VersionResponse struct {
	LatestVersion string `json:"latest_version"`
	DownloadURL   string `json:"download_url"`
	ReleaseNotes  string `json:"release_notes,omitempty"`
}

// GetInfo returns current version information
func GetInfo() Info {
	return Info{
		Version:   Version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// CheckForUpdate checks if a new version is available
func CheckForUpdate(updateURL string) (*VersionResponse, error) {
	if updateURL == "" {
		updateURL = UpdateCheckURL
	}
	if updateURL == "" {
		return nil, fmt.Errorf("no update URL configured; set panel_url in your config or provide a URL explicitly")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(updateURL)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check failed with status: %d", resp.StatusCode)
	}

	var versionResp VersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionResp); err != nil {
		return nil, fmt.Errorf("failed to parse version response: %w", err)
	}

	return &versionResp, nil
}

// DownloadUpdate downloads and installs a new version
func DownloadUpdate(downloadURL, targetPath string) error {
	fmt.Printf("[INFO] Downloading update from %s...\n", downloadURL)

	// Download new binary
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "proxera-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write downloaded binary
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write update: %w", err)
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	// Backup current binary
	backupPath := targetPath + ".backup"
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Replace with new binary
	if err := os.Rename(tmpFile.Name(), targetPath); err != nil {
		// Restore backup on failure
		if restoreErr := os.Rename(backupPath, targetPath); restoreErr != nil {
			return fmt.Errorf("CRITICAL: failed to install update and failed to restore backup: install=%w, restore=%v", err, restoreErr)
		}
		return fmt.Errorf("failed to install update (backup restored): %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	fmt.Println("[OK] Update installed successfully!")
	fmt.Println("[INFO] Restart the agent for changes to take effect")

	return nil
}

// SelfUpdate performs a self-update
func SelfUpdate() error {
	// Get current binary path
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Check for update
	updateInfo, err := CheckForUpdate("")
	if err != nil {
		return err
	}

	if updateInfo.LatestVersion == Version {
		fmt.Printf("[OK] Already running latest version (%s)\n", Version)
		return nil
	}

	fmt.Printf("[INFO] New version available: %s (current: %s)\n", updateInfo.LatestVersion, Version)
	if updateInfo.ReleaseNotes != "" {
		fmt.Printf("\nRelease notes:\n%s\n\n", updateInfo.ReleaseNotes)
	}

	// Download and install update
	return DownloadUpdate(updateInfo.DownloadURL, binaryPath)
}

// GetBinaryPath returns the path to the running binary
func GetBinaryPath() (string, error) {
	return os.Executable()
}

