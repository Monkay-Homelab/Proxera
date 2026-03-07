package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/models"
)

// AgentConnection represents an active agent WebSocket connection
type AgentConnection struct {
	Agent  *models.Agent
	Conn   *websocket.Conn
	Send   chan []byte
	mu     sync.Mutex
}

// PendingResponse tracks a pending command response
type PendingResponse struct {
	ResponseChan chan string
	ErrorChan    chan error
}

// Hub maintains active agent connections
type Hub struct {
	agents           map[string]*AgentConnection
	register         chan *AgentConnection
	unregister       chan *AgentConnection
	pendingResponses map[string]*PendingResponse // agentID -> pending response
	mu               sync.RWMutex
}

var hub = &Hub{
	agents:           make(map[string]*AgentConnection),
	register:         make(chan *AgentConnection),
	unregister:       make(chan *AgentConnection),
	pendingResponses: make(map[string]*PendingResponse),
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.agents[conn.Agent.AgentID] = conn
			h.mu.Unlock()
			log.Printf("Agent registered: %s (%s)", conn.Agent.Name, conn.Agent.AgentID)

			// Update agent status to online (preserves existing IP data)
			UpdateAgentStatus(conn.Agent.AgentID, "online")

			// Resolve any open agent_offline alerts
			go triggerAgentOnlineResolution(conn.Agent.AgentID, conn.Agent.UserID)

			// Send stored metrics interval to agent
			go func(c *AgentConnection) {
				var interval int
				err := database.DB.QueryRow(context.Background(),
					`SELECT COALESCE(metrics_interval, 300) FROM agents WHERE agent_id = $1`, c.Agent.AgentID,
				).Scan(&interval)
				if err == nil && interval != 300 {
					cmd := models.AgentCommand{
						Type:    "set_metrics_interval",
						Payload: map[string]interface{}{"interval": interval},
					}
					data, err := json.Marshal(cmd)
					if err == nil {
						select {
						case c.Send <- data:
							log.Printf("Sent stored metrics_interval=%d to agent %s", interval, c.Agent.AgentID)
						default:
						}
					}
				}
			}(conn)

		case conn := <-h.unregister:
			h.mu.Lock()
			if existing, ok := h.agents[conn.Agent.AgentID]; ok && existing == conn {
				delete(h.agents, conn.Agent.AgentID)
				close(conn.Send)
				log.Printf("Agent unregistered: %s (%s)", conn.Agent.Name, conn.Agent.AgentID)

				// Update agent status to offline (preserves existing IP data)
				UpdateAgentStatus(conn.Agent.AgentID, "offline")

				// Trigger agent offline alert
				go func(a *models.Agent) {
					var wanIP string
					database.DB.QueryRow(context.Background(),
						`SELECT COALESCE(wan_ip, '') FROM agents WHERE agent_id = $1`, a.AgentID,
					).Scan(&wanIP)
					triggerAgentOfflineAlert(a.AgentID, a.UserID, a.Name, wanIP, time.Now())
				}(conn.Agent)
			}
			h.mu.Unlock()

		}
	}
}

// safeSend sends data to a channel, recovering from panic if the channel is closed.
// Returns true if the send succeeded, false if the channel was closed.
func safeSend(ch chan []byte, data []byte) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	select {
	case ch <- data:
		return true
	case <-time.After(CmdTimeoutFast):
		return false
	}
}

// SendCommandToAgent sends a command to a specific agent via WebSocket
// Returns an error if the agent is not currently connected
// Does not wait for a response - use SendCommandAndWaitForResponse for synchronous commands
func SendCommandToAgent(agentID string, command models.AgentCommand) error {
	hub.mu.RLock()
	conn, ok := hub.agents[agentID]
	hub.mu.RUnlock()

	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Agent not connected")
	}

	data, err := json.Marshal(command)
	if err != nil {
		return err
	}

	if !safeSend(conn.Send, data) {
		return fmt.Errorf("agent %s: send channel closed or full", agentID)
	}
	return nil
}

