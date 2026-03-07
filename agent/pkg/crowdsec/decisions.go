package crowdsec

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// Decision represents a CrowdSec decision (blocked IP)
type Decision struct {
	ID       int    `json:"id"`
	Origin   string `json:"origin"`
	Scope    string `json:"scope"`
	Value    string `json:"value"`
	Reason   string `json:"reason"`
	Scenario string `json:"scenario"`
	Action   string `json:"action"`
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// alertWithDecisions represents the nested alert structure from cscli decisions list
type alertWithDecisions struct {
	Decisions []Decision `json:"decisions"`
}

// ListDecisions returns active local decisions (from local detection)
func (m *Manager) ListDecisions() ([]Decision, error) {
	cmd := exec.Command("cscli", "decisions", "list", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// cscli returns error when no decisions exist
		return []Decision{}, nil
	}

	// cscli decisions list returns alerts with nested decisions
	var alerts []alertWithDecisions
	if err := json.Unmarshal(output, &alerts); err != nil {
		return []Decision{}, nil
	}

	// Flatten decisions from all alerts
	var decisions []Decision
	for _, alert := range alerts {
		decisions = append(decisions, alert.Decisions...)
	}
	if decisions == nil {
		decisions = []Decision{}
	}
	return decisions, nil
}

// AddDecision manually adds a ban decision
func (m *Manager) AddDecision(ip, duration, reason string) error {
	args := []string{"decisions", "add", "--ip", ip, "--duration", duration, "--reason", reason}
	cmd := exec.Command("cscli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add decision: %s", string(output))
	}
	return nil
}

// DeleteDecision removes a decision by ID
func (m *Manager) DeleteDecision(id int) error {
	cmd := exec.Command("cscli", "decisions", "delete", "--id", strconv.Itoa(id))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete decision: %s", string(output))
	}
	return nil
}
