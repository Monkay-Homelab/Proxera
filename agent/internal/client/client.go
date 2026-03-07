package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/proxera/agent/pkg/version"
)

// Client handles communication with the Proxera panel
type Client struct {
	PanelURL string
	APIKey   string
	AgentID  string
}

// NewClient creates a new panel client
func NewClient(panelURL, apiKey, agentID string) *Client {
	return &Client{
		PanelURL: panelURL,
		APIKey:   apiKey,
		AgentID:  agentID,
	}
}

// RegisterRequest is the request to register an agent
type RegisterRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// RegisterResponse is returned when registering
type RegisterResponse struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
	WSURL   string `json:"ws_url"`
}

// Register registers the agent with the panel
func Register(panelURL, bearerToken, agentName string) (*RegisterResponse, error) {
	url := fmt.Sprintf("%s/api/agents/register", panelURL)

	req := RegisterRequest{
		Name:    agentName,
		Version: version.Version,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearerToken))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	var registerResp RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&registerResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &registerResp, nil
}
