package crowdsec

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// allowedBouncerPackages is a whitelist of known CrowdSec bouncer package names
var allowedBouncerPackages = map[string]bool{
	"crowdsec-firewall-bouncer-iptables":  true,
	"crowdsec-firewall-bouncer-nftables":  true,
	"crowdsec-nginx-bouncer":              true,
	"crowdsec-custom-bouncer":             true,
	"crowdsec-blocklist-mirror":           true,
}

// validBouncerPackage checks if a package name is an allowed CrowdSec bouncer
var validBouncerPackageRe = regexp.MustCompile(`^crowdsec-[a-z0-9-]+$`)

// Bouncer represents a registered CrowdSec bouncer
type Bouncer struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	Type      string `json:"type"`
	Version   string `json:"version"`
	LastPull  string `json:"last_pull"`
	Revoked   bool   `json:"revoked"`
	Valid     bool   `json:"valid"`
}

// ListBouncers returns all registered bouncers
func (m *Manager) ListBouncers() ([]Bouncer, error) {
	var bouncers []Bouncer
	if err := runJSON([]string{"bouncers", "list", "-o", "json"}, &bouncers); err != nil {
		return nil, err
	}
	if bouncers == nil {
		bouncers = []Bouncer{}
	}
	// CrowdSec outputs "revoked" (true = invalid), compute "valid" for the panel
	for i := range bouncers {
		bouncers[i].Valid = !bouncers[i].Revoked
	}
	return bouncers, nil
}

// InstallBouncer installs a bouncer package via system package manager
func (m *Manager) InstallBouncer(packageName string) error {
	if !allowedBouncerPackages[packageName] {
		if !validBouncerPackageRe.MatchString(packageName) {
			return fmt.Errorf("invalid bouncer package name: %s", packageName)
		}
		return fmt.Errorf("unknown bouncer package: %s", packageName)
	}

	if !m.IsInstalled() {
		return fmt.Errorf("CrowdSec must be installed first")
	}

	if runtime.GOOS != "linux" {
		return fmt.Errorf("bouncer installation is only supported on Linux")
	}

	// Block nginx bouncer on nginx.org packages — it requires libnginx-mod-http-lua
	// which is only available for distro-packaged nginx
	if packageName == "crowdsec-nginx-bouncer" && isNginxOrgPackage() {
		return fmt.Errorf("crowdsec-nginx-bouncer is incompatible with nginx.org packages (requires libnginx-mod-http-lua). Use crowdsec-firewall-bouncer-iptables instead")
	}

	log.Printf("[crowdsec] Installing bouncer package: %s", packageName)

	var cmd *exec.Cmd
	if _, err := exec.LookPath("apt-get"); err == nil {
		cmd = exec.Command("apt-get", "install", "-y", packageName)
	} else if _, err := exec.LookPath("yum"); err == nil {
		cmd = exec.Command("yum", "install", "-y", packageName)
	} else if _, err := exec.LookPath("dnf"); err == nil {
		cmd = exec.Command("dnf", "install", "-y", packageName)
	} else {
		return fmt.Errorf("unsupported package manager")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install %s: %s", packageName, string(output))
	}

	// Only test and reload nginx for nginx-related bouncers (not firewall bouncer)
	if strings.Contains(packageName, "nginx") {
		log.Println("[crowdsec] Testing and reloading nginx after bouncer install...")
		testCmd := exec.Command("nginx", "-t")
		if output, err := testCmd.CombinedOutput(); err != nil {
			log.Printf("[crowdsec] Warning: nginx test failed after bouncer install: %s", string(output))
		} else if err := exec.Command("nginx", "-s", "reload").Run(); err != nil {
			log.Printf("[crowdsec] Warning: nginx reload failed after bouncer install: %v", err)
		}
	}

	log.Printf("[crowdsec] Bouncer %s installed successfully", packageName)
	return nil
}

// RemoveBouncer removes a bouncer package via system package manager
func (m *Manager) RemoveBouncer(packageName string) error {
	if !validBouncerPackageRe.MatchString(packageName) {
		return fmt.Errorf("invalid bouncer package name: %s", packageName)
	}

	log.Printf("[crowdsec] Removing bouncer package: %s", packageName)

	var cmd *exec.Cmd
	if _, err := exec.LookPath("apt-get"); err == nil {
		cmd = exec.Command("apt-get", "remove", "-y", packageName)
	} else if _, err := exec.LookPath("yum"); err == nil {
		cmd = exec.Command("yum", "remove", "-y", packageName)
	} else if _, err := exec.LookPath("dnf"); err == nil {
		cmd = exec.Command("dnf", "remove", "-y", packageName)
	} else {
		return fmt.Errorf("unsupported package manager")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove %s: %s", packageName, string(output))
	}

	// Only reload nginx for nginx-related bouncers
	if strings.Contains(packageName, "nginx") {
		log.Println("[crowdsec] Reloading nginx after bouncer removal...")
		if err := exec.Command("nginx", "-s", "reload").Run(); err != nil {
			log.Printf("[crowdsec] Warning: nginx reload failed after bouncer removal: %v", err)
		}
	}

	log.Printf("[crowdsec] Bouncer %s removed successfully", packageName)
	return nil
}
