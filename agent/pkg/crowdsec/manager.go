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
type Manager struct {
	// dockerContainer is set when running in Docker mode.
	// All cscli commands are executed via "docker exec <container> ...".
	dockerContainer string
}

// NewManager creates a new CrowdSec manager.
// Pass an empty container name for standalone mode.
func NewManager(dockerContainer string) *Manager {
	return &Manager{dockerContainer: dockerContainer}
}

// IsDocker returns true if CrowdSec is managed as a Docker container.
func (m *Manager) IsDocker() bool {
	return m.dockerContainer != ""
}

// StatusInfo contains combined CrowdSec status information
type StatusInfo struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version"`
}

// IsInstalled checks if CrowdSec is installed (or container exists in Docker mode)
func (m *Manager) IsInstalled() bool {
	if m.IsDocker() {
		return m.dockerContainerExists()
	}
	_, err := exec.LookPath("cscli")
	return err == nil
}

// IsRunning checks if the CrowdSec service is active
func (m *Manager) IsRunning() bool {
	if m.IsDocker() {
		return m.dockerContainerRunning()
	}
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

	if info.Running {
		output, err := m.runCscliCmd("version")
		if err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "version:") {
					ver := strings.TrimSpace(strings.TrimPrefix(line, "version:"))
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

// dockerContainerExists checks if the CrowdSec container exists (any state)
func (m *Manager) dockerContainerExists() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "inspect", m.dockerContainer)
	return cmd.Run() == nil
}

// dockerContainerRunning checks if the CrowdSec container is running
func (m *Manager) dockerContainerRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", m.dockerContainer)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// runCscliCmd runs a cscli command, routing through docker exec in Docker mode
func (m *Manager) runCscliCmd(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if m.IsDocker() {
		dockerArgs := append([]string{"exec", m.dockerContainer, "cscli"}, args...)
		cmd = exec.CommandContext(ctx, "docker", dockerArgs...)
	} else {
		cmd = exec.CommandContext(ctx, "cscli", args...)
	}

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
func (m *Manager) runJSON(args []string, dest interface{}) error {
	output, err := m.runCscliCmd(args...)
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
