package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/proxera/agent/internal/api"
	"github.com/proxera/agent/internal/client"
	"github.com/proxera/agent/internal/config"
	"github.com/proxera/agent/pkg/crowdsec"
	"github.com/proxera/agent/pkg/deploy"
	"github.com/proxera/agent/pkg/metrics"
	"github.com/proxera/agent/pkg/nginx"
	"github.com/proxera/agent/pkg/types"
	"github.com/proxera/agent/pkg/version"
)

func main() {
	// Command flags
	configFile := flag.String("config", "proxera.yaml", "Path to configuration file")
	genConfig := flag.Bool("generate", false, "Generate nginx configurations")
	testConfig := flag.Bool("test", false, "Test nginx configuration")
	reloadNginx := flag.Bool("reload", false, "Reload nginx")
	applyAll := flag.Bool("apply", false, "Apply all configurations (generate + enable + reload)")
	serve := flag.Bool("serve", false, "Start agent API server")
	initConfig := flag.Bool("init", false, "Create example configuration file")
	setupAgent := flag.Bool("setup", false, "Setup nginx automatically (install, configure, start)")
	registerAgent := flag.Bool("register", false, "Register agent with panel (requires panel URL and auth token)")
	connectAgent := flag.Bool("connect", false, "Connect to panel via WebSocket")
	showVersion := flag.Bool("version", false, "Show version information")
	checkUpdate := flag.Bool("check-update", false, "Check for available updates")
	selfUpdate := flag.Bool("update", false, "Update to the latest version")

	// Registration flags
	panelURL := flag.String("panel-url", "", "Panel URL for registration (e.g., https://your-api-host:8080)")
	authToken := flag.String("token", "", "User authentication token from panel")
	agentName := flag.String("name", "", "Friendly name for this agent")

	flag.Parse()

	// Show version
	if *showVersion {
		info := version.GetInfo()
		fmt.Printf("Proxera Agent v%s\n", info.Version)
		fmt.Printf("Go: %s\n", info.GoVersion)
		fmt.Printf("OS/Arch: %s/%s\n", info.OS, info.Arch)
		return
	}

	// Check for updates
	if *checkUpdate {
		fmt.Println("[INFO] Checking for updates...")
		updateInfo, err := version.CheckForUpdate("")
		if err != nil {
			log.Fatalf("Failed to check for updates: %v", err)
		}

		if updateInfo.LatestVersion == version.Version {
			fmt.Printf("[OK] You are running the latest version (%s)\n", version.Version)
		} else {
			fmt.Printf("[INFO] New version available: %s (current: %s)\n", updateInfo.LatestVersion, version.Version)
			if updateInfo.ReleaseNotes != "" {
				fmt.Printf("\nRelease notes:\n%s\n", updateInfo.ReleaseNotes)
			}
			fmt.Println("\nRun 'proxera -update' to install")
		}
		return
	}

	// Self-update
	if *selfUpdate {
		if err := version.SelfUpdate(); err != nil {
			log.Fatalf("Update failed: %v", err)
		}
		return
	}

	// Setup nginx automatically
	if *setupAgent {
		setup := nginx.NewSetup("/usr/sbin/nginx", "/etc/nginx/conf.d", "/etc/nginx/conf.d")
		if err := setup.RunSetup(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		return
	}

	// Register with panel
	if *registerAgent {
		if *panelURL == "" || *authToken == "" || *agentName == "" {
			log.Fatal("Registration requires: -panel-url, -token, and -name")
		}

		fmt.Println("[INFO] Registering agent with panel...")
		resp, err := client.Register(*panelURL, *authToken, *agentName)
		if err != nil {
			log.Fatalf("Registration failed: %v", err)
		}

		fmt.Printf("[OK] Agent registered successfully!\n\n")
		fmt.Printf("Agent ID: %s\n", resp.AgentID)
		fmt.Printf("API Key:  %s\n", resp.APIKey)
		fmt.Printf("\nSave these credentials to your config file:\n")
		fmt.Printf("  agent_id: %s\n", resp.AgentID)
		fmt.Printf("  api_key: %s\n", resp.APIKey)
		fmt.Printf("  panel_url: %s\n", *panelURL)
		fmt.Printf("\nTo connect to panel:\n")
		fmt.Printf("  proxera -connect\n")
		return
	}

	// Connect to panel via WebSocket
	if *connectAgent {
		// Load config to get credentials
		cfg, err := config.Load(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		if cfg.APIKey == "" || cfg.AgentID == "" {
			log.Fatal("Agent not registered. Run: proxera -register -panel-url <url> -token <token> -name <name>")
		}

		// Determine WebSocket URL - convert http(s):// to ws(s)://
		panelURL := cfg.PanelURL
		if panelURL == "" {
			log.Fatal("panel_url not set in config. Run: proxera -register -panel-url <url> -token <token> -name <name>")
		}

		// Replace http:// with ws:// and https:// with wss://
		wsURL := ""
		if len(panelURL) >= 8 && panelURL[:8] == "https://" {
			wsURL = "wss://" + panelURL[8:] + "/ws/agent"
		} else if len(panelURL) >= 7 && panelURL[:7] == "http://" {
			wsURL = "ws://" + panelURL[7:] + "/ws/agent"
		} else {
			wsURL = "wss://" + panelURL + "/ws/agent"
		}

		fmt.Printf("[INFO] Connecting to panel at %s...\n", wsURL)

		ws, err := client.NewWSClient(wsURL, cfg.APIKey, cfg.AgentID)
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}

		// Initialize nginx manager and deployer for remote config deployment
		nginxBinary := cfg.NginxBinary
		if nginxBinary == "" {
			nginxBinary = "/usr/sbin/nginx"
		}
		nginxConfigPath := cfg.NginxConfigPath
		if nginxConfigPath == "" {
			nginxConfigPath = "/etc/nginx/conf.d"
		}
		nginxEnabledPath := cfg.NginxEnabledPath
		if nginxEnabledPath == "" {
			nginxEnabledPath = "/etc/nginx/conf.d"
		}
		mgr := nginx.NewManager(nginxBinary, nginxConfigPath, nginxEnabledPath)
		dep := deploy.NewDeployer(mgr, nginxConfigPath, nginxEnabledPath)
		ws.SetDeployer(dep, mgr)

		// Initialize metrics collector for nginx access logs
		collector := metrics.NewCollector("/var/log/nginx")
		ws.SetCollector(collector)

		// Initialize CrowdSec manager
		csManager := crowdsec.NewManager()
		ws.SetCrowdSec(csManager)

		// Set metrics interval from config (default 300s / 5min)
		if cfg.MetricsInterval > 0 {
			ws.SetMetricsInterval(cfg.MetricsInterval)
		}

		ws.Start()
		fmt.Println("[OK] Connected to panel! Press Ctrl+C to disconnect.")

		// Wait for interrupt signal or connection close
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		select {
		case <-sigChan:
			fmt.Println("\n[INFO] Disconnecting...")
			ws.Close()
		case <-ws.DisconnectChan():
			fmt.Println("\n[WARN] Connection lost, exiting...")
			// Exit so systemd can restart us
		}
		return
	}

	// Initialize example config
	if *initConfig {
		createExampleConfig(*configFile)
		return
	}

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create nginx manager
	manager := nginx.NewManager(cfg.NginxBinary, cfg.NginxConfigPath, cfg.NginxEnabledPath)

	// Handle commands
	switch {
	case *serve:
		// Start API server
		server := api.NewServer(cfg)
		if err := server.Start(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}

	case *genConfig:
		// Generate configurations
		for _, host := range cfg.Hosts {
			if err := nginx.GenerateConfig(host, cfg.NginxConfigPath); err != nil {
				log.Printf("[ERROR] Failed to generate config for %s: %v", host.Domain, err)
				continue
			}
		}
		fmt.Println("\n[OK] All configurations generated")

	case *testConfig:
		// Test nginx configuration
		if err := manager.Test(); err != nil {
			log.Fatalf("[ERROR] %v", err)
		}

	case *reloadNginx:
		// Reload nginx
		if err := manager.Reload(); err != nil {
			log.Fatalf("[ERROR] %v", err)
		}

	case *applyAll:
		// Apply all: generate + enable + reload with rollback
		fmt.Println("[INFO] Applying all configurations...\n")

		for _, host := range cfg.Hosts {
			fmt.Printf("[INFO] Processing %s...\n", host.Domain)

			// Generate config
			if err := nginx.GenerateConfig(host, cfg.NginxConfigPath); err != nil {
				log.Printf("[ERROR] Failed to generate config for %s: %v\n", host.Domain, err)
				continue
			}

			// Enable config
			if err := nginx.EnableConfig(host.Domain, cfg.NginxConfigPath, cfg.NginxEnabledPath); err != nil {
				log.Printf("[ERROR] Failed to enable config for %s: %v\n", host.Domain, err)
				continue
			}

			// Apply with rollback
			if err := manager.ApplyWithRollback(host.Domain); err != nil {
				log.Printf("[ERROR] Failed to apply config for %s: %v\n", host.Domain, err)
				continue
			}

			fmt.Printf("[OK] Successfully applied %s\n\n", host.Domain)
		}

		fmt.Println("[OK] Done! All configurations applied")

	default:
		flag.Usage()
		fmt.Println("\nExamples:")
		fmt.Println("  proxera -version                                  # Show version information")
		fmt.Println("  proxera -check-update                             # Check for available updates")
		fmt.Println("  proxera -update                                   # Update to latest version")
		fmt.Println("  proxera -setup                                    # Setup nginx automatically")
		fmt.Println("  proxera -init                                     # Create example config")
		fmt.Println("  proxera -register -panel-url <url> -token <jwt> -name \"My Server\"  # Register with panel")
		fmt.Println("  proxera -connect                                  # Connect to panel")
		fmt.Println("  proxera -generate                                 # Generate nginx configs")
		fmt.Println("  proxera -test                                     # Test nginx config")
		fmt.Println("  proxera -reload                                   # Reload nginx")
		fmt.Println("  proxera -apply                                    # Apply all configs")
		fmt.Println("  proxera -serve                                    # Start agent API server")
	}
}

func createExampleConfig(path string) {
	example := &types.AgentConfig{
		AgentPort:        52080,
		NginxBinary:      "/usr/sbin/nginx",
		NginxConfigPath:  "/etc/nginx/conf.d",
		NginxEnabledPath: "/etc/nginx/conf.d",
		Hosts: []types.Host{
			{
				Domain:      "example.com",
				UpstreamURL: "http://localhost:3000",
				SSL:         false,
				WebSocket:   false,
			},
			{
				Domain:      "api.example.com",
				UpstreamURL: "http://localhost:8080",
				SSL:         true,
				CertPath:    "/etc/ssl/certs/api.example.com.crt",
				KeyPath:     "/etc/ssl/private/api.example.com.key",
				Headers: map[string]string{
					"X-Custom-Header": "value",
				},
			},
		},
	}

	if err := config.Save(example, path); err != nil {
		log.Fatalf("Failed to create example config: %v", err)
	}

	fmt.Printf("[OK] Created example configuration: %s\n", path)
	fmt.Println("\nEdit the file and then run:")
	fmt.Println("  proxera -apply")
}
