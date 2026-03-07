package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Setup ensures nginx is installed and configured
type Setup struct {
	nginxBinary  string
	configPath   string
	enabledPath  string
}

// NewSetup creates a new setup helper
func NewSetup(nginxBinary, configPath, enabledPath string) *Setup {
	return &Setup{
		nginxBinary:  nginxBinary,
		configPath:   configPath,
		enabledPath:  enabledPath,
	}
}

// CheckNginx verifies if nginx is installed
func (s *Setup) CheckNginx() (bool, error) {
	_, err := exec.LookPath(s.nginxBinary)
	if err != nil {
		// Try common paths
		commonPaths := []string{
			"/usr/sbin/nginx",
			"/usr/bin/nginx",
			"/usr/local/bin/nginx",
		}

		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				s.nginxBinary = path
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
}

// getDistroCodename returns the distribution codename (e.g., "jammy", "noble")
func getDistroCodename() (string, error) {
	// Try lsb_release first
	if out, err := exec.Command("lsb_release", "-cs").Output(); err == nil {
		codename := strings.TrimSpace(string(out))
		if codename != "" {
			return codename, nil
		}
	}

	// Fall back to /etc/os-release
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("cannot determine distro codename: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VERSION_CODENAME=") {
			codename := strings.TrimPrefix(line, "VERSION_CODENAME=")
			codename = strings.Trim(codename, "\"")
			if codename != "" {
				return codename, nil
			}
		}
	}

	return "", fmt.Errorf("cannot determine distro codename from /etc/os-release")
}

// setupNginxOfficialRepo configures the official nginx.org stable repository for Debian/Ubuntu
func setupNginxOfficialRepo() error {
	fmt.Println("[INFO] Setting up official nginx.org stable repository...")

	// 1. Install prerequisites
	cmd := exec.Command("apt-get", "install", "-y", "curl", "gnupg2", "ca-certificates", "lsb-release")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to install prerequisites: %s\n%s", err, string(out))
	}

	// 2. Import nginx signing key
	fmt.Println("[INFO] Importing nginx signing key...")
	curlCmd := exec.Command("bash", "-c",
		"curl -fsSL https://nginx.org/keys/nginx_signing.key | gpg --dearmor -o /usr/share/keyrings/nginx-archive-keyring.gpg")
	if out, err := curlCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to import nginx signing key: %s\n%s", err, string(out))
	}

	// 3. Determine distro codename
	codename, err := getDistroCodename()
	if err != nil {
		return err
	}
	fmt.Printf("[INFO] Detected distro codename: %s\n", codename)

	// 4. Add official stable repo
	repoLine := fmt.Sprintf("deb [signed-by=/usr/share/keyrings/nginx-archive-keyring.gpg] https://nginx.org/packages/ubuntu %s nginx\n", codename)
	if err := os.WriteFile("/etc/apt/sources.list.d/nginx.list", []byte(repoLine), 0644); err != nil {
		return fmt.Errorf("failed to write nginx repo file: %w", err)
	}

	// 5. Pin nginx.org repo to prioritize over distro packages
	pinContent := `Package: *
Pin: origin nginx.org
Pin: release o=nginx
Pin-Priority: 900
`
	if err := os.WriteFile("/etc/apt/preferences.d/99nginx", []byte(pinContent), 0644); err != nil {
		return fmt.Errorf("failed to write nginx pin file: %w", err)
	}

	fmt.Println("[OK] Official nginx.org repository configured")
	return nil
}

// InstallNginx installs nginx from the official nginx.org stable repository
func (s *Setup) InstallNginx() error {
	fmt.Println("[INFO] Installing nginx...")

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("apt-get"); err == nil {
			return s.installNginxApt()
		} else if _, err := exec.LookPath("yum"); err == nil {
			return s.installNginxYum()
		} else if _, err := exec.LookPath("dnf"); err == nil {
			return s.installNginxDnf()
		}
		return fmt.Errorf("unsupported package manager - please install nginx manually")
	default:
		return fmt.Errorf("automatic nginx installation not supported on %s - please install manually", runtime.GOOS)
	}
}

