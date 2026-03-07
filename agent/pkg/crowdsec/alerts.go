package crowdsec

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// AlertSource represents the source IP info in an alert
type AlertSource struct {
	IP      string  `json:"ip"`
	Scope   string  `json:"scope"`
	Value   string  `json:"value"`
	ASName  string  `json:"as_name"`
	ASNum   string  `json:"as_number"`
	Country string  `json:"cn"`
	Range   string  `json:"range"`
	Lat     float64 `json:"latitude"`
	Lon     float64 `json:"longitude"`
}

// Alert represents a CrowdSec alert
type Alert struct {
	ID          int         `json:"id"`
	CreatedAt   string      `json:"created_at"`
	Scenario    string      `json:"scenario"`
	Message     string      `json:"message"`
	Source      AlertSource `json:"source"`
	EventsCount int        `json:"events_count"`
	StartAt     string      `json:"start_at"`
	StopAt      string      `json:"stop_at"`
	Remediation bool        `json:"remediation"`
	Simulated   bool        `json:"simulated"`
}

// ListAlerts returns all alerts
func (m *Manager) ListAlerts() ([]Alert, error) {
	cmd := exec.Command("cscli", "alerts", "list", "-o", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return []Alert{}, nil
	}

	var alerts []Alert
	if err := json.Unmarshal(output, &alerts); err != nil {
		return []Alert{}, nil
	}
	if alerts == nil {
		alerts = []Alert{}
	}
	return alerts, nil
}

// DeleteAlert removes an alert by ID
func (m *Manager) DeleteAlert(id int) error {
	cmd := exec.Command("cscli", "alerts", "delete", "--id", strconv.Itoa(id))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete alert: %s", string(output))
	}
	return nil
}
