// Package localagent embeds agent logic for the AIO Control Node binary.
// It manages local nginx and CrowdSec via direct function calls instead of WebSocket.
package localagent

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/proxera/agent/pkg/crowdsec"
	"github.com/proxera/agent/pkg/deploy"
	"github.com/proxera/agent/pkg/metrics"
	"github.com/proxera/agent/pkg/nginx"
	"github.com/proxera/agent/pkg/types"
	"github.com/proxera/agent/pkg/version"
	"github.com/proxera/backend/internal/database"
)

// MetricsInsertFunc is a callback for inserting metrics into the database.
// Set by the control node main.go to point to handlers.InsertMetricsBuckets.
type MetricsInsertFunc func(agentID string, buckets []interface{}) error

// DDNSUpdateFunc is a callback for updating DNS records when the agent's WAN IP changes.
// Set by the control node main.go to point to handlers.UpdateDDNSForAgent.
type DDNSUpdateFunc func(agentDBID int, agentUserID int, newWanIP string)

// Manager provides direct access to agent functionality for the local node.
type Manager struct {
	agentID   string
	nginxMgr  *nginx.Manager
	deployer  *deploy.Deployer
	collector *metrics.Collector
	crowdsec  *crowdsec.Manager
	logDir    string
	isDocker  bool

	bouncerSync     *crowdsec.BouncerSync
	metricsInterval time.Duration
	metricsInsert   MetricsInsertFunc
	ddnsUpdate      DDNSUpdateFunc
	lastWanIP       string
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// Config holds local agent configuration.
type Config struct {
	NginxBinary      string
	NginxConfigPath  string
	NginxEnabledPath string
	NginxLogDir      string
	NginxReloadCmd   string // override for Docker: e.g. "docker exec proxera-nginx-1 nginx -s reload"
	NginxTestCmd     string // override for Docker: e.g. "docker exec proxera-nginx-1 nginx -t"
	MetricsInterval  time.Duration
}

// DefaultConfig returns the default local agent configuration.
// Nginx paths are auto-detected from common locations.
func DefaultConfig() Config {
	binary := detectNginxBinary()
	configPath, enabledPath := detectNginxPaths()
	logDir := detectNginxLogDir()

	return Config{
		NginxBinary:      binary,
		NginxConfigPath:  configPath,
		NginxEnabledPath: enabledPath,
		NginxLogDir:      logDir,
		MetricsInterval:  5 * time.Minute,
	}
}

// detectNginxBinary finds the nginx binary.
func detectNginxBinary() string {
	if path, err := exec.LookPath("nginx"); err == nil {
		return path
	}
	for _, p := range []string{"/usr/sbin/nginx", "/usr/local/sbin/nginx", "/usr/local/bin/nginx"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return "/usr/sbin/nginx"
}

// detectNginxPaths finds nginx config and enabled directories.
// Returns (configPath, enabledPath).
func detectNginxPaths() (string, string) {
	// Debian/Ubuntu style: sites-available + sites-enabled
	if dirExists("/etc/nginx/sites-available") && dirExists("/etc/nginx/sites-enabled") {
		return "/etc/nginx/sites-available", "/etc/nginx/sites-enabled"
	}
	// Standard conf.d (most distros, Docker)
	if dirExists("/etc/nginx/conf.d") {
		return "/etc/nginx/conf.d", "/etc/nginx/conf.d"
	}
	// FreeBSD / Homebrew
	if dirExists("/usr/local/etc/nginx/conf.d") {
		return "/usr/local/etc/nginx/conf.d", "/usr/local/etc/nginx/conf.d"
	}
	return "/etc/nginx/conf.d", "/etc/nginx/conf.d"
}

// detectNginxLogDir finds the nginx log directory.
func detectNginxLogDir() string {
	for _, d := range []string{"/var/log/nginx", "/usr/local/var/log/nginx"} {
		if dirExists(d) {
			return d
		}
	}
	return "/var/log/nginx"
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// New creates a new local agent manager.
func New(cfg Config) *Manager {
	nginxMgr := nginx.NewManager(cfg.NginxBinary, cfg.NginxConfigPath, cfg.NginxEnabledPath)
	if cfg.NginxReloadCmd != "" {
		nginxMgr.SetReloadCmd(cfg.NginxReloadCmd)
	}
	if cfg.NginxTestCmd != "" {
		nginxMgr.SetTestCmd(cfg.NginxTestCmd)
	}
	deployer := deploy.NewDeployer(nginxMgr, cfg.NginxConfigPath, cfg.NginxEnabledPath)
	collector := metrics.NewCollector(cfg.NginxLogDir)
	csMgr := crowdsec.NewManager(os.Getenv("CROWDSEC_CONTAINER"))

	// In Docker mode, start a bouncer sync that generates nginx geo blocklist
	var bs *crowdsec.BouncerSync
	if csMgr.IsDocker() {
		bs = crowdsec.NewBouncerSync(csMgr, cfg.NginxConfigPath, nginxMgr.Test, nginxMgr.Reload)
	}

	return &Manager{
		nginxMgr:         nginxMgr,
		deployer:         deployer,
		collector:        collector,
		crowdsec:         csMgr,
		logDir:           cfg.NginxLogDir,
		isDocker:         os.Getenv("PROXERA_DOCKER") == "true",
		bouncerSync:      bs,
		metricsInterval:  cfg.MetricsInterval,
		stopCh:           make(chan struct{}),
	}
}

// RegisterLocalAgent creates or retrieves the local agent record in the database.
// Returns the agent ID.
func (m *Manager) RegisterLocalAgent() (string, error) {
	ctx := context.Background()

	// Check if local agent already exists
	var agentID string
	err := database.DB.QueryRow(ctx,
		`SELECT agent_id FROM agents WHERE is_local = true LIMIT 1`,
	).Scan(&agentID)
	if err == nil {
		m.agentID = agentID
		log.Printf("[local-agent] Found existing local agent: %s", agentID)
		return agentID, nil
	}

	// Get the admin user (first user) to own the local agent
	var userID int
	err = database.DB.QueryRow(ctx,
		`SELECT id FROM users WHERE role = 'admin' ORDER BY id ASC LIMIT 1`,
	).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("no admin user found; register an admin account first")
	}

	// Create local agent with a placeholder api_key (required NOT NULL column)
	agentID = fmt.Sprintf("local-%d", time.Now().UnixNano())
	localAPIKey := fmt.Sprintf("local_%s", agentID)
	_, err = database.DB.Exec(ctx,
		`INSERT INTO agents (agent_id, name, user_id, api_key, status, is_local, version, os, arch, created_at, updated_at, last_heartbeat)
		 VALUES ($1, $2, $3, $4, 'online', true, $5, $6, $7, NOW(), NOW(), NOW())`,
		agentID, "Local (Control Node)", userID, localAPIKey, version.Version, runtime.GOOS, runtime.GOARCH,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create local agent: %w", err)
	}

	m.agentID = agentID
	log.Printf("[local-agent] Created local agent: %s (owner: user %d)", agentID, userID)
	return agentID, nil
}

// Start begins the local agent background tasks (heartbeat, metrics collection, bouncer sync).
func (m *Manager) Start() {
	m.wg.Add(2)
	go m.heartbeatLoop()
	go m.metricsLoop()
	if m.bouncerSync != nil {
		m.bouncerSync.Start()
	}
	log.Println("[local-agent] Started local agent manager")
}

// Stop gracefully shuts down the local agent.
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	if m.bouncerSync != nil {
		m.bouncerSync.Stop()
	}
	log.Println("[local-agent] Stopped local agent manager")
}

// heartbeatLoop periodically updates the local agent's status in the database.
func (m *Manager) heartbeatLoop() {
	defer m.wg.Done()

	m.sendHeartbeat()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.sendHeartbeat()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) sendHeartbeat() {
	if m.agentID == "" {
		return
	}

	hostCount := m.countDeployedHosts()
	nginxVer := nginx.DetectNginxVersion()
	csInstalled := m.crowdsec != nil && m.crowdsec.IsInstalled()
	lanIP := getOutboundIP()
	wanIP := getPublicIP()

	// Check for WAN IP change and trigger DDNS update
	if wanIP != "" && wanIP != m.lastWanIP && m.lastWanIP != "" && m.ddnsUpdate != nil {
		log.Printf("[local-agent] WAN IP changed: %s -> %s, triggering DDNS update", m.lastWanIP, wanIP)
		var agentDBID, agentUserID int
		err := database.DB.QueryRow(context.Background(),
			`SELECT id, user_id FROM agents WHERE agent_id = $1`, m.agentID,
		).Scan(&agentDBID, &agentUserID)
		if err == nil {
			go m.ddnsUpdate(agentDBID, agentUserID, wanIP)
		}
	}
	if wanIP != "" {
		m.lastWanIP = wanIP
	}

	ctx := context.Background()
	_, err := database.DB.Exec(ctx,
		`UPDATE agents SET
			status = 'online',
			host_count = $1,
			version = $2,
			os = $3,
			arch = $4,
			nginx_version = $5,
			crowdsec_installed = $6,
			lan_ip = $7,
			wan_ip = $8,
			ip_address = $7,
			last_seen = NOW(),
			last_heartbeat = NOW(),
			updated_at = NOW()
		 WHERE agent_id = $9`,
		hostCount, version.Version, runtime.GOOS, runtime.GOARCH, nginxVer, csInstalled, lanIP, wanIP, m.agentID,
	)
	if err != nil {
		log.Printf("[local-agent] heartbeat update failed: %v", err)
	}
}

// getOutboundIP gets the preferred outbound IP of this machine (LAN IP).
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// getPublicIP gets the public/WAN IP by querying an external service.
func getPublicIP() string {
	client := &http.Client{Timeout: 5 * time.Second}
	services := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}
	for _, svc := range services {
		resp, err := client.Get(svc)
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
			if net.ParseIP(ip) != nil {
				return ip
			}
		} else {
			resp.Body.Close()
		}
	}
	return ""
}