// installNginxApt installs nginx from official repo on Debian/Ubuntu
func (s *Setup) installNginxApt() error {
	// Setup official nginx.org repository
	if err := setupNginxOfficialRepo(); err != nil {
		return fmt.Errorf("failed to setup nginx repo: %w", err)
	}

	// Update package list with new repo
	cmd := exec.Command("apt-get", "update")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update package list: %s\n%s", err, string(out))
	}

	// Install nginx non-interactively with --force-confnew to accept nginx.org's nginx.conf
	fmt.Println("[INFO] Installing nginx from official repository...")
	cmd = exec.Command("apt-get", "install", "-y",
		"-o", "Dpkg::Options::=--force-confnew",
		"nginx")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nginx installation failed: %s\n%s", err, string(out))
	}

	// Remove default.conf shipped by nginx.org (avoids port 80 conflict)
	os.Remove("/etc/nginx/conf.d/default.conf")

	fmt.Println("[OK] Nginx installed from official nginx.org repository")
	return nil
}

// installNginxYum installs nginx from official repo on RHEL/CentOS
func (s *Setup) installNginxYum() error {
	// Create nginx.repo
	repoContent := `[nginx-stable]
name=nginx stable repo
baseurl=https://nginx.org/packages/centos/$releasever/$basearch/
gpgcheck=1
enabled=1
gpgkey=https://nginx.org/keys/nginx_signing.key
module_hotfixes=true
`
	if err := os.WriteFile("/etc/yum.repos.d/nginx.repo", []byte(repoContent), 0644); err != nil {
		return fmt.Errorf("failed to write nginx repo: %w", err)
	}

	cmd := exec.Command("yum", "install", "-y", "nginx")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx installation failed: %s\n%s", err, string(output))
	}

	os.Remove("/etc/nginx/conf.d/default.conf")
	fmt.Println("[OK] Nginx installed from official nginx.org repository")
	return nil
}

// installNginxDnf installs nginx from official repo on Fedora
func (s *Setup) installNginxDnf() error {
	repoContent := `[nginx-stable]
name=nginx stable repo
baseurl=https://nginx.org/packages/centos/$releasever/$basearch/
gpgcheck=1
enabled=1
gpgkey=https://nginx.org/keys/nginx_signing.key
module_hotfixes=true
`
	if err := os.WriteFile("/etc/yum.repos.d/nginx.repo", []byte(repoContent), 0644); err != nil {
		return fmt.Errorf("failed to write nginx repo: %w", err)
	}

	cmd := exec.Command("dnf", "install", "-y", "nginx")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx installation failed: %s\n%s", err, string(output))
	}

	os.Remove("/etc/nginx/conf.d/default.conf")
	fmt.Println("[OK] Nginx installed from official nginx.org repository")
	return nil
}

// UpgradeNginx upgrades an existing nginx installation to the official nginx.org package.
// Must be called AFTER configs have been migrated to conf.d/ (see Phase 5 migration).
func (s *Setup) UpgradeNginx() error {
	if _, err := exec.LookPath("apt-get"); err == nil {
		// Setup repo if not already configured
		if _, err := os.Stat("/etc/apt/sources.list.d/nginx.list"); os.IsNotExist(err) {
			if err := setupNginxOfficialRepo(); err != nil {
				return fmt.Errorf("failed to setup nginx repo: %w", err)
			}
		}

		cmd := exec.Command("apt-get", "update")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to update package list: %s\n%s", err, string(out))
		}

		// Remove Ubuntu distro nginx packages first — they conflict with nginx.org's
		// single "nginx" package. The distro uses nginx + nginx-common + nginx-core,
		// while nginx.org ships everything as a single "nginx" package.
		// Configs are already migrated to conf.d before this function is called.
		fmt.Println("[INFO] Removing Ubuntu distro nginx packages...")
		exec.Command("systemctl", "stop", "nginx").Run()

		// Fix any interrupted dpkg state from previous failed attempts
		fixCmd := exec.Command("dpkg", "--configure", "-a")
		fixCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		fixCmd.Run() // best-effort

		// Check if distro packages are installed and remove them
		for _, pkg := range []string{"nginx", "nginx-common", "nginx-core"} {
			checkCmd := exec.Command("dpkg", "-s", pkg)
			if checkCmd.Run() == nil {
				rmCmd := exec.Command("dpkg", "--force-remove-reinstreq", "--force-depends", "--remove", pkg)
				if out, err := rmCmd.CombinedOutput(); err != nil {
					fmt.Printf("Warning: failed to remove %s: %s\n", pkg, strings.TrimSpace(string(out)))
				}
			}
		}

		// Install nginx from nginx.org repo
		fmt.Println("[INFO] Installing nginx from official nginx.org repository...")
		cmd = exec.Command("apt-get", "install", "-y",
			"-o", "Dpkg::Options::=--force-confnew",
			"nginx")
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("nginx upgrade failed: %s\n%s", err, string(out))
		}

		// Start nginx with new binary
		exec.Command("systemctl", "start", "nginx").Run()

		os.Remove("/etc/nginx/conf.d/default.conf")
		return nil
	}

	return fmt.Errorf("upgrade not supported for this package manager")
}

