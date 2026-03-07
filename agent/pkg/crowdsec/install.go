package crowdsec

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// isNginxOrgPackage detects if nginx was installed from nginx.org (not distro packages).
// nginx.org packages are incompatible with crowdsec-nginx-bouncer which requires libnginx-mod-http-lua.
func isNginxOrgPackage() bool {
	// Check for nginx.org apt source file (created by our setup or manual install)
	if _, err := os.Stat("/etc/apt/sources.list.d/nginx.list"); err == nil {
		return true
	}

	// Fallback: check nginx -V output for nginx.org signature
	cmd := exec.Command("nginx", "-V")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "nginx.org")
}

// Install installs CrowdSec, the firewall bouncer, and default collections
func (m *Manager) Install(enrollmentKey string) error {
	if m.IsInstalled() {
		return fmt.Errorf("CrowdSec is already installed")
	}

	log.Println("[crowdsec] Starting CrowdSec installation...")

	// Step 1: Install CrowdSec base package (no nginx reload)
	if err := m.installCrowdSec(); err != nil {
		return fmt.Errorf("failed to install crowdsec: %w", err)
	}

	// Step 2: Update hub index before installing collections
	log.Println("[crowdsec] Updating hub index...")
	{
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, "cscli", "hub", "update")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[crowdsec] Warning: hub update failed (continuing): %s", string(output))
		}
		cancel()
	}

	// Step 3: Install collections (before bouncer, since bouncer install may reload nginx)
	collections := []string{
		"crowdsecurity/nginx",
		"crowdsecurity/base-http-scenarios",
		"crowdsecurity/http-cve",
	}
	var collectionFailures []string
	for _, col := range collections {
		log.Printf("[crowdsec] Installing collection: %s", col)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, "cscli", "collections", "install", col)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[crowdsec] Warning: failed to install collection %s: %s", col, string(output))
			collectionFailures = append(collectionFailures, col)
		}
		cancel()
	}
	if len(collectionFailures) == len(collections) {
		return fmt.Errorf("all CrowdSec collections failed to install: %v", collectionFailures)
	} else if len(collectionFailures) > 0 {
		log.Printf("[crowdsec] Warning: %d/%d collections failed: %v", len(collectionFailures), len(collections), collectionFailures)
	}

	// Step 4: Configure nginx log acquisition
	log.Println("[crowdsec] Configuring log acquisition...")
	if err := m.configureAcquisition(); err != nil {
		return fmt.Errorf("failed to configure acquisition: %w", err)
	}

	// Step 5: Enroll in CrowdSec Central API if key provided
	if enrollmentKey != "" {
		log.Println("[crowdsec] Enrolling in CrowdSec Central API...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, "cscli", "console", "enroll", enrollmentKey)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[crowdsec] Warning: enrollment failed: %s", string(output))
		}
		cancel()
	}

	// Step 6: Enable and restart CrowdSec
	// Must use restart (not start) because the apt postinst already started CrowdSec
	// with auto-discovered setup files. Our configureAcquisition() deleted those files,
	// so we need a restart to pick up only our proxera-nginx.yaml config.
	log.Println("[crowdsec] Enabling and restarting CrowdSec service...")
	exec.Command("systemctl", "enable", "crowdsec").Run()
	if output, err := exec.Command("systemctl", "restart", "crowdsec").CombinedOutput(); err != nil {
		journalOutput, _ := exec.Command("journalctl", "-u", "crowdsec", "--no-pager", "-n", "30").CombinedOutput()
		return fmt.Errorf("failed to restart crowdsec: %w\n%s\nservice logs:\n%s",
			err, strings.TrimSpace(string(output)), strings.TrimSpace(string(journalOutput)))
	}

	// Step 7: Install firewall bouncer
	// Uses iptables-level blocking — works with any nginx (including nginx.org packages),
	// doesn't reload nginx, and doesn't drop WebSocket connections.
	log.Println("[crowdsec] Installing firewall bouncer...")
	if err := m.installFirewallBouncer(); err != nil {
		return fmt.Errorf("failed to install firewall bouncer: %w", err)
	}

	// Verify installation
	if err := m.verifyInstallation(); err != nil {
		log.Printf("[crowdsec] Warning: post-install verification issue: %v", err)
	}

	log.Println("[crowdsec] CrowdSec installation complete")
	return nil
}