func (m *Manager) countDeployedHosts() int {
	matches, err := filepath.Glob(filepath.Join(m.deployer.EnabledPath(), "proxera_*.conf"))
	if err != nil {
		return 0
	}
	return len(matches)
}

// metricsLoop periodically collects and inserts metrics directly into the database.
func (m *Manager) metricsLoop() {
	defer m.wg.Done()

	if m.collector == nil {
		return
	}

	ticker := time.NewTicker(m.metricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.collectAndInsertMetrics()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) collectAndInsertMetrics() {
	if m.collector == nil || m.agentID == "" || m.metricsInsert == nil {
		return
	}

	m.collector.Collect()
	buckets := m.collector.Flush()
	if len(buckets) == 0 {
		return
	}

	// Convert to generic interface slice for the callback
	generic := make([]interface{}, len(buckets))
	for i, b := range buckets {
		generic[i] = b
	}

	if err := m.metricsInsert(m.agentID, generic); err != nil {
		log.Printf("[local-agent] metrics insert failed: %v", err)
	} else {
		log.Printf("[local-agent] Inserted %d metrics bucket(s)", len(buckets))
	}
}

// SetMetricsInsert sets the callback function for inserting metrics.
func (m *Manager) SetMetricsInsert(fn MetricsInsertFunc) {
	m.metricsInsert = fn
}

// SetDDNSUpdate sets the callback function for DDNS updates when WAN IP changes.
func (m *Manager) SetDDNSUpdate(fn DDNSUpdateFunc) {
	m.ddnsUpdate = fn
}

// --- Command execution (direct function calls, no WebSocket) ---

// ApplyHost deploys a host configuration to the local nginx.
func (m *Manager) ApplyHost(host types.Host) error {
	return m.deployer.ApplyHost(host)
}

// RemoveHost removes a host configuration from local nginx.
func (m *Manager) RemoveHost(domain string) error {
	return m.deployer.RemoveHost(domain)
}

// ApplyAll synchronizes all host configurations.
func (m *Manager) ApplyAll(hosts []types.Host) (int, error) {
	return m.deployer.ApplyAll(hosts)
}

// Reload tests and reloads nginx.
func (m *Manager) Reload() error {
	if err := m.nginxMgr.Test(); err != nil {
		return fmt.Errorf("nginx test failed: %w", err)
	}
	return m.nginxMgr.Reload()
}

// GetNginxLogs retrieves recent nginx access logs.
func (m *Manager) GetNginxLogs(lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 1000 {
		lines = 1000
	}
	logPath := filepath.Join(m.logDir, "crowdsec_access.log")
	cmd := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fall back to default access.log
		logPath = filepath.Join(m.logDir, "access.log")
		cmd = exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logPath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to read nginx logs: %w", err)
		}
	}
	return string(output), nil
}

