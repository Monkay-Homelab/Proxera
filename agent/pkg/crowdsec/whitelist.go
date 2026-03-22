package crowdsec

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const whitelistPath = "/etc/crowdsec/parsers/s02-enrich/proxera-whitelist.yaml"

// WhitelistEntry represents an IP whitelist entry
type WhitelistEntry struct {
	IP          string `json:"ip" yaml:"-"`
	Description string `json:"description" yaml:"-"`
}

// whitelistFile represents the YAML whitelist file structure
type whitelistFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Whitelist   struct {
		Reason string   `yaml:"reason"`
		IPs    []string `yaml:"ip"`
	} `yaml:"whitelist"`
}

// ListWhitelists returns all whitelisted IPs
func (m *Manager) ListWhitelists() ([]WhitelistEntry, error) {
	entries := []WhitelistEntry{}

	data, err := m.readWhitelistFile()
	if err != nil {
		return entries, nil
	}

	var wl whitelistFile
	if err := yaml.Unmarshal(data, &wl); err != nil {
		return nil, fmt.Errorf("failed to parse whitelist file: %w", err)
	}

	for _, ip := range wl.Whitelist.IPs {
		entries = append(entries, WhitelistEntry{
			IP:          ip,
			Description: wl.Whitelist.Reason,
		})
	}

	return entries, nil
}

func (m *Manager) readWhitelistFile() ([]byte, error) {
	if m.IsDocker() {
		return exec.Command("docker", "exec", m.dockerContainer, "cat", whitelistPath).Output()
	}
	return os.ReadFile(whitelistPath)
}

// AddWhitelist adds an IP to the whitelist
func (m *Manager) AddWhitelist(entry WhitelistEntry) error {
	wl := m.loadOrCreateWhitelist()

	// Check for duplicates
	for _, ip := range wl.Whitelist.IPs {
		if ip == entry.IP {
			return fmt.Errorf("IP %s is already whitelisted", entry.IP)
		}
	}

	wl.Whitelist.IPs = append(wl.Whitelist.IPs, entry.IP)

	if err := m.saveWhitelist(wl); err != nil {
		return err
	}

	return m.reloadCrowdSec()
}

// RemoveWhitelist removes an IP from the whitelist
func (m *Manager) RemoveWhitelist(ip string) error {
	wl := m.loadOrCreateWhitelist()

	found := false
	var newIPs []string
	for _, existing := range wl.Whitelist.IPs {
		if existing == ip {
			found = true
			continue
		}
		newIPs = append(newIPs, existing)
	}

	if !found {
		return fmt.Errorf("IP %s not found in whitelist", ip)
	}

	wl.Whitelist.IPs = newIPs

	// If no IPs left, remove the whitelist file entirely
	// (CrowdSec rejects empty IP lists and fails to start)
	if len(newIPs) == 0 {
		if m.IsDocker() {
			exec.Command("docker", "exec", m.dockerContainer, "rm", "-f", whitelistPath).Run()
		} else {
			os.Remove(whitelistPath)
		}
		log.Println("[crowdsec] Whitelist empty, removed whitelist file")
		return m.reloadCrowdSec()
	}

	if err := m.saveWhitelist(wl); err != nil {
		return err
	}

	return m.reloadCrowdSec()
}

func (m *Manager) loadOrCreateWhitelist() *whitelistFile {
	wl := &whitelistFile{
		Name:        "proxera/whitelist",
		Description: "Proxera managed whitelist",
	}
	wl.Whitelist.Reason = "Whitelisted by Proxera panel"

	data, err := m.readWhitelistFile()
	if err != nil {
		return wl
	}

	if err := yaml.Unmarshal(data, wl); err != nil {
		return wl
	}

	return wl
}

func (m *Manager) saveWhitelist(wl *whitelistFile) error {
	data, err := yaml.Marshal(wl)
	if err != nil {
		return fmt.Errorf("failed to marshal whitelist: %w", err)
	}

	if m.IsDocker() {
		// Write via docker exec: pipe YAML into the container
		cmd := exec.Command("docker", "exec", "-i", m.dockerContainer, "sh", "-c",
			fmt.Sprintf("mkdir -p %s && cat > %s", filepath.Dir(whitelistPath), whitelistPath))
		cmd.Stdin = strings.NewReader(string(data))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to write whitelist in container: %s", string(output))
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(whitelistPath), 0755); err != nil {
		return fmt.Errorf("failed to create whitelist directory: %w", err)
	}

	if err := os.WriteFile(whitelistPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write whitelist file: %w", err)
	}

	return nil
}

func (m *Manager) reloadCrowdSec() error {
	log.Println("[crowdsec] Reloading CrowdSec...")
	if m.IsDocker() {
		cmd := exec.Command("docker", "restart", m.dockerContainer)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to restart crowdsec container: %s", strings.TrimSpace(string(output)))
		}
		return nil
	}
	cmd := exec.Command("systemctl", "reload", "crowdsec")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try restart if reload fails
		cmd = exec.Command("systemctl", "restart", "crowdsec")
		if output, err = cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to reload crowdsec: %s", strings.TrimSpace(string(output)))
		}
	}
	return nil
}
