// Command proxera-control is the all-in-one Control Node binary.
// It embeds the SvelteKit panel, runs the backend API, and manages the
// local nginx/CrowdSec via direct function calls (no WebSocket to self).
// Remote agents still connect via WebSocket as before.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/proxera/agent/pkg/metrics"
	"github.com/proxera/backend/internal/api"
	"github.com/proxera/backend/internal/api/handlers"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/localagent"
	"github.com/proxera/backend/internal/logging"
	"github.com/proxera/backend/internal/models"
	"github.com/proxera/backend/internal/notifications"
)

//go:embed panel/*
var panelFS embed.FS

func main() {
	logging.Setup()
	slog.Info("Proxera Control Node starting...", "component", "control")

	// Load environment variables
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../../.env"); err != nil {
			slog.Info("no .env file found, using environment variables", "component", "control")
		}
	}

	// Validate required secrets
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		slog.Error("JWT_SECRET must be set and at least 32 characters", "component", "control")
		os.Exit(1)
	}

	// Connect to database
	if err := database.Connect(); err != nil {
		slog.Error("failed to connect to database", "component", "control", "error", err)
		os.Exit(1)
	}

	// Initialize database tables
	if err := database.Initialize(); err != nil {
		slog.Error("failed to initialize database", "component", "control", "error", err)
		os.Exit(1)
	}

	// Start WebSocket hub for remote agent connections
	handlers.StartHub()

	// Start background jobs with panic recovery
	var safeGo func(name string, fn func())
	safeGo = func(name string, fn func()) {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("background job crashed, restarting in 5s", "component", "control", "job", name, "panic", r)
					time.Sleep(5 * time.Second)
					safeGo(name, fn)
				}
			}()
			fn()
		}()
	}
	safeGo("cert-renewal", handlers.StartCertRenewalJob)
	safeGo("alert-worker", handlers.StartAlertWorker)
	safeGo("cooldown-cleanup", notifications.StartCooldownCleanup)
	safeGo("token-cleanup", startTokenCleanupJob)

	// --- First-run detection ---
	isDocker := os.Getenv("PROXERA_DOCKER") == "true"
	status := localagent.DetectSystem()
	slog.Info("nginx status", "component", "system", "installed", status.NginxInstalled, "version", status.NginxVersion, "running", status.NginxRunning)
	slog.Info("crowdsec status", "component", "system", "installed", status.CrowdSecInstalled, "running", status.CrowdSecRunning)
	if isDocker {
		slog.Info("running in Docker mode, nginx managed via external container", "component", "system")
		safeGo("logrotate", startLogRotateJob)
	}

	// In Docker mode, nginx is in a separate container — always enable local agent
	enableLocalAgent := status.NginxInstalled || isDocker

	if !enableLocalAgent {
		slog.Warn("nginx is not installed, local proxy management will be unavailable", "component", "system")
		slog.Warn("install nginx 1.25+ and restart the control node to enable local management", "component", "system")
	}

	// --- Local agent setup ---
	var localMgr *localagent.Manager
	if enableLocalAgent {
		if err := localagent.EnsureDirectories(); err != nil {
			slog.Warn("failed to create directories", "component", "local-agent", "error", err)
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
				slog.Error("failed to insert visitor IPs", "component", "local-agent", "error", err)
			}
			return nil
		})

		// Register local agent (deferred until first admin user exists)
		if _, err := localMgr.RegisterLocalAgent(); err != nil {
			slog.Info("deferred local agent registration", "component", "local-agent", "error", err)
			slog.Info("local agent will register after first admin account is created", "component", "local-agent")
		} else {
			localMgr.Start()
		}

		// Make local agent accessible to handlers
		handlers.SetLocalAgent(localMgr)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:               "Proxera Control Node",
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           120 * time.Second,
		BodyLimit:             10 * 1024 * 1024, // 10MB
		DisableStartupMessage: false,
	})

	// Panic recovery — prevents a single panic from crashing the server
	app.Use(fiberrecover.New())

	// Setup API routes (same as standalone backend)
	api.SetupRoutes(app)

	// --- Embedded panel ---
	panelRoot, err := fs.Sub(panelFS, "panel")
	if err != nil {
		slog.Error("failed to create panel sub-filesystem", "component", "control", "error", err)
		os.Exit(1)
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
	slog.Info("control node listening", "component", "control", "address", addr)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan
		slog.Info("received signal, shutting down gracefully", "component", "control", "signal", sig.String())
		if localMgr != nil {
			localMgr.Stop()
		}
		handlers.DrainWebSocketConnections()
		if err := app.Shutdown(); err != nil {
			slog.Error("error during shutdown", "component", "control", "error", err)
		}
		database.DB.Close()
		slog.Info("database connection pool closed", "component", "control")
	}()

	if err := app.Listen(addr); err != nil {
		slog.Error("failed to start server", "component", "control", "error", err)
		os.Exit(1)
	}
}

// startLogRotateJob triggers nginx log rotation daily via docker exec.
// Only intended for Docker mode where the nginx container has the logrotate
// config mounted at /etc/logrotate.d/nginx.
func startLogRotateJob() {
	const container = "proxera-nginx"
	const interval = 24 * time.Hour

	slog.Info("starting daily nginx log rotation job", "component", "logrotate")

	// Run once on startup
	runLogRotate(container)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		runLogRotate(container)
	}
}

func runLogRotate(container string) {
	slog.Info("running nginx log rotation", "component", "logrotate")
	cmd := exec.Command("docker", "exec", container, "logrotate", "/etc/logrotate.d/nginx")
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Error("failed to rotate nginx logs", "component", "logrotate", "error", err, "output", string(output))
	} else {
		slog.Info("nginx log rotation completed successfully", "component", "logrotate")
	}
}

// startTokenCleanupJob periodically deletes expired password reset tokens.
// Tokens have a 1-hour TTL but are only cleaned up per-user on new reset
// requests. This job removes all tokens older than 24 hours every hour.
func startTokenCleanupJob() {
	const interval = 1 * time.Hour

	slog.Info("starting password reset token cleanup job", "component", "token-cleanup")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		result, err := database.DB.Exec(ctx,
			"DELETE FROM password_reset_tokens WHERE created_at < NOW() - INTERVAL '24 hours'")
		cancel()
		if err != nil {
			slog.Error("failed to cleanup expired reset tokens", "component", "token-cleanup", "error", err)
			continue
		}
		if result.RowsAffected() > 0 {
			slog.Info("cleaned up expired reset tokens", "component", "token-cleanup", "deleted", result.RowsAffected())
		}
	}
}
