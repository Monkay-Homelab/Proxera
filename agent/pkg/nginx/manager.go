package nginx

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupInfo describes a single nginx config backup file.
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Domain    string    `json:"domain"`
	Timestamp time.Time `json:"timestamp"`
	SizeBytes int64     `json:"size_bytes"`
}

// Manager handles nginx operations
type Manager struct {
	nginxBinary  string
	configPath   string
	enabledPath  string
	backupPath   string
	reloadCmd    string // optional override for reload (e.g. "docker exec nginx nginx -s reload")
	testCmd      string // optional override for config test
}

// NewManager creates a new nginx manager
func NewManager(nginxBinary, configPath, enabledPath string) *Manager {
	return &Manager{
		nginxBinary:  nginxBinary,
		configPath:   configPath,
		enabledPath:  enabledPath,
		backupPath:   filepath.Join(configPath, ".backups"),
	}
}

// SetReloadCmd sets an external command to use for nginx reload (e.g. Docker exec).
func (m *Manager) SetReloadCmd(cmd string) { m.reloadCmd = cmd }

// SetTestCmd sets an external command to use for nginx config test.
func (m *Manager) SetTestCmd(cmd string) { m.testCmd = cmd }

// Test validates the nginx configuration
func (m *Manager) Test() error {
	var cmd *exec.Cmd
	if m.testCmd != "" {
		cmd = exec.Command("sh", "-c", m.testCmd)
	} else {
		cmd = exec.Command(m.nginxBinary, "-t")
	}
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("nginx config test failed: %s\n%s", err, string(output))
	}

	log.Println("Nginx configuration is valid")
	return nil
}

// Reload reloads nginx configuration
func (m *Manager) Reload() error {
	var cmd *exec.Cmd
	if m.reloadCmd != "" {
		cmd = exec.Command("sh", "-c", m.reloadCmd)
	} else {
		cmd = exec.Command(m.nginxBinary, "-s", "reload")
	}
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("nginx reload failed: %s\n%s", err, string(output))
	}

	log.Println("Nginx reloaded successfully")
	return nil
}

// BackupConfig creates a backup of current configuration
func (m *Manager) BackupConfig(domain string) error {
	// Ensure backup directory exists
	if err := os.MkdirAll(m.backupPath, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	safeDomain := SanitizeDomain(domain)
	source := filepath.Join(m.configPath, fmt.Sprintf("proxera_%s.conf", safeDomain))

	// Only backup if file exists
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil // No existing config to backup
	}

	timestamp := time.Now().Format("20060102_150405")
	backup := filepath.Join(m.backupPath, fmt.Sprintf("proxera_%s_%s.conf", safeDomain, timestamp))

	// Copy file
	input, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if err := os.WriteFile(backup, input, 0644); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}

	log.Printf("Backed up config to: %s", backup)
	return nil
}

// RestoreBackup restores the most recent backup
func (m *Manager) RestoreBackup(domain string) error {
	safeDomain := SanitizeDomain(domain)
	pattern := filepath.Join(m.backupPath, fmt.Sprintf("proxera_%s_*.conf", safeDomain))

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("no backup found for %s", domain)
	}

	// Get most recent backup (last in sorted list)
	latestBackup := matches[len(matches)-1]
	target := filepath.Join(m.configPath, fmt.Sprintf("proxera_%s.conf", safeDomain))

	// Read backup
	input, err := os.ReadFile(latestBackup)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Restore
	if err := os.WriteFile(target, input, 0644); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	log.Printf("Restored config from: %s", latestBackup)
	return nil
}

// ListBackups returns all backups for the given domain, sorted newest first (up to 20).
func (m *Manager) ListBackups(domain string) ([]BackupInfo, error) {
	safeDomain := SanitizeDomain(domain)
	pattern := filepath.Join(m.backupPath, fmt.Sprintf("proxera_%s_*.conf", safeDomain))

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	var infos []BackupInfo
	for _, path := range matches {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		filename := filepath.Base(path)

		// Parse timestamp from filename: proxera_<safeDomain>_<20060102_150405>.conf
		prefix := fmt.Sprintf("proxera_%s_", safeDomain)
		suffix := ".conf"
		if len(filename) <= len(prefix)+len(suffix) {
			continue
		}
		tsStr := filename[len(prefix) : len(filename)-len(suffix)]
		ts, err := time.ParseInLocation("20060102_150405", tsStr, time.Local)
		if err != nil {
			ts = fi.ModTime()
		}

		infos = append(infos, BackupInfo{
			Filename:  filename,
			Domain:    domain,
			Timestamp: ts,
			SizeBytes: fi.Size(),
		})
	}

	// Sort newest first
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Timestamp.After(infos[j].Timestamp)
	})

	// Cap at 20 most recent
	if len(infos) > 20 {
		infos = infos[:20]
	}
	return infos, nil
}

