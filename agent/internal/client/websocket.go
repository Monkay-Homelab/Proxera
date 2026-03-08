package client

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/proxera/agent/internal/config"
	"github.com/proxera/agent/pkg/crowdsec"
	"github.com/proxera/agent/pkg/deploy"
	"github.com/proxera/agent/pkg/metrics"
	"github.com/proxera/agent/pkg/nginx"
	"github.com/proxera/agent/pkg/types"
	"github.com/proxera/agent/pkg/version"
)

const (
	// Channel buffer sizes
	sendBufferSize = 256

	// Timeouts and intervals
	pongWait       = 90 * time.Second  // Max time to wait for pong before disconnect
	pingInterval   = 30 * time.Second  // How often to send pings to keep connection alive
	heartbeatRate  = 60 * time.Second  // How often to send heartbeat to backend
	httpIPTimeout  = 5 * time.Second   // Timeout for external IP lookup requests
	sendTimeout    = 10 * time.Second  // Max time to wait for send channel to accept response
	updateDelay    = 2 * time.Second   // Delay after update before exit for systemd restart

	// Default metrics collection interval
	defaultMetricsInterval = 5 * time.Minute
)

// ipClient is a reusable HTTP client for external IP lookups.
var ipClient = &http.Client{Timeout: httpIPTimeout}

// WSClient manages the WebSocket connection to the panel
type WSClient struct {
	conn            *websocket.Conn
	apiKey          string
	agentID         string
	send            chan []byte
	disconnect      chan struct{}
	deployer        *deploy.Deployer
	manager         *nginx.Manager
	collector       *metrics.Collector
	queue           *metrics.Queue
	crowdsec        *crowdsec.Manager
	metricsInterval atomic.Int64   // stores time.Duration as int64, atomic for goroutine safety (H21)
	metricsReset    chan struct{}
	cachedHostCount atomic.Int32   // cached deployed host count, invalidated on deploy/remove
	closeOnce       sync.Once // Ensures conn.Close() called exactly once (C7)
	disconnectOnce  sync.Once // Ensures close(disconnect) called exactly once (H20)
	sendOnce        sync.Once // Ensures close(send) called exactly once (H20)
	cmdWg           sync.WaitGroup // Tracks in-flight command handler goroutines (H18)
}

// SetDeployer sets the deployer and manager on the WSClient
func (ws *WSClient) SetDeployer(d *deploy.Deployer, m *nginx.Manager) {
	ws.deployer = d
	ws.manager = m
}

// SetCollector sets the metrics collector and initializes the persistent queue
func (ws *WSClient) SetCollector(c *metrics.Collector) {
	ws.collector = c
	ws.queue = metrics.NewQueue()
}

// SetCrowdSec sets the CrowdSec manager on the WSClient
func (ws *WSClient) SetCrowdSec(cs *crowdsec.Manager) {
	ws.crowdsec = cs
}

// SetMetricsInterval sets the metrics collection interval
func (ws *WSClient) SetMetricsInterval(seconds int) {
	if seconds <= 0 {
		seconds = 300
	}
	ws.metricsInterval.Store(int64(time.Duration(seconds) * time.Second))
	log.Printf("[metrics] Interval set to %d seconds", seconds)
}

// countDeployedHosts returns the cached host count. Use refreshHostCount() to update it.
func (ws *WSClient) countDeployedHosts() int {
	if n := ws.cachedHostCount.Load(); n >= 0 {
		return int(n)
	}
	return ws.refreshHostCount()
}

// refreshHostCount recounts Proxera-managed config files and updates the cache.
func (ws *WSClient) refreshHostCount() int {
	if ws.deployer == nil {
		ws.cachedHostCount.Store(0)
		return 0
	}
	matches, err := filepath.Glob(filepath.Join(ws.deployer.EnabledPath(), "proxera_*.conf"))
	if err != nil {
		ws.cachedHostCount.Store(0)
		return 0
	}
	ws.cachedHostCount.Store(int32(len(matches)))
	return len(matches)
}

// Message represents a WebSocket message
type Message struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// NewWSClient creates a new WebSocket client
func NewWSClient(wsURL, apiKey, agentID string) (*WSClient, error) {
	// Parse and prepare WebSocket URL
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	// Add authorization header
	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	client := &WSClient{
		conn:         conn,
		apiKey:       apiKey,
		agentID:      agentID,
		send:         make(chan []byte, sendBufferSize),
		disconnect:   make(chan struct{}),
		metricsReset: make(chan struct{}, 1),
	}
	client.metricsInterval.Store(int64(defaultMetricsInterval))
	client.cachedHostCount.Store(-1) // -1 = not yet computed

	log.Printf("[OK] Connected to panel WebSocket")
	return client, nil
}