// configureAcquisition creates the acquisition config so CrowdSec monitors nginx logs
func (m *Manager) configureAcquisition() error {
	const acquDir = "/etc/crowdsec/acquis.d"
	const acquFile = "/etc/crowdsec/acquis.d/proxera-nginx.yaml"

	if err := os.MkdirAll(acquDir, 0755); err != nil {
		return fmt.Errorf("failed to create acquis.d directory: %w", err)
	}

	config := `# Proxera nginx log acquisition
# Auto-generated by Proxera agent
# Only parse combined-format logs (not proxera_metrics format)
filenames:
  - /var/log/nginx/crowdsec_access.log
  - /var/log/nginx/access.log
labels:
  type: nginx
`
	if err := os.WriteFile(acquFile, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write acquisition config: %w", err)
	}

	// Remove default acquisition configs generated by cscli setup (syslog, auth.log noise)
	for _, f := range []string{"setup.linux.yaml", "setup.sshd.yaml", "setup.nginx.yaml", "setup.pgsql.yaml"} {
		os.Remove(acquDir + "/" + f)
	}

	log.Println("[crowdsec] Configured nginx log acquisition")
	return nil
}

// verifyInstallation checks that CrowdSec is properly set up after install
func (m *Manager) verifyInstallation() error {
	// 1. Verify CrowdSec is running
	if !m.IsRunning() {
		return fmt.Errorf("CrowdSec service is not running after installation")
	}
	log.Println("[crowdsec] Verified: CrowdSec service is running")

	// 2. Verify nginx bouncer is registered
	bouncers, err := m.ListBouncers()
	if err != nil {
		log.Printf("[crowdsec] Warning: could not verify bouncers: %v", err)
	} else if len(bouncers) == 0 {
		log.Println("[crowdsec] Warning: no bouncers registered - nginx bouncer may not be active")
	} else {
		log.Printf("[crowdsec] Verified: %d bouncer(s) registered", len(bouncers))
	}

	// 3. Verify nginx collection is installed
	collections, err := m.ListCollections()
	if err != nil {
		log.Printf("[crowdsec] Warning: could not verify collections: %v", err)
	} else {
		hasNginx := false
		for _, c := range collections {
			if c.Name == "crowdsecurity/nginx" {
				hasNginx = true
				break
			}
		}
		if !hasNginx {
			log.Println("[crowdsec] Warning: crowdsecurity/nginx collection not found")
		} else {
			log.Println("[crowdsec] Verified: nginx collection installed")
		}
	}

	return nil
}

// installCrowdSec installs only the crowdsec base package
func (m *Manager) installCrowdSec() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("CrowdSec installation is only supported on Linux")
	}

	if _, err := exec.LookPath("apt-get"); err == nil {
		cmds := [][]string{
			{"bash", "-c", "curl -s https://install.crowdsec.net | bash"},
			{"apt-get", "update"},
			{"apt-get", "install", "-y", "crowdsec"},
		}
		return m.runCommands(cmds)
	} else if _, err := exec.LookPath("yum"); err == nil {
		cmds := [][]string{
			{"bash", "-c", "curl -s https://install.crowdsec.net | bash"},
			{"yum", "install", "-y", "crowdsec"},
		}
		return m.runCommands(cmds)
	} else if _, err := exec.LookPath("dnf"); err == nil {
		cmds := [][]string{
			{"bash", "-c", "curl -s https://install.crowdsec.net | bash"},
			{"dnf", "install", "-y", "crowdsec"},
		}
		return m.runCommands(cmds)
	}

	return fmt.Errorf("unsupported package manager - please install CrowdSec manually")
}