// SendCommandAndWaitForResponse sends a command and waits for the response
// Blocks until the agent responds or the timeout is reached
// Used for commands that require synchronous responses like log retrieval
func SendCommandAndWaitForResponse(agentID string, command models.AgentCommand, timeout time.Duration) (string, error) {
	hub.mu.Lock()
	conn, ok := hub.agents[agentID]
	if !ok {
		hub.mu.Unlock()
		return "", fmt.Errorf("agent not connected")
	}

	// Create response channels
	pending := &PendingResponse{
		ResponseChan: make(chan string, 1),
		ErrorChan:    make(chan error, 1),
	}

	// Store pending response
	responseKey := fmt.Sprintf("%s:%s", agentID, command.Type)
	hub.pendingResponses[responseKey] = pending
	hub.mu.Unlock()

	// Clean up after we're done
	defer func() {
		hub.mu.Lock()
		delete(hub.pendingResponses, responseKey)
		hub.mu.Unlock()
	}()

	// Send command
	data, err := json.Marshal(command)
	if err != nil {
		return "", err
	}

	if !safeSend(conn.Send, data) {
		return "", fmt.Errorf("agent %s: send channel closed or full", agentID)
	}

	// Wait for response or timeout
	select {
	case response := <-pending.ResponseChan:
		return response, nil
	case err := <-pending.ErrorChan:
		return "", err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout waiting for agent response")
	}
}

// StartHub initializes the WebSocket hub
func StartHub() {
	go hub.Run()
}

// AgentWebSocket handles WebSocket connections from agents
func AgentWebSocket(c *websocket.Conn) {
	// Get agent from context (set by middleware)
	agent, ok := c.Locals("agent").(*models.Agent)
	if !ok || agent == nil {
		log.Printf("WebSocket connection rejected: invalid agent context")
		return
	}

	conn := &AgentConnection{
		Agent: agent,
		Conn:  c,
		Send:  make(chan []byte, 256),
	}

	hub.register <- conn

	// Start goroutines for reading and writing
	go conn.writePump()
	conn.readPump()
}