// EnsureDirectories creates required directories if they don't exist
func (s *Setup) EnsureDirectories() error {
	dirs := []string{
		s.configPath,
		s.configPath + "/.backups",
		"/var/log/nginx",
	}

	// Only create enabledPath if different from configPath (symlink pattern)
	if s.enabledPath != s.configPath {
		dirs = append(dirs, s.enabledPath)
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	fmt.Println("[OK] All required directories exist")
	return nil
}

// EnsureNginxRunning ensures nginx service is running
func (s *Setup) EnsureNginxRunning() error {
	// Check if nginx is running
	cmd := exec.Command(s.nginxBinary, "-t")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nginx config test failed: %w", err)
	}

	// Try to start nginx with systemctl
	cmd = exec.Command("systemctl", "is-active", "nginx")
	if err := cmd.Run(); err != nil {
		// Nginx not running, try to start
		fmt.Println("[INFO] Starting nginx service...")
		cmd = exec.Command("systemctl", "start", "nginx")
		if err := cmd.Run(); err != nil {
			// Try direct nginx start
			cmd = exec.Command(s.nginxBinary)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start nginx: %w", err)
			}
		}
	}

	// Enable nginx to start on boot
	cmd = exec.Command("systemctl", "enable", "nginx")
	cmd.Run() // Ignore error - not critical

	fmt.Println("[OK] Nginx is running")
	return nil
}

// RunSetup performs complete nginx setup
func (s *Setup) RunSetup() error {
	fmt.Println("[INFO] Setting up Proxera agent...\n")

	// Check if nginx is installed
	installed, err := s.CheckNginx()
	if err != nil {
		return err
	}

	if !installed {
		fmt.Println("[WARN] Nginx not found")
		fmt.Println("[INFO] Attempting to install nginx...")

		if err := s.InstallNginx(); err != nil {
			return fmt.Errorf("failed to install nginx: %w\n\nPlease install nginx manually:\n  Ubuntu/Debian: sudo apt-get install nginx\n  RHEL/CentOS: sudo yum install nginx", err)
		}
	} else {
		fmt.Println("[OK] Nginx is installed")
	}

	// Ensure directories exist
	if err := s.EnsureDirectories(); err != nil {
		return err
	}

	// Ensure nginx is running
	if err := s.EnsureNginxRunning(); err != nil {
		return err
	}

	fmt.Println("\n[OK] Setup complete! Proxera agent is ready to use.")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Create config: proxera -init")
	fmt.Println("  2. Edit proxera.yaml with your hosts")
	fmt.Println("  3. Apply configs: sudo proxera -apply")

	return nil
}

// verifyNginxConfIncludes ensures nginx.conf contains an include for /etc/nginx/conf.d/*.conf.
// If the include is missing (e.g. after a corrupted upgrade), it adds it inside the http block.
func (s *Setup) verifyNginxConfIncludes() {
	data, err := os.ReadFile("/etc/nginx/nginx.conf")
	if err != nil {
		return
	}
	if strings.Contains(string(data), "include /etc/nginx/conf.d/") {
		return
	}
	// Add the include directive inside the http block
	content := string(data)
	httpIdx := strings.Index(content, "http {")
	if httpIdx < 0 {
		return
	}
	insertIdx := httpIdx + len("http {")
	newContent := content[:insertIdx] + "\n    include /etc/nginx/conf.d/*.conf;" + content[insertIdx:]
	os.WriteFile("/etc/nginx/nginx.conf", []byte(newContent), 0644)
}