// safeBackupPath validates that filename does not escape the backup directory.
func (m *Manager) safeBackupPath(filename string) (string, error) {
	cleaned := filepath.Clean(filename)
	if cleaned != filepath.Base(cleaned) {
		return "", fmt.Errorf("invalid backup filename")
	}
	full := filepath.Join(m.backupPath, cleaned)
	if !strings.HasPrefix(full, filepath.Clean(m.backupPath)+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid backup filename")
	}
	return full, nil
}

// GetBackupContent returns the raw text of a named backup file.
func (m *Manager) GetBackupContent(domain, filename string) (string, error) {
	backupFile, err := m.safeBackupPath(filename)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(backupFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("backup file not found: %s", filename)
		}
		return "", fmt.Errorf("failed to read backup: %w", err)
	}
	return string(data), nil
}

// RestoreSpecificBackup restores a named backup file, validates nginx config, then reloads.
func (m *Manager) RestoreSpecificBackup(domain, filename string) error {
	safeDomain := SanitizeDomain(domain)

	backupFile, err := m.safeBackupPath(filename)
	if err != nil {
		return err
	}
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", filename)
	}

	target := filepath.Join(m.configPath, fmt.Sprintf("proxera_%s.conf", safeDomain))

	// Back up current live config before restoring
	if err := m.BackupConfig(domain); err != nil {
		log.Printf("Warning: failed to back up current config before restore: %v", err)
	}

	// Read the backup
	content, err := os.ReadFile(backupFile)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Write to live location
	if err := os.WriteFile(target, content, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Validate — if test fails, restore the previous live config
	if err := m.Test(); err != nil {
		// Revert to most-recent backup (which is the pre-restore state we just backed up)
		if restoreErr := m.RestoreBackup(domain); restoreErr != nil {
			log.Printf("Warning: failed to revert after invalid backup: %v", restoreErr)
		}
		return fmt.Errorf("backup config is invalid, reverted: %v", err)
	}

	if err := m.Reload(); err != nil {
		return fmt.Errorf("config restored but nginx reload failed: %w", err)
	}

	log.Printf("Restored backup %s for domain %s", filename, domain)
	return nil
}

// ApplyWithRollback applies configuration with automatic rollback on failure
func (m *Manager) ApplyWithRollback(domain string) error {
	// Create backup first
	hasBackup := true
	if err := m.BackupConfig(domain); err != nil {
		hasBackup = false
		log.Printf("Warning: Failed to create backup for %s: %v", domain, err)
	}

	// Test configuration
	if err := m.Test(); err != nil {
		// Config is invalid, restore backup if available
		if !hasBackup {
			return fmt.Errorf("config test failed and no backup available: %v", err)
		}
		log.Printf("Configuration test failed for %s, attempting rollback...", domain)
		if restoreErr := m.RestoreBackup(domain); restoreErr != nil {
			return fmt.Errorf("config test failed and rollback failed: %v (original error: %v)", restoreErr, err)
		}
		return fmt.Errorf("config test failed, rolled back to previous version: %v", err)
	}

	// Reload nginx
	if err := m.Reload(); err != nil {
		// Reload failed, restore backup
		if !hasBackup {
			return fmt.Errorf("nginx reload failed and no backup available: %v", err)
		}
		log.Printf("Nginx reload failed for %s, attempting rollback...", domain)
		if restoreErr := m.RestoreBackup(domain); restoreErr != nil {
			return fmt.Errorf("reload failed and rollback failed: %v (original error: %v)", restoreErr, err)
		}

		// Try to reload with old config
		if reloadErr := m.Reload(); reloadErr != nil {
			return fmt.Errorf("reload failed and rollback reload failed: %v (original error: %v)", reloadErr, err)
		}

		return fmt.Errorf("reload failed, rolled back to previous version: %v", err)
	}

	return nil
}