// installFirewallBouncer installs the iptables-based firewall bouncer.
// This works with any nginx version (including nginx.org packages) and
// doesn't trigger an nginx reload.
func (m *Manager) installFirewallBouncer() error {
	pkg := "crowdsec-firewall-bouncer-iptables"
	if _, err := exec.LookPath("apt-get"); err == nil {
		return m.runCommands([][]string{{"apt-get", "install", "-y", pkg}})
	} else if _, err := exec.LookPath("yum"); err == nil {
		return m.runCommands([][]string{{"yum", "install", "-y", pkg}})
	} else if _, err := exec.LookPath("dnf"); err == nil {
		return m.runCommands([][]string{{"dnf", "install", "-y", pkg}})
	}
	return fmt.Errorf("unsupported package manager")
}

// installNginxBouncer installs just the nginx bouncer package.
// NOTE: Not used during auto-install (incompatible with nginx.org packages).
// Kept for manual bouncer installs on distro-nginx servers.
func (m *Manager) installNginxBouncer() error {
	pkg := "crowdsec-nginx-bouncer"
	if _, err := exec.LookPath("apt-get"); err == nil {
		return m.runCommands([][]string{{"apt-get", "install", "-y", pkg}})
	} else if _, err := exec.LookPath("yum"); err == nil {
		return m.runCommands([][]string{{"yum", "install", "-y", pkg}})
	} else if _, err := exec.LookPath("dnf"); err == nil {
		return m.runCommands([][]string{{"dnf", "install", "-y", pkg}})
	}
	return fmt.Errorf("unsupported package manager")
}

func (m *Manager) runCommands(cmds [][]string) error {
	for _, args := range cmds {
		log.Printf("[crowdsec] Running: %v", args)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("command %v timed out after 5m", args)
			}
			return fmt.Errorf("command %v failed: %s\n%s", args, err, string(output))
		}
	}
	return nil
}

// Uninstall removes CrowdSec and the nginx bouncer
func (m *Manager) Uninstall() error {
	if !m.IsInstalled() {
		return fmt.Errorf("CrowdSec is not installed")
	}

	log.Println("[crowdsec] Stopping CrowdSec services...")
	exec.Command("systemctl", "stop", "crowdsec").Run()
	exec.Command("systemctl", "disable", "crowdsec").Run()

	// Clean up Proxera-managed files
	os.Remove("/etc/crowdsec/acquis.d/proxera-nginx.yaml")
	os.Remove("/etc/crowdsec/parsers/s02-enrich/proxera-whitelist.yaml")
	log.Println("[crowdsec] Cleaned up Proxera config files")

	log.Println("[crowdsec] Removing CrowdSec packages...")
	if _, err := exec.LookPath("apt-get"); err == nil {
		// Remove whichever bouncer is installed (ignore errors for the one that isn't)
		exec.Command("apt-get", "purge", "-y", "crowdsec-nginx-bouncer").Run()
		exec.Command("apt-get", "purge", "-y", "crowdsec-firewall-bouncer-iptables").Run()
		cmd := exec.Command("apt-get", "purge", "-y", "crowdsec")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to remove crowdsec: %s", string(output))
		}
	} else if _, err := exec.LookPath("yum"); err == nil {
		exec.Command("yum", "remove", "-y", "crowdsec-nginx-bouncer").Run()
		exec.Command("yum", "remove", "-y", "crowdsec-firewall-bouncer-iptables").Run()
		cmd := exec.Command("yum", "remove", "-y", "crowdsec")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to remove crowdsec: %s", string(output))
		}
	} else if _, err := exec.LookPath("dnf"); err == nil {
		exec.Command("dnf", "remove", "-y", "crowdsec-nginx-bouncer").Run()
		exec.Command("dnf", "remove", "-y", "crowdsec-firewall-bouncer-iptables").Run()
		cmd := exec.Command("dnf", "remove", "-y", "crowdsec")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to remove crowdsec: %s", string(output))
		}
	}

	// Reload nginx to remove bouncer config
	log.Println("[crowdsec] Reloading nginx...")
	exec.Command("nginx", "-s", "reload").Run()

	log.Println("[crowdsec] CrowdSec uninstallation complete")
	return nil
}
