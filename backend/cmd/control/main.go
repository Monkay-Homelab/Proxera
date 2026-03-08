// Command proxera-control is the all-in-one Control Node binary.
// It embeds the SvelteKit panel, runs the backend API, and manages the
// local nginx/CrowdSec via direct function calls (no WebSocket to self).
// Remote agents still connect via WebSocket as before.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/joho/godotenv"
	"github.com/proxera/agent/pkg/metrics"
	"github.com/proxera/backend/internal/api"
	"github.com/proxera/backend/internal/api/handlers"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/localagent"
	"github.com/proxera/backend/internal/models"
	"github.com/proxera/backend/internal/notifications"
)

//go:embed panel/*
var panelFS embed.FS

func main() {
	log.Println("Proxera Control Node starting...")

	// Load environment variables
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../../.env"); err != nil {
			log.Println("No .env file found, using environment variables")
		}
	}

	// Validate required secrets
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be set and at least 32 characters")
	}

	// Connect to database
	if err := database.Connect(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Initialize database tables
	if err := database.Initialize(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Migrate existing plaintext API keys to hashed
	handlers.MigrateAPIKeyHashes()

	// Start WebSocket hub for remote agent connections
	handlers.StartHub()

	// Start background jobs
	go handlers.StartCertRenewalJob()
	go handlers.StartAlertWorker()
	go notifications.StartCooldownCleanup()

	// --- First-run detection ---
	isDocker := os.Getenv("PROXERA_DOCKER") == "true"
	status := localagent.DetectSystem()
	log.Printf("[system] nginx: installed=%v version=%s running=%v", status.NginxInstalled, status.NginxVersion, status.NginxRunning)
	log.Printf("[system] crowdsec: installed=%v running=%v", status.CrowdSecInstalled, status.CrowdSecRunning)
	if isDocker {
		log.Println("[system] Running in Docker mode — nginx managed via external container")
	}

	// In Docker mode, nginx is in a separate container — always enable local agent
	enableLocalAgent := status.NginxInstalled || isDocker

	if !enableLocalAgent {
		log.Println("[system] WARNING: nginx is not installed. Local proxy management will be unavailable.")
		log.Println("[system] Install nginx 1.25+ and restart the control node to enable local management.")
	}

	// --- Local agent setup ---
	var localMgr *localagent.Manager
	if enableLocalAgent {
		if err := localagent.EnsureDirectories(); err != nil {
			log.Printf("[local-agent] Warning: failed to create directories: %v", err)
		}

		cfg := localagent.DefaultConfig()

		// Override paths from env
		if p := os.Getenv("NGINX_CONFIG_PATH"); p != "" {
			cfg.NginxConfigPath = p
			cfg.NginxEnabledPath = p
		}
		if p := os.Getenv("NGINX_ENABLED_PATH"); p != "" {
			cfg.NginxEnabledPath = p
		}

		// Docker mode: use docker exec for nginx reload/test
		if isDocker {
			if cmd := os.Getenv("NGINX_RELOAD_CMD"); cmd != "" {
				cfg.NginxReloadCmd = cmd
			}
			if cmd := os.Getenv("NGINX_TEST_CMD"); cmd != "" {
				cfg.NginxTestCmd = cmd
			}
		}

		localMgr = localagent.New(cfg)
		localMgr.SetDDNSUpdate(handlers.UpdateDDNSForAgent)
		localMgr.SetMetricsInsert(func(agentID string, buckets []interface{}) error {
			converted := make([]models.IncomingMetricsBucket, 0, len(buckets))
			for _, raw := range buckets {
				b, ok := raw.(metrics.MetricsBucket)
				if !ok {
					continue
				}
				converted = append(converted, models.IncomingMetricsBucket{
					Timestamp:       b.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
					Domain:          b.Domain,
					RequestCount:    b.RequestCount,
					BytesSent:       b.BytesSent,
					BytesReceived:   b.BytesReceived,
					Status2xx:       b.Status2xx,
					Status3xx:       b.Status3xx,
					Status4xx:       b.Status4xx,
					Status5xx:       b.Status5xx,
					AvgLatencyMs:    b.AvgLatencyMs,
					LatencyP50Ms:    b.LatencyP50Ms,
					LatencyP95Ms:    b.LatencyP95Ms,
					LatencyP99Ms:    b.LatencyP99Ms,
					AvgUpstreamMs:   b.AvgUpstreamMs,
					AvgRequestSize:  b.AvgRequestSize,
					AvgResponseSize: b.AvgResponseSize,
					CacheHits:       b.CacheHits,
					CacheMisses:     b.CacheMisses,
					UniqueIPs:       b.UniqueIPs,
					ConnectionCount: b.ConnectionCount,
					IPRequestCounts: b.IPRequestCounts,
				})
			}
			if err := handlers.InsertMetricsBuckets(agentID, converted); err != nil {
				return err
			}
			if err := handlers.InsertVisitorIPs(agentID, converted); err != nil {
				log.Printf("[local-agent] Failed to insert visitor IPs: %v", err)
			}
			return nil
		})

		// Register local agent (deferred until first admin user exists)
		if _, err := localMgr.RegisterLocalAgent(); err != nil {
			log.Printf("[local-agent] Deferred registration: %v", err)
			log.Println("[local-agent] Local agent will register after first admin account is created.")
		} else {
			localMgr.Start()
		}

		// Make local agent accessible to handlers
		handlers.SetLocalAgent(localMgr)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName: "Proxera Control Node",
	})

	// Setup API routes (same as standalone backend)
	api.SetupRoutes(app)

	// --- System status endpoint ---
	app.Get("/api/system/status", func(c *fiber.Ctx) error {
		return c.JSON(localagent.DetectSystem())
	})

	// --- Embedded panel ---
	panelRoot, err := fs.Sub(panelFS, "panel")
	if err != nil {
		log.Fatal("Failed to create panel sub-filesystem:", err)
	}

	// Serve static assets from the embedded panel
	app.Use("/", filesystem.New(filesystem.Config{
		Root:       http.FS(panelRoot),
		Browse:     false,
		Index:      "index.html",
		MaxAge:     86400,
		NotFoundFile: "index.html", // SPA fallback
		Next: func(c *fiber.Ctx) bool {
			// Skip API, WebSocket, health, and install routes
			path := c.Path()
			return strings.HasPrefix(path, "/api/") ||
				strings.HasPrefix(path, "/ws/") ||
				path == "/health" ||
				path == "/install.sh"
		},
	}))

	// Get port from env
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("API_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	log.Printf("Control Node listening on %s", addr)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received %s, shutting down gracefully...", sig)
		if localMgr != nil {
			localMgr.Stop()
		}
		if err := app.Shutdown(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	if err := app.Listen(addr); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
