package crowdsec

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Manager handles CrowdSec operations
type Manager struct{}

// NewManager creates a new CrowdSec manager
func NewManager() *Manager {
	return &Manager{}
}

// StatusInfo contains combined CrowdSec status information
type StatusInfo struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version"`
}

// IsInstalled checks if CrowdSec is installed
func (m *Manager) IsInstalled() bool {
	_, err := exec.LookPath("cscli")
	return err == nil
}

// IsRunning checks if the CrowdSec service is active
func (m *Manager) IsRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "crowdsec")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "active"
}

// Status returns combined CrowdSec status
func (m *Manager) Status() (*StatusInfo, error) {
	info := &StatusInfo{
		Installed: m.IsInstalled(),
		Running:   m.IsRunning(),
	}

	if info.Installed {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "cscli", "version")
		output, err := cmd.CombinedOutput()
		if err == nil {
			// Output is multi-line like "version: v1.7.6-...\nCodename: ...\n..."
			// Extract just the version from the first line
			for _, line := range strings.Split(string(output), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "version:") {
					ver := strings.TrimSpace(strings.TrimPrefix(line, "version:"))
					// Strip build metadata, keep just vX.Y.Z
					if parts := strings.SplitN(ver, "-", 2); len(parts) > 0 {
						info.Version = parts[0]
					}
					break
				}
			}
		}
	}

	return info, nil
}

// runCscli executes a cscli command with a 30-second timeout and returns the raw output
func runCscli(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "cscli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("cscli %s timed out after 30s", strings.Join(args, " "))
		}
		return nil, fmt.Errorf("cscli %s failed: %s", strings.Join(args, " "), string(output))
	}
	return output, nil
}

// runJSON executes a cscli command with -o json and unmarshals the result.
// CrowdSec wraps list output in objects like {"collections": [...]} or returns
// plain arrays depending on the command and version. This handles both.
func runJSON(args []string, dest interface{}) error {
	output, err := runCscli(args...)
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return nil
	}

	// Try direct unmarshal first (works for plain arrays)
	if err := json.Unmarshal([]byte(trimmed), dest); err == nil {
		return nil
	}

	// If direct unmarshal failed, it might be wrapped in an object like {"key": [...]}
	// Try to extract the first array value from the object
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &wrapper); err != nil {
		return fmt.Errorf("failed to parse cscli output: %w", err)
	}

	for _, v := range wrapper {
		if err := json.Unmarshal(v, dest); err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to extract data from cscli output")
}