// PerformNginxUpgrade performs a full nginx upgrade from distro packages to nginx.org stable:
//  1. Validates current nginx config
//  2. Migrates configs from sites-available/sites-enabled to conf.d (if needed)
//  3. Verifies and reloads current nginx with migrated configs
//  4. Upgrades the nginx package from official repo
//  5. Cleans up old directories, default configs, and migrates backups
//  6. Final test and reload
//
// The caller is responsible for CrowdSec bouncer pre-check (step 1 of the migration plan)
// and config regeneration + proxera.yaml update (steps 11-12).
// Returns number of configs migrated and any error.
func (s *Setup) PerformNginxUpgrade() (int, error) {
	fmt.Println("[INFO] Starting nginx upgrade...")

	// Pre-check: ensure current config is valid
	cmd := exec.Command(s.nginxBinary, "-t")
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("nginx -t failed before upgrade (fix existing issues first): %s", strings.TrimSpace(string(out)))
	}

	// Detect if migration from sites-available → conf.d is needed
	sitesAvailable := "/etc/nginx/sites-available"
	sitesEnabled := "/etc/nginx/sites-enabled"
	confD := "/etc/nginx/conf.d"

	migrated := 0

	matches, _ := filepath.Glob(filepath.Join(sitesAvailable, "proxera_*.conf"))
	needsMigration := len(matches) > 0

	if needsMigration {
		fmt.Printf("[INFO] Migrating %d config(s) from sites-available to conf.d...\n", len(matches))

		// Copy proxera_*.conf from sites-available to conf.d
		os.MkdirAll(confD, 0755)
		for _, src := range matches {
			data, err := os.ReadFile(src)
			if err != nil {
				return 0, fmt.Errorf("failed to read %s: %w", filepath.Base(src), err)
			}
			dst := filepath.Join(confD, filepath.Base(src))
			if err := os.WriteFile(dst, data, 0644); err != nil {
				return 0, fmt.Errorf("failed to write %s: %w", filepath.Base(dst), err)
			}
			migrated++
		}

		// Remove proxera_*.conf symlinks from sites-enabled
		enabledMatches, _ := filepath.Glob(filepath.Join(sitesEnabled, "proxera_*.conf"))
		for _, link := range enabledMatches {
			os.Remove(link)
		}
		// Remove Ubuntu's default site symlink
		os.Remove(filepath.Join(sitesEnabled, "default"))

		// Verify and reload with current nginx before package upgrade
		cmd = exec.Command(s.nginxBinary, "-t")
		if out, err := cmd.CombinedOutput(); err != nil {
			return migrated, fmt.Errorf("nginx -t failed after config migration: %s", strings.TrimSpace(string(out)))
		}
		cmd = exec.Command(s.nginxBinary, "-s", "reload")
		if out, err := cmd.CombinedOutput(); err != nil {
			return migrated, fmt.Errorf("nginx reload failed after config migration: %s", strings.TrimSpace(string(out)))
		}
		fmt.Println("[OK] Configs migrated to conf.d and verified")
	}

	// Upgrade nginx package
	fmt.Println("[INFO] Upgrading nginx package from official repository...")
	if err := s.UpgradeNginx(); err != nil {
		return migrated, fmt.Errorf("nginx package upgrade failed: %w", err)
	}
	fmt.Println("[OK] Nginx package upgraded")

	// Remove default.conf shipped by nginx.org (avoids port 80 conflicts)
	os.Remove("/etc/nginx/conf.d/default.conf")

	// Verify nginx.conf includes conf.d
	s.verifyNginxConfIncludes()

	if needsMigration {
		// Remove source files from sites-available (already copied to conf.d)
		for _, src := range matches {
			os.Remove(src)
		}
		os.Remove(filepath.Join(sitesAvailable, "default"))

		// Migrate .backups directory
		oldBackups := filepath.Join(sitesAvailable, ".backups")
		newBackups := filepath.Join(confD, ".backups")
		if info, err := os.Stat(oldBackups); err == nil && info.IsDir() {
			os.MkdirAll(newBackups, 0755)
			entries, _ := os.ReadDir(oldBackups)
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				srcPath := filepath.Join(oldBackups, entry.Name())
				dstPath := filepath.Join(newBackups, entry.Name())
				if data, readErr := os.ReadFile(srcPath); readErr == nil {
					os.WriteFile(dstPath, data, 0644)
				}
			}
			os.RemoveAll(oldBackups)
		}
	}

	// Final test and reload
	cmd = exec.Command(s.nginxBinary, "-t")
	if out, err := cmd.CombinedOutput(); err != nil {
		return migrated, fmt.Errorf("nginx -t failed after upgrade: %s", strings.TrimSpace(string(out)))
	}

	cmd = exec.Command(s.nginxBinary, "-s", "reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		return migrated, fmt.Errorf("nginx reload failed after upgrade: %s", strings.TrimSpace(string(out)))
	}

	fmt.Println("[OK] Nginx upgrade complete!")
	return migrated, nil
}