// Start begins the WebSocket client read/write loops
func (ws *WSClient) Start() {
	go ws.readPump()
	go ws.writePump()
	go ws.heartbeat()
	go ws.metricsLoop()
}

// readPump reads messages from the panel
func (ws *WSClient) readPump() {
	defer func() {
		ws.disconnectOnce.Do(func() { close(ws.disconnect) })
		ws.closeOnce.Do(func() { ws.conn.Close() })
	}()

	// Set up read deadline and pong handler to keep connection alive
	ws.conn.SetReadDeadline(time.Now().Add(pongWait))
	ws.conn.SetPongHandler(func(string) error {
		ws.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := ws.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}

		// Parse message
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to parse message: %v", err)
			continue
		}

		// Handle different message types
		ws.handleMessage(&msg)
	}
}

// writePump writes messages to the panel
func (ws *WSClient) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		ws.closeOnce.Do(func() { ws.conn.Close() })
	}()

	for {
		select {
		case message, ok := <-ws.send:
			if !ok {
				ws.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := ws.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			if err := ws.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// heartbeat sends periodic heartbeat messages
func (ws *WSClient) heartbeat() {
	// Send immediately on startup so the panel has fresh data right away
	ws.SendHeartbeat("online", ws.countDeployedHosts())

	ticker := time.NewTicker(heartbeatRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ws.SendHeartbeat("online", ws.countDeployedHosts())
		case <-ws.disconnect:
			return
		}
	}
}

// metricsLoop periodically collects and pushes metrics to the panel
func (ws *WSClient) metricsLoop() {
	if ws.collector == nil {
		log.Println("[metrics] collector is nil, metricsLoop exiting")
		return
	}

	// Drain any backlog from previous sessions before entering the regular loop
	ws.drainBacklog()

	interval := time.Duration(ws.metricsInterval.Load())
	log.Printf("[metrics] metricsLoop started, collecting every %s", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ws.collectAndSendMetrics()
		case <-ws.metricsReset:
			ticker.Stop()
			interval = time.Duration(ws.metricsInterval.Load())
			ticker = time.NewTicker(interval)
			log.Printf("[metrics] metricsLoop reset to %s", interval)
		case <-ws.disconnect:
			log.Println("[metrics] metricsLoop stopping (disconnect)")
			ws.flushAndQueueRemaining()
			return
		}
	}
}

// collectAndSendMetrics collects new log entries and sends completed buckets.
// If the send fails, buckets are persisted to the on-disk queue.
func (ws *WSClient) collectAndSendMetrics() {
	if ws.collector == nil {
		return
	}

	// Try to drain any backlog before collecting fresh data
	ws.drainBacklog()

	ws.collector.Collect()
	buckets := ws.collector.Flush()
	log.Printf("[metrics] collected and flushed: %d buckets", len(buckets))
	if len(buckets) == 0 {
		return
	}

	if !ws.trySendMetricsBuckets(buckets) {
		if ws.queue != nil {
			if err := ws.queue.Enqueue(buckets); err != nil {
				log.Printf("[metrics] failed to queue buckets: %v", err)
			}
		}
	}
}

// trySendMetricsBuckets attempts to send metric buckets to the panel.
// Returns true if the data was accepted into the send channel.
func (ws *WSClient) trySendMetricsBuckets(buckets []metrics.MetricsBucket) bool {
	msg := Message{
		Type: "metrics_report",
		Payload: map[string]interface{}{
			"buckets": buckets,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal metrics report: %v", err)
		return false
	}

	select {
	case ws.send <- data:
		log.Printf("Sent metrics report with %d bucket(s)", len(buckets))
		return true
	default:
		log.Println("Send channel full, cannot send metrics report")
		return false
	}
}

// drainBacklog sends queued metrics from previous sessions (or failed sends).
// Stops draining if the send channel fills up, re-queuing the remainder.
func (ws *WSClient) drainBacklog() {
	if ws.queue == nil || !ws.queue.HasBacklog() {
		return
	}

	backlogSize := ws.queue.BacklogSize()
	log.Printf("[metrics] draining backlog (%d queued file(s))...", backlogSize)
	sent := 0

	for {
		buckets, err := ws.queue.Dequeue()
		if err != nil {
			log.Printf("[metrics] backlog dequeue error: %v", err)
			continue
		}
		if buckets == nil {
			break // queue empty
		}

		if !ws.trySendMetricsBuckets(buckets) {
			// Channel full — re-queue this batch and stop draining
			if err := ws.queue.Enqueue(buckets); err != nil {
				log.Printf("[metrics] failed to re-queue backlog: %v", err)
			}
			break
		}
		sent += len(buckets)
	}

	if sent > 0 {
		log.Printf("[metrics] drained %d backlog bucket(s)", sent)
	}
}

// flushAndQueueRemaining persists any in-memory buckets to the queue on disconnect
// so they survive agent restarts.
func (ws *WSClient) flushAndQueueRemaining() {
	if ws.collector == nil || ws.queue == nil {
		return
	}

	ws.collector.Collect()
	buckets := ws.collector.FlushAll()
	if len(buckets) == 0 {
		return
	}

	if err := ws.queue.Enqueue(buckets); err != nil {
		log.Printf("[metrics] failed to queue %d remaining bucket(s) on disconnect: %v", len(buckets), err)
	} else {
		log.Printf("[metrics] queued %d remaining bucket(s) before disconnect", len(buckets))
	}
}

// getOutboundIP gets the preferred outbound IP of this machine (LAN IP)
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// getPublicIP gets the public/WAN IP address by querying an external service
func getPublicIP() string {
	// Try multiple services for reliability
	services := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}

	for _, service := range services {
		resp, err := ipClient.Get(service)
		if err != nil {
			continue
		}

		if resp.StatusCode == 200 {
			body := make([]byte, 64)
			n, err := resp.Body.Read(body)
			resp.Body.Close()
			if err != nil && err != io.EOF {
				continue
			}
			ip := strings.TrimSpace(string(body[:n]))
			if ip != "" {
				return ip
			}
		} else {
			resp.Body.Close()
		}
	}

	return ""
}

// SendHeartbeat sends a heartbeat message to the panel
func (ws *WSClient) SendHeartbeat(status string, hostCount int) {
	lanIP := getOutboundIP()
	wanIP := getPublicIP()

	csInstalled := ws.crowdsec != nil && ws.crowdsec.IsInstalled()
	csRunning := ws.crowdsec != nil && ws.crowdsec.IsRunning()

	nginxVer := nginx.DetectNginxVersion()

	msg := Message{
		Type: "heartbeat",
		Payload: map[string]interface{}{
			"status":             status,
			"host_count":         hostCount,
			"version":            version.Version,
			"os":                 runtime.GOOS,
			"arch":               runtime.GOARCH,
			"lan_ip":             lanIP,
			"wan_ip":             wanIP,
			"ip_address":         lanIP, // Keep for backwards compatibility
			"crowdsec_installed": csInstalled,
			"crowdsec_running":   csRunning,
			"nginx_version":     nginxVer,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal heartbeat: %v", err)
		return
	}

	select {
	case ws.send <- data:
	default:
		log.Println("Send channel full, dropping heartbeat")
	}
}

// SendResponse sends a response to a command
func (ws *WSClient) SendResponse(commandType string, success bool, message string, errorMsg string) {
	msg := Message{
		Type: "response",
		Payload: map[string]interface{}{
			"command_type": commandType,
			"success":      success,
			"message":      message,
			"error":        errorMsg,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}

	select {
	case ws.send <- data:
	case <-ws.disconnect:
		log.Printf("Connection closed, dropping response for %s", commandType)
	case <-time.After(sendTimeout):
		log.Printf("Send channel full for %s, dropping response for %s", sendTimeout, commandType)
	}
}

// runCommand runs a command handler goroutine with panic recovery and WaitGroup tracking
func (ws *WSClient) runCommand(name string, fn func()) {
	ws.cmdWg.Add(1)
	go func() {
		defer ws.cmdWg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in command handler %s: %v", name, r)
			}
		}()
		fn()
	}()
}

// handleMessage handles incoming messages from the panel
func (ws *WSClient) handleMessage(msg *Message) {
	log.Printf("Received message: type=%s", msg.Type)

	switch msg.Type {
	case "apply_host":
		log.Println("Received apply_host command")
		ws.runCommand("apply_host", func() {
			if ws.deployer == nil {
				ws.SendResponse("apply_host", false, "", "deployer not initialized")
				return
			}

			// Parse host from payload
			payloadBytes, err := json.Marshal(msg.Payload)
			if err != nil {
				ws.SendResponse("apply_host", false, "", fmt.Sprintf("failed to marshal payload: %v", err))
				return
			}

			var host types.Host
			if err := json.Unmarshal(payloadBytes, &host); err != nil {
				ws.SendResponse("apply_host", false, "", fmt.Sprintf("failed to parse host: %v", err))
				return
			}

			if err := ws.deployer.ApplyHost(host); err != nil {
				log.Printf("Failed to apply host %s: %v", host.Domain, err)
				ws.SendResponse("apply_host", false, "", err.Error())
				return
			}

			ws.refreshHostCount()
			ws.SendResponse("apply_host", true, fmt.Sprintf("Host %s deployed successfully", host.Domain), "")
		})

	case "remove_host":
		log.Println("Received remove_host command")
		ws.runCommand("remove_host", func() {
			if ws.deployer == nil {
				ws.SendResponse("remove_host", false, "", "deployer not initialized")
				return
			}

			domain, _ := msg.Payload["domain"].(string)
			if domain == "" {
				ws.SendResponse("remove_host", false, "", "domain is required")
				return
			}

			if err := ws.deployer.RemoveHost(domain); err != nil {
				log.Printf("Failed to remove host %s: %v", domain, err)
				ws.SendResponse("remove_host", false, "", err.Error())
				return
			}

			ws.refreshHostCount()
			ws.SendResponse("remove_host", true, fmt.Sprintf("Host %s removed successfully", domain), "")
		})

	case "apply":
		log.Println("Received apply command")
		ws.runCommand("apply", func() {
			if ws.deployer == nil {
				ws.SendResponse("apply", false, "", "deployer not initialized")
				return
			}

			// Parse hosts array from payload
			hostsRaw, ok := msg.Payload["hosts"]
			if !ok {
				ws.SendResponse("apply", false, "", "hosts array is required")
				return
			}

			hostsBytes, err := json.Marshal(hostsRaw)
			if err != nil {
				ws.SendResponse("apply", false, "", fmt.Sprintf("failed to marshal hosts: %v", err))
				return
			}

			var hosts []types.Host
			if err := json.Unmarshal(hostsBytes, &hosts); err != nil {
				ws.SendResponse("apply", false, "", fmt.Sprintf("failed to parse hosts: %v", err))
				return
			}

			applied, err := ws.deployer.ApplyAll(hosts)
			if err != nil {
				log.Printf("Apply all failed: %v", err)
				ws.SendResponse("apply", false, "", err.Error())
				return
			}

			ws.refreshHostCount()
			ws.SendResponse("apply", true, fmt.Sprintf("Successfully applied %d host(s)", applied), "")
		})

	case "reload":
		log.Println("Received reload command")
		ws.runCommand("reload", func() {
			if ws.manager == nil {
				ws.SendResponse("reload", false, "", "nginx manager not initialized")
				return
			}

			if err := ws.manager.Test(); err != nil {
				ws.SendResponse("reload", false, "", fmt.Sprintf("nginx test failed: %v", err))
				return
			}

			if err := ws.manager.Reload(); err != nil {
				ws.SendResponse("reload", false, "", fmt.Sprintf("nginx reload failed: %v", err))
				return
			}

			ws.SendResponse("reload", true, "Nginx reloaded successfully", "")
		})

	case "update":
		// Handle update command
		log.Println("Received update command - starting self-update process")

		// Perform update in a goroutine to allow response to be sent
		ws.runCommand("update", func() {
			if err := version.SelfUpdate(); err != nil {
				log.Printf("Update failed: %v", err)
				ws.SendResponse("update", false, "", err.Error())
				return
			}

			// Update successful, send response before restarting
			ws.SendResponse("update", true, "Update completed, restarting agent", "")

			// Wait a moment for response to be sent
			time.Sleep(updateDelay)

			// Exit cleanly - systemd will restart us with the new binary
			log.Println("Update complete, exiting for systemd restart...")
			os.Exit(0)
		})

	case "get_metrics":
		// Handle on-demand metrics request
		log.Println("Received get_metrics command")
		ws.runCommand("get_metrics", func() {
			if ws.collector == nil {
				ws.SendResponse("get_metrics", false, "", "metrics collector not initialized")
				return
			}

			ws.collector.Collect()
			buckets := ws.collector.FlushAll()

			bucketsJSON, err := json.Marshal(buckets)
			if err != nil {
				ws.SendResponse("get_metrics", false, "", fmt.Sprintf("failed to marshal metrics: %v", err))
				return
			}

			ws.SendResponse("get_metrics", true, string(bucketsJSON), "")
		})

	case "set_metrics_interval":
		log.Println("Received set_metrics_interval command")
		ws.runCommand("set_metrics_interval", func() {
			intervalFloat, ok := msg.Payload["interval"].(float64)
			if !ok || intervalFloat <= 0 {
				ws.SendResponse("set_metrics_interval", false, "", "interval (seconds) is required and must be > 0")
				return
			}
			seconds := int(intervalFloat)
			ws.metricsInterval.Store(int64(time.Duration(seconds) * time.Second))
			log.Printf("[metrics] Interval updated to %d seconds", seconds)

			// Signal metricsLoop to restart with new interval
			select {
			case ws.metricsReset <- struct{}{}:
			default:
			}

			ws.SendResponse("set_metrics_interval", true, fmt.Sprintf("Metrics interval set to %d seconds", seconds), "")
		})

	case "upgrade_nginx":
		log.Println("Received upgrade_nginx command")
		ws.runCommand("upgrade_nginx", func() {
			// Step 1: CrowdSec nginx bouncer pre-check
			if _, err := os.Stat("/etc/nginx/conf.d/crowdsec_nginx.conf"); err == nil {
				ws.SendResponse("upgrade_nginx", false, "",
					"Cannot upgrade nginx: CrowdSec nginx bouncer is installed. "+
						"The Lua modules (libnginx-mod-http-lua) are incompatible with nginx.org packages. "+
						"Uninstall the CrowdSec bouncer first, or switch to the firewall bouncer (crowdsec-firewall-bouncer-iptables).")
				return
			}

			// Perform the full upgrade (migrate configs, upgrade package, cleanup)
			setup := nginx.NewSetup("/usr/sbin/nginx", "/etc/nginx/conf.d", "/etc/nginx/conf.d")
			migrated, err := setup.PerformNginxUpgrade()
			if err != nil {
				ws.SendResponse("upgrade_nginx", false, "", fmt.Sprintf("Nginx upgrade failed: %v", err))
				return
			}

			// Update proxera.yaml paths so agent uses conf.d on next restart
			const proxeraConfigPath = "/etc/proxera/proxera.yaml"
			if cfg, loadErr := config.Load(proxeraConfigPath); loadErr == nil {
				if cfg.NginxConfigPath != "/etc/nginx/conf.d" || cfg.NginxEnabledPath != "/etc/nginx/conf.d" {
					cfg.NginxConfigPath = "/etc/nginx/conf.d"
					cfg.NginxEnabledPath = "/etc/nginx/conf.d"
					if saveErr := config.Save(cfg, proxeraConfigPath); saveErr != nil {
						log.Printf("Warning: failed to update proxera.yaml paths: %v", saveErr)
					} else {
						log.Println("Updated proxera.yaml paths to /etc/nginx/conf.d")
					}
				}
			}

			msg := "Nginx upgraded successfully. Deploy All recommended to regenerate configs with new http2 directive."
			if migrated > 0 {
				msg = fmt.Sprintf("Nginx upgraded successfully, migrated %d config(s) from sites-available to conf.d. Deploy All recommended to regenerate configs.", migrated)
			}
			ws.SendResponse("upgrade_nginx", true, msg, "")
		})

	case "get_logs":
		// Handle get logs command
		log.Println("Received get_logs command")

		// Get logs asynchronously
		ws.runCommand("get_logs", func() {
			logs, err := ws.getSystemLogs()
			if err != nil {
				log.Printf("Failed to get logs: %v", err)
				ws.SendResponse("get_logs", false, "", err.Error())
				return
			}

			ws.SendResponse("get_logs", true, logs, "")
		})

	case "get_nginx_logs":
		log.Println("Received get_nginx_logs command")
		ws.runCommand("get_nginx_logs", func() {
			lines := 200
			if n, ok := msg.Payload["lines"].(float64); ok && n > 0 {
				lines = int(n)
			}
			logs, err := ws.getNginxLogs(lines)
			if err != nil {
				ws.SendResponse("get_nginx_logs", false, "", err.Error())
				return
			}
			ws.SendResponse("get_nginx_logs", true, logs, "")
		})

	case "crowdsec_install":
		log.Println("Received crowdsec_install command")
		ws.runCommand("crowdsec_install", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_install", false, "", "crowdsec manager not initialized")
				return
			}
			enrollmentKey, _ := msg.Payload["enrollment_key"].(string)
			if err := ws.crowdsec.Install(enrollmentKey); err != nil {
				ws.SendResponse("crowdsec_install", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_install", true, "CrowdSec installed successfully", "")
		})

	case "crowdsec_uninstall":
		log.Println("Received crowdsec_uninstall command")
		ws.runCommand("crowdsec_uninstall", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_uninstall", false, "", "crowdsec manager not initialized")
				return
			}
			if err := ws.crowdsec.Uninstall(); err != nil {
				ws.SendResponse("crowdsec_uninstall", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_uninstall", true, "CrowdSec uninstalled successfully", "")
		})

	case "crowdsec_status":
		log.Println("Received crowdsec_status command")
		ws.runCommand("crowdsec_status", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_status", false, "", "crowdsec manager not initialized")
				return
			}
			status, err := ws.crowdsec.Status()
			if err != nil {
				ws.SendResponse("crowdsec_status", false, "", err.Error())
				return
			}
			data, _ := json.Marshal(status)
			ws.SendResponse("crowdsec_status", true, string(data), "")
		})

	case "crowdsec_decisions_list":
		log.Println("Received crowdsec_decisions_list command")
		ws.runCommand("crowdsec_decisions_list", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_decisions_list", false, "", "crowdsec manager not initialized")
				return
			}
			decisions, err := ws.crowdsec.ListDecisions()
			if err != nil {
				ws.SendResponse("crowdsec_decisions_list", false, "", err.Error())
				return
			}
			data, _ := json.Marshal(decisions)
			ws.SendResponse("crowdsec_decisions_list", true, string(data), "")
		})

	case "crowdsec_decisions_add":
		log.Println("Received crowdsec_decisions_add command")
		ws.runCommand("crowdsec_decisions_add", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_decisions_add", false, "", "crowdsec manager not initialized")
				return
			}
			ip, _ := msg.Payload["ip"].(string)
			duration, _ := msg.Payload["duration"].(string)
			reason, _ := msg.Payload["reason"].(string)
			if ip == "" {
				ws.SendResponse("crowdsec_decisions_add", false, "", "ip is required")
				return
			}
			if duration == "" {
				duration = "24h"
			}
			if reason == "" {
				reason = "Manual ban via Proxera panel"
			}
			if err := ws.crowdsec.AddDecision(ip, duration, reason); err != nil {
				ws.SendResponse("crowdsec_decisions_add", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_decisions_add", true, fmt.Sprintf("IP %s banned for %s", ip, duration), "")
		})

	case "crowdsec_decisions_delete":
		log.Println("Received crowdsec_decisions_delete command")
		ws.runCommand("crowdsec_decisions_delete", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_decisions_delete", false, "", "crowdsec manager not initialized")
				return
			}
			idFloat, ok := msg.Payload["id"].(float64)
			if !ok {
				ws.SendResponse("crowdsec_decisions_delete", false, "", "id is required")
				return
			}
			if err := ws.crowdsec.DeleteDecision(int(idFloat)); err != nil {
				ws.SendResponse("crowdsec_decisions_delete", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_decisions_delete", true, "Decision deleted", "")
		})

	case "crowdsec_alerts_list":
		log.Println("Received crowdsec_alerts_list command")
		ws.runCommand("crowdsec_alerts_list", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_alerts_list", false, "", "crowdsec manager not initialized")
				return
			}
			alerts, err := ws.crowdsec.ListAlerts()
			if err != nil {
				ws.SendResponse("crowdsec_alerts_list", false, "", err.Error())
				return
			}
			data, _ := json.Marshal(alerts)
			ws.SendResponse("crowdsec_alerts_list", true, string(data), "")
		})

	case "crowdsec_alerts_delete":
		log.Println("Received crowdsec_alerts_delete command")
		ws.runCommand("crowdsec_alerts_delete", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_alerts_delete", false, "", "crowdsec manager not initialized")
				return
			}
			idFloat, ok := msg.Payload["id"].(float64)
			if !ok {
				ws.SendResponse("crowdsec_alerts_delete", false, "", "id is required")
				return
			}
			if err := ws.crowdsec.DeleteAlert(int(idFloat)); err != nil {
				ws.SendResponse("crowdsec_alerts_delete", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_alerts_delete", true, "Alert deleted", "")
		})

	case "crowdsec_collections_list":
		log.Println("Received crowdsec_collections_list command")
		ws.runCommand("crowdsec_collections_list", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_collections_list", false, "", "crowdsec manager not initialized")
				return
			}
			collections, err := ws.crowdsec.ListCollections()
			if err != nil {
				ws.SendResponse("crowdsec_collections_list", false, "", err.Error())
				return
			}
			data, _ := json.Marshal(collections)
			ws.SendResponse("crowdsec_collections_list", true, string(data), "")
		})

	case "crowdsec_collections_install":
		log.Println("Received crowdsec_collections_install command")
		ws.runCommand("crowdsec_collections_install", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_collections_install", false, "", "crowdsec manager not initialized")
				return
			}
			name, _ := msg.Payload["name"].(string)
			if name == "" {
				ws.SendResponse("crowdsec_collections_install", false, "", "name is required")
				return
			}
			if err := ws.crowdsec.InstallCollection(name); err != nil {
				ws.SendResponse("crowdsec_collections_install", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_collections_install", true, fmt.Sprintf("Collection %s installed", name), "")
		})

	case "crowdsec_collections_remove":
		log.Println("Received crowdsec_collections_remove command")
		ws.runCommand("crowdsec_collections_remove", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_collections_remove", false, "", "crowdsec manager not initialized")
				return
			}
			name, _ := msg.Payload["name"].(string)
			if name == "" {
				ws.SendResponse("crowdsec_collections_remove", false, "", "name is required")
				return
			}
			if err := ws.crowdsec.RemoveCollection(name); err != nil {
				ws.SendResponse("crowdsec_collections_remove", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_collections_remove", true, fmt.Sprintf("Collection %s removed", name), "")
		})

	case "crowdsec_bouncers_list":
		log.Println("Received crowdsec_bouncers_list command")
		ws.runCommand("crowdsec_bouncers_list", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_bouncers_list", false, "", "crowdsec manager not initialized")
				return
			}
			bouncers, err := ws.crowdsec.ListBouncers()
			if err != nil {
				ws.SendResponse("crowdsec_bouncers_list", false, "", err.Error())
				return
			}
			data, _ := json.Marshal(bouncers)
			ws.SendResponse("crowdsec_bouncers_list", true, string(data), "")
		})

	case "crowdsec_bouncer_install":
		log.Println("Received crowdsec_bouncer_install command")
		ws.runCommand("crowdsec_bouncer_install", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_bouncer_install", false, "", "crowdsec manager not initialized")
				return
			}
			pkg, _ := msg.Payload["package"].(string)
			if pkg == "" {
				ws.SendResponse("crowdsec_bouncer_install", false, "", "package name is required")
				return
			}
			if err := ws.crowdsec.InstallBouncer(pkg); err != nil {
				ws.SendResponse("crowdsec_bouncer_install", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_bouncer_install", true, fmt.Sprintf("Bouncer %s installed successfully", pkg), "")
		})

	case "crowdsec_bouncer_remove":
		log.Println("Received crowdsec_bouncer_remove command")
		ws.runCommand("crowdsec_bouncer_remove", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_bouncer_remove", false, "", "crowdsec manager not initialized")
				return
			}
			pkg, _ := msg.Payload["package"].(string)
			if pkg == "" {
				ws.SendResponse("crowdsec_bouncer_remove", false, "", "package name is required")
				return
			}
			if err := ws.crowdsec.RemoveBouncer(pkg); err != nil {
				ws.SendResponse("crowdsec_bouncer_remove", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_bouncer_remove", true, fmt.Sprintf("Bouncer %s removed successfully", pkg), "")
		})

	case "crowdsec_metrics":
		log.Println("Received crowdsec_metrics command")
		ws.runCommand("crowdsec_metrics", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_metrics", false, "", "crowdsec manager not initialized")
				return
			}
			metricsData, err := ws.crowdsec.GetMetrics()
			if err != nil {
				ws.SendResponse("crowdsec_metrics", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_metrics", true, metricsData, "")
		})

	case "crowdsec_whitelist_list":
		log.Println("Received crowdsec_whitelist_list command")
		ws.runCommand("crowdsec_whitelist_list", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_whitelist_list", false, "", "crowdsec manager not initialized")
				return
			}
			entries, err := ws.crowdsec.ListWhitelists()
			if err != nil {
				ws.SendResponse("crowdsec_whitelist_list", false, "", err.Error())
				return
			}
			data, _ := json.Marshal(entries)
			ws.SendResponse("crowdsec_whitelist_list", true, string(data), "")
		})

	case "crowdsec_whitelist_add":
		log.Println("Received crowdsec_whitelist_add command")
		ws.runCommand("crowdsec_whitelist_add", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_whitelist_add", false, "", "crowdsec manager not initialized")
				return
			}
			ip, _ := msg.Payload["ip"].(string)
			description, _ := msg.Payload["description"].(string)
			if ip == "" {
				ws.SendResponse("crowdsec_whitelist_add", false, "", "ip is required")
				return
			}
			entry := crowdsec.WhitelistEntry{IP: ip, Description: description}
			if err := ws.crowdsec.AddWhitelist(entry); err != nil {
				ws.SendResponse("crowdsec_whitelist_add", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_whitelist_add", true, fmt.Sprintf("IP %s whitelisted", ip), "")
		})

	case "crowdsec_whitelist_remove":
		log.Println("Received crowdsec_whitelist_remove command")
		ws.runCommand("crowdsec_whitelist_remove", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_whitelist_remove", false, "", "crowdsec manager not initialized")
				return
			}
			ip, _ := msg.Payload["ip"].(string)
			if ip == "" {
				ws.SendResponse("crowdsec_whitelist_remove", false, "", "ip is required")
				return
			}
			if err := ws.crowdsec.RemoveWhitelist(ip); err != nil {
				ws.SendResponse("crowdsec_whitelist_remove", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_whitelist_remove", true, fmt.Sprintf("IP %s removed from whitelist", ip), "")
		})

	case "crowdsec_get_ban_duration":
		log.Println("Received crowdsec_get_ban_duration command")
		ws.runCommand("crowdsec_get_ban_duration", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_get_ban_duration", false, "", "crowdsec manager not initialized")
				return
			}
			duration, err := ws.crowdsec.GetBanDuration()
			if err != nil {
				ws.SendResponse("crowdsec_get_ban_duration", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_get_ban_duration", true, duration, "")
		})

	case "crowdsec_set_ban_duration":
		log.Println("Received crowdsec_set_ban_duration command")
		ws.runCommand("crowdsec_set_ban_duration", func() {
			if ws.crowdsec == nil {
				ws.SendResponse("crowdsec_set_ban_duration", false, "", "crowdsec manager not initialized")
				return
			}
			duration, _ := msg.Payload["duration"].(string)
			if duration == "" {
				ws.SendResponse("crowdsec_set_ban_duration", false, "", "duration is required")
				return
			}
			if err := ws.crowdsec.SetBanDuration(duration); err != nil {
				ws.SendResponse("crowdsec_set_ban_duration", false, "", err.Error())
				return
			}
			ws.SendResponse("crowdsec_set_ban_duration", true, fmt.Sprintf("Ban duration updated to %s", duration), "")
		})

	case "list_backups":
		log.Println("Received list_backups command")
		ws.runCommand("list_backups", func() {
			if ws.manager == nil {
				ws.SendResponse("list_backups", false, "", "nginx manager not initialized")
				return
			}
			domain, _ := msg.Payload["domain"].(string)
			if domain == "" {
				ws.SendResponse("list_backups", false, "", "domain is required")
				return
			}
			backups, err := ws.manager.ListBackups(domain)
			if err != nil {
				ws.SendResponse("list_backups", false, "", err.Error())
				return
			}
			data, _ := json.Marshal(backups)
			ws.SendResponse("list_backups", true, string(data), "")
		})

	case "get_backup":
		log.Println("Received get_backup command")
		ws.runCommand("get_backup", func() {
			if ws.manager == nil {
				ws.SendResponse("get_backup", false, "", "nginx manager not initialized")
				return
			}
			domain, _ := msg.Payload["domain"].(string)
			filename, _ := msg.Payload["filename"].(string)
			if domain == "" || filename == "" {
				ws.SendResponse("get_backup", false, "", "domain and filename are required")
				return
			}
			content, err := ws.manager.GetBackupContent(domain, filename)
			if err != nil {
				ws.SendResponse("get_backup", false, "", err.Error())
				return
			}
			ws.SendResponse("get_backup", true, content, "")
		})

	case "restore_backup":
		log.Println("Received restore_backup command")
		ws.runCommand("restore_backup", func() {
			if ws.manager == nil {
				ws.SendResponse("restore_backup", false, "", "nginx manager not initialized")
				return
			}
			domain, _ := msg.Payload["domain"].(string)
			filename, _ := msg.Payload["filename"].(string)
			if domain == "" || filename == "" {
				ws.SendResponse("restore_backup", false, "", "domain and filename are required")
				return
			}
			if err := ws.manager.RestoreSpecificBackup(domain, filename); err != nil {
				ws.SendResponse("restore_backup", false, "", err.Error())
				return
			}
			ws.SendResponse("restore_backup", true, fmt.Sprintf("Backup %s restored for %s", filename, domain), "")
		})

	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

// getNginxLogs retrieves recent nginx access logs
func (ws *WSClient) getNginxLogs(lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 1000 {
		lines = 1000
	}

	logFile := "/var/log/nginx/crowdsec_access.log"
	cmd := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to read nginx logs: %w", err)
	}

	return string(output), nil
}

func (ws *WSClient) getSystemLogs() (string, error) {
	// Try to get logs from systemd journal
	cmd := exec.Command("journalctl", "-u", "proxera", "-n", "100", "--no-pager")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get systemd logs: %w", err)
	}

	return string(output), nil
}

// Close closes the WebSocket connection and waits for in-flight commands
func (ws *WSClient) Close() {
	ws.sendOnce.Do(func() { close(ws.send) })
	ws.closeOnce.Do(func() { ws.conn.Close() })
	ws.cmdWg.Wait()
}

// Wait waits for the WebSocket connection to close
func (ws *WSClient) Wait() {
	<-ws.disconnect
}

// DisconnectChan returns the disconnect channel for select statements
func (ws *WSClient) DisconnectChan() <-chan struct{} {
	return ws.disconnect
}