// readPump reads messages from the agent WebSocket connection
// Handles heartbeats, command responses, and keeps the connection alive with pong messages
// Automatically unregisters the agent when the connection closes
func (ac *AgentConnection) readPump() {
	defer func() {
		hub.unregister <- ac
		ac.Conn.Close()
	}()

	ac.Conn.SetReadDeadline(time.Now().Add(WSPongTimeout))
	ac.Conn.SetPongHandler(func(string) error {
		ac.Conn.SetReadDeadline(time.Now().Add(WSPongTimeout))
		return nil
	})

	for {
		_, message, err := ac.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming messages from agent
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to parse agent message: %v", err)
			continue
		}

		// Handle different message types
		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "heartbeat":
			// Update agent heartbeat
			payload, ok := msg["payload"].(map[string]interface{})
			if !ok {
				continue
			}

			status := "online"
			if s, ok := payload["status"].(string); ok {
				status = s
			}

			hostCount := 0
			if hc, ok := payload["host_count"].(float64); ok {
				hostCount = int(hc)
			}

			version := ac.Agent.Version
			if v, ok := payload["version"].(string); ok && v != "" {
				version = v
			}

			os := ac.Agent.OS
			if o, ok := payload["os"].(string); ok && o != "" {
				os = o
			}

			arch := ac.Agent.Arch
			if a, ok := payload["arch"].(string); ok && a != "" {
				arch = a
			}

			ipAddress := ac.Agent.IPAddress
			if ip, ok := payload["ip_address"].(string); ok && ip != "" {
				ipAddress = ip
			}

			lanIP := ""
			if lip, ok := payload["lan_ip"].(string); ok && lip != "" {
				lanIP = lip
			}

			wanIP := ""
			if wip, ok := payload["wan_ip"].(string); ok && wip != "" {
				wanIP = wip
			}

			// Detect WAN IP change and trigger DDNS updates
			if wanIP != "" {
				var currentWanIP string
				err := database.DB.QueryRow(context.Background(),
					`SELECT COALESCE(wan_ip, '') FROM agents WHERE id = $1`, ac.Agent.ID,
				).Scan(&currentWanIP)
				if err == nil && currentWanIP != wanIP {
					log.Printf("[DDNS] Agent %s WAN IP changed: %s -> %s", ac.Agent.AgentID, currentWanIP, wanIP)
					go UpdateDDNSForAgent(ac.Agent.ID, ac.Agent.UserID, wanIP)
				}
			}

			// Parse CrowdSec status
			csInstalled := false
			if cs, ok := payload["crowdsec_installed"].(bool); ok {
				csInstalled = cs
			}

			nginxVersion := ac.Agent.NginxVersion
			if nv, ok := payload["nginx_version"].(string); ok && nv != "" {
				nginxVersion = nv
			}

			if err := UpdateAgentHeartbeat(ac.Agent.AgentID, status, hostCount, ipAddress, version, os, arch, lanIP, wanIP, csInstalled, nginxVersion); err != nil {
				log.Printf("Failed to update agent heartbeat: %v", err)
			}

		case "response":
			// Handle command response from agent
			log.Printf("Agent response: %v", msg)

			// Extract response details
			payload, ok := msg["payload"].(map[string]interface{})
			if !ok {
				continue
			}

			commandType, ok := payload["command_type"].(string)
			if !ok {
				continue
			}

			success, ok := payload["success"].(bool)
			if !ok {
				continue
			}

			// Check if there's a pending response for this command
			responseKey := fmt.Sprintf("%s:%s", ac.Agent.AgentID, commandType)
			hub.mu.RLock()
			pending, exists := hub.pendingResponses[responseKey]
			hub.mu.RUnlock()

			if exists {
				if success {
					// Get the message/logs from the response
					if message, ok := payload["message"].(string); ok {
						pending.ResponseChan <- message
					} else {
						pending.ErrorChan <- fmt.Errorf("no message in response")
					}
				} else {
					// Get error message
					if errorMsg, ok := payload["error"].(string); ok {
						pending.ErrorChan <- errors.New(errorMsg)
					} else {
						pending.ErrorChan <- fmt.Errorf("command failed")
					}
				}
			} else {
				log.Printf("[ws] Late response for %s (no pending entry, likely timed out)", responseKey)
			}

		case "metrics_report":
			// Handle metrics data from agent
			log.Printf("[metrics] Received metrics_report from agent %s", ac.Agent.AgentID)
			payload, ok := msg["payload"].(map[string]interface{})
			if !ok {
				log.Printf("[metrics] Invalid payload from agent %s", ac.Agent.AgentID)
				continue
			}

			bucketsRaw, ok := payload["buckets"]
			if !ok {
				log.Printf("[metrics] No buckets field in metrics_report from agent %s", ac.Agent.AgentID)
				continue
			}

			bucketsJSON, err := json.Marshal(bucketsRaw)
			if err != nil {
				log.Printf("Failed to marshal metrics buckets: %v", err)
				continue
			}

			var buckets []models.IncomingMetricsBucket

			if err := json.Unmarshal(bucketsJSON, &buckets); err != nil {
				log.Printf("Failed to parse metrics buckets: %v", err)
				continue
			}

			if len(buckets) == 0 {
				continue
			}

			// Batch insert into metrics table
			if err := InsertMetricsBuckets(ac.Agent.AgentID, buckets); err != nil {
				log.Printf("Failed to insert metrics: %v", err)
			} else {
				log.Printf("Inserted %d metrics bucket(s) for agent %s", len(buckets), ac.Agent.AgentID)
			}

			// Insert visitor IP data
			if err := InsertVisitorIPs(ac.Agent.AgentID, buckets); err != nil {
				log.Printf("Failed to insert visitor IPs: %v", err)
			}

			// Evaluate metric-based alerts
			go evaluateErrorRateAlerts(ac.Agent.AgentID, ac.Agent.UserID, buckets)
			go evaluateHighLatencyAlerts(ac.Agent.AgentID, ac.Agent.UserID, buckets)
			go evaluateTrafficSpikeAlerts(ac.Agent.AgentID, ac.Agent.UserID, buckets)
			go evaluateHostDownAlerts(ac.Agent.AgentID, ac.Agent.UserID, buckets)
			go evaluateBandwidthAlerts(ac.Agent.AgentID, ac.Agent.UserID, buckets)

		default:
			log.Printf("Unknown message type from agent: %s", msgType)
		}
	}
}

// writePump writes messages to the agent
func (ac *AgentConnection) writePump() {
	ticker := time.NewTicker(WSPingInterval)
	defer func() {
		ticker.Stop()
		ac.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-ac.Send:
			ac.Conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
			if !ok {
				ac.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := ac.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			ac.Conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
			if err := ac.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
