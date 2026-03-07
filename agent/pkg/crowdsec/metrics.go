package crowdsec

import (
	"fmt"
	"strings"
)

// GetMetrics returns CrowdSec metrics as raw JSON string
func (m *Manager) GetMetrics() (string, error) {
	output, err := m.runCscliCmd("metrics", "-o", "json")
	if err != nil {
		return "", fmt.Errorf("failed to get metrics: %v", err)
	}
	return strings.TrimSpace(string(output)), nil
}