// GetSystemLogs retrieves recent system logs for the proxera service.
func (m *Manager) GetSystemLogs() (string, error) {
	if m.isDocker {
		// In Docker mode, there's no systemd journal — return container stdout logs
		return "System logs are not available in Docker mode. Use 'docker logs' to view container output.", nil
	}
	cmd := exec.Command("journalctl", "-u", "proxera", "-n", "100", "--no-pager")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get systemd logs: %w", err)
	}
	return string(output), nil
}

// ListBackups returns backups for a domain.
func (m *Manager) ListBackups(domain string) (interface{}, error) {
	return m.nginxMgr.ListBackups(domain)
}

// GetBackupContent returns the content of a specific backup file.
func (m *Manager) GetBackupContent(domain, filename string) (string, error) {
	return m.nginxMgr.GetBackupContent(domain, filename)
}

// RestoreBackup restores a specific backup for a domain.
func (m *Manager) RestoreBackup(domain, filename string) error {
	return m.nginxMgr.RestoreSpecificBackup(domain, filename)
}

// --- CrowdSec operations ---

func (m *Manager) CrowdSecStatus() (interface{}, error)         { return m.crowdsec.Status() }
func (m *Manager) CrowdSecInstall(enrollmentKey string) error   { return m.crowdsec.Install(enrollmentKey) }
func (m *Manager) CrowdSecUninstall() error                     { return m.crowdsec.Uninstall() }
func (m *Manager) CrowdSecListDecisions() (interface{}, error)  { return m.crowdsec.ListDecisions() }
func (m *Manager) CrowdSecAddDecision(ip, duration, reason string) error {
	return m.crowdsec.AddDecision(ip, duration, reason)
}
func (m *Manager) CrowdSecDeleteDecision(id int) error          { return m.crowdsec.DeleteDecision(id) }
func (m *Manager) CrowdSecListAlerts() (interface{}, error)     { return m.crowdsec.ListAlerts() }
func (m *Manager) CrowdSecDeleteAlert(id int) error             { return m.crowdsec.DeleteAlert(id) }
func (m *Manager) CrowdSecListCollections() (interface{}, error) { return m.crowdsec.ListCollections() }
func (m *Manager) CrowdSecInstallCollection(name string) error  { return m.crowdsec.InstallCollection(name) }
func (m *Manager) CrowdSecRemoveCollection(name string) error   { return m.crowdsec.RemoveCollection(name) }
func (m *Manager) CrowdSecListBouncers() (interface{}, error)   { return m.crowdsec.ListBouncers() }
func (m *Manager) CrowdSecInstallBouncer(pkg string) error      { return m.crowdsec.InstallBouncer(pkg) }
func (m *Manager) CrowdSecRemoveBouncer(pkg string) error       { return m.crowdsec.RemoveBouncer(pkg) }
func (m *Manager) CrowdSecGetMetrics() (string, error)          { return m.crowdsec.GetMetrics() }
func (m *Manager) CrowdSecListWhitelists() (interface{}, error) { return m.crowdsec.ListWhitelists() }
func (m *Manager) CrowdSecAddWhitelist(ip, description string) error {
	return m.crowdsec.AddWhitelist(crowdsec.WhitelistEntry{IP: ip, Description: description})
}
func (m *Manager) CrowdSecRemoveWhitelist(ip string) error { return m.crowdsec.RemoveWhitelist(ip) }

