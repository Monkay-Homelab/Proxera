package crowdsec

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetMetrics returns CrowdSec metrics as raw JSON string
func (m *Manager) GetMetrics() (string, error) {
	cmd := exec.Command("cscli", "metrics", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get metrics: %s", string(output))
	}
	return strings.TrimSpace(string(output)), nil
}
