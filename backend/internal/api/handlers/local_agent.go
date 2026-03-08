package handlers

import (
	"github.com/proxera/agent/pkg/types"
)

// LocalAgent is the interface for the embedded local agent manager.
// It allows handlers to dispatch commands directly instead of via WebSocket.
type LocalAgent interface {
	IsLocalAgent(agentID string) bool
	AgentID() string
	RegisterLocalAgent() (string, error)
	Start()
	ApplyHost(host types.Host) error
	RemoveHost(domain string) error
	ApplyAll(hosts []types.Host) (int, error)
	Reload() error
	GetNginxLogs(lines int) (string, error)
	GetSystemLogs() (string, error)
	ListBackups(domain string) (interface{}, error)
	GetBackupContent(domain, filename string) (string, error)
	RestoreBackup(domain, filename string) error
	CrowdSecStatus() (interface{}, error)
	CrowdSecInstall(enrollmentKey string) error
	CrowdSecUninstall() error
	CrowdSecListDecisions() (interface{}, error)
	CrowdSecAddDecision(ip, duration, reason string) error
	CrowdSecDeleteDecision(id int) error
	CrowdSecListAlerts() (interface{}, error)
	CrowdSecDeleteAlert(id int) error
	CrowdSecListCollections() (interface{}, error)
	CrowdSecInstallCollection(name string) error
	CrowdSecRemoveCollection(name string) error
	CrowdSecListBouncers() (interface{}, error)
	CrowdSecInstallBouncer(pkg string) error
	CrowdSecRemoveBouncer(pkg string) error
	CrowdSecGetMetrics() (string, error)
	CrowdSecListWhitelists() (interface{}, error)
	CrowdSecAddWhitelist(ip, description string) error
	CrowdSecRemoveWhitelist(ip string) error
	CrowdSecGetBanDuration() (string, error)
	CrowdSecSetBanDuration(duration string) error
}

var localAgent LocalAgent

// SetLocalAgent sets the local agent manager for direct command dispatch.
func SetLocalAgent(la LocalAgent) {
	localAgent = la
}

// GetLocalAgent returns the local agent manager, or nil if not running in AIO mode.
func GetLocalAgent() LocalAgent {
	return localAgent
}

// IsLocalAgent checks if the given agent ID belongs to the local embedded agent.
func IsLocalAgent(agentID string) bool {
	return localAgent != nil && localAgent.IsLocalAgent(agentID)
}

// TryRegisterLocalAgent attempts to register the local agent if it hasn't been registered yet.
// Called after the first admin user is created.
func TryRegisterLocalAgent() {
	if localAgent == nil || localAgent.AgentID() != "" {
		return
	}
	if _, err := localAgent.RegisterLocalAgent(); err != nil {
		return
	}
	localAgent.Start()
}