// --- First-run detection ---

// SystemStatus reports what's installed on the local system.
type SystemStatus struct {
	NginxInstalled    bool   `json:"nginx_installed"`
	NginxVersion      string `json:"nginx_version,omitempty"`
	NginxRunning      bool   `json:"nginx_running"`
	CrowdSecInstalled bool   `json:"crowdsec_installed"`
	CrowdSecRunning   bool   `json:"crowdsec_running"`
}

// DetectSystem checks what's installed on the local machine.
// In Docker mode, nginx and crowdsec run in separate containers,
// so we check reachability instead of local binaries.
func DetectSystem() SystemStatus {
	s := SystemStatus{}

	isDocker := os.Getenv("PROXERA_DOCKER") == "true"

	if isDocker {
		// In Docker mode, nginx is in a separate container — check via test command
		if cmd := os.Getenv("NGINX_TEST_CMD"); cmd != "" {
			testCmd := exec.Command("sh", "-c", cmd)
			if err := testCmd.Run(); err == nil {
				s.NginxInstalled = true
				s.NginxRunning = true
				// Try to get version from the nginx container
				reloadCmd := os.Getenv("NGINX_RELOAD_CMD")
				if reloadCmd != "" {
					// Derive container name from reload cmd (e.g. "docker kill -s HUP project-nginx-1")
					// Use "docker exec <container> nginx -v" instead
					parts := strings.Fields(reloadCmd)
					if len(parts) >= 1 {
						container := parts[len(parts)-1]
						verCmd := exec.Command("docker", "exec", container, "nginx", "-v")
						if out, err := verCmd.CombinedOutput(); err == nil {
							s.NginxVersion = strings.TrimSpace(string(out))
						}
					}
				}
			}
		} else {
			// No test command configured but Docker mode — assume nginx is available
			s.NginxInstalled = true
			s.NginxRunning = true
		}
	} else {
		// Native mode — check local binaries
		if path, err := exec.LookPath("nginx"); err == nil && path != "" {
			s.NginxInstalled = true
			s.NginxVersion = nginx.DetectNginxVersion()
			cmd := exec.Command("systemctl", "is-active", "--quiet", "nginx")
			s.NginxRunning = cmd.Run() == nil
		}
	}

	// Check CrowdSec
	if isDocker {
		if container := os.Getenv("CROWDSEC_CONTAINER"); container != "" {
			// Check if container exists
			if exec.Command("docker", "inspect", container).Run() == nil {
				s.CrowdSecInstalled = true
				// Check if container is running
				out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", container).Output()
				s.CrowdSecRunning = err == nil && strings.TrimSpace(string(out)) == "true"
			}
		}
	} else {
		if _, err := exec.LookPath("cscli"); err == nil {
			s.CrowdSecInstalled = true
			cmd := exec.Command("systemctl", "is-active", "--quiet", "crowdsec")
			s.CrowdSecRunning = cmd.Run() == nil
		}
	}

	return s
}

// IsLocalAgent returns true if this is the local agent.
func (m *Manager) IsLocalAgent(agentID string) bool {
	return m.agentID != "" && m.agentID == agentID
}

// AgentID returns the local agent's ID.
func (m *Manager) AgentID() string {
	return m.agentID
}

// NginxInstalled returns whether nginx is available on this system.
func NginxInstalled() bool {
	_, err := exec.LookPath("nginx")
	return err == nil
}

// EnsureDirectories creates necessary directories for the local agent.
func EnsureDirectories() error {
	dirs := []string{
		"/etc/nginx/ssl",
		"/var/lib/proxera",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	return nil
}
