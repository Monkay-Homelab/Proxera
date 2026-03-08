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

// GetBanDuration reads the default ban duration from CrowdSec's profiles.yaml.
func (m *Manager) GetBanDuration() (string, error) {
	profilesPath := "/etc/crowdsec/profiles.yaml"

	var data []byte
	var err error
	if m.IsDocker() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "exec", m.dockerContainer, "cat", profilesPath)
		data, err = cmd.Output()
	} else {
		data, err = exec.Command("cat", profilesPath).Output()
	}
	if err != nil {
		return "4h", nil // CrowdSec default
	}

	// Parse duration from YAML: find "duration: <value>" under decisions
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "duration:") && !strings.HasPrefix(trimmed, "#") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "duration:"))
			if val != "" {
				return val, nil
			}
		}
	}

	return "4h", nil
}

// SetBanDuration updates the default ban duration in CrowdSec's profiles.yaml
// and reloads CrowdSec to apply the change.
func (m *Manager) SetBanDuration(duration string) error {
	profilesPath := "/etc/crowdsec/profiles.yaml"

	// Read current profiles.yaml
	var data []byte
	var err error
	if m.IsDocker() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "exec", m.dockerContainer, "cat", profilesPath)
		data, err = cmd.Output()
	} else {
		data, err = exec.Command("cat", profilesPath).Output()
	}
	if err != nil {
		return fmt.Errorf("failed to read profiles.yaml: %w", err)
	}

	// Replace all "duration: <old>" lines (not commented) with new duration
	lines := strings.Split(string(data), "\n")
	var result []string
	replaced := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "duration:") && !strings.HasPrefix(trimmed, "#") {
			// Preserve indentation
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			result = append(result, indent+"duration: "+duration)
			replaced = true
		} else {
			result = append(result, line)
		}
	}

	if !replaced {
		return fmt.Errorf("no duration field found in profiles.yaml")
	}

	newContent := strings.Join(result, "\n")

	// Write back
	if m.IsDocker() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", m.dockerContainer, "sh", "-c", fmt.Sprintf("cat > %s", profilesPath))
		cmd.Stdin = strings.NewReader(newContent)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to write profiles.yaml: %s", string(out))
		}
	} else {
		writeCmd := exec.Command("sh", "-c", fmt.Sprintf("cat > %s", profilesPath))
		writeCmd.Stdin = strings.NewReader(newContent)
		if out, err := writeCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to write profiles.yaml: %s", string(out))
		}
	}

	// Reload CrowdSec to apply
	if m.IsDocker() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", "exec", m.dockerContainer, "sh", "-c", "kill -HUP 1")
		cmd.CombinedOutput()
	} else {
		exec.Command("systemctl", "reload", "crowdsec").Run()
	}

	return nil
}
