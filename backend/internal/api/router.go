package api

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/proxera/backend/internal/api/handlers"
	"github.com/proxera/backend/internal/api/middleware"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/reqstats"
)

func SetupRoutes(app *fiber.App) {
	// Middleware
	app.Use(logger.New())
	app.Use(reqstats.TrackRequests())
	corsOrigins := os.Getenv("CORS_ORIGINS")
	if corsOrigins == "" {
		// Default to same-origin only — no cross-origin requests allowed
		// Set CORS_ORIGINS to explicitly allow specific origins (e.g., "https://proxera.example.com")
		corsOrigins = ""
	}
	if corsOrigins != "" {
		app.Use(cors.New(cors.Config{
			AllowOrigins:     corsOrigins,
			AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
			AllowMethods:     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
			AllowCredentials: true,
		}))
	}

	// API routes
	api := app.Group("/api")

	// Auth routes (public, rate limited)
	auth := api.Group("/auth")
	// Registration status is a lightweight read — no rate limit
	auth.Get("/registration-status", handlers.RegistrationStatus)
	auth.Use(limiter.New(limiter.Config{
		Max:        20,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Too many requests, please try again later",
			})
		},
	}))
	auth.Post("/register", handlers.Register)
	auth.Post("/login", handlers.Login)
	auth.Post("/logout", handlers.Logout)
	auth.Post("/reset-password", handlers.ResetPassword)
	auth.Get("/verify-email", handlers.VerifyEmail)
	auth.Post("/resend-verification", handlers.ResendVerification)

	// Setup routes (protected — first-run setup steps)
	setup := api.Group("/setup")
	setup.Use(middleware.Auth, middleware.FlatRateLimit)
	setup.Get("/status", handlers.SetupStatus)
	setup.Post("/crowdsec-eula", middleware.AdminOnly, handlers.AcceptCrowdSecEULA)

	// System routes (protected — all authenticated users)
	system := api.Group("/system")
	system.Use(middleware.Auth, middleware.FlatRateLimit)
	system.Get("/status", handlers.GetSystemStatus)

	// User routes (protected)
	user := api.Group("/user")
	user.Use(middleware.Auth, middleware.FlatRateLimit)
	user.Get("/me", handlers.GetCurrentUser)
	user.Get("/nav-context", handlers.GetNavContext)
	user.Post("/change-password", handlers.ChangePassword)
	user.Delete("/me", handlers.DeleteAccount)
	user.Get("/api-keys", handlers.ListAPIKeys)
	user.Post("/api-keys", handlers.CreateAPIKey)
	user.Delete("/api-keys/:keyId", handlers.DeleteAPIKey)

	// Agents routes (protected - for panel users, viewers blocked from writes)
	agents := api.Group("/agents")
	agents.Use(middleware.Auth, middleware.FlatRateLimit, middleware.RejectViewer)
	agents.Post("/register", handlers.RegisterAgent)
	agents.Get("/", handlers.ListAgents)
	agents.Get("/:agentId", handlers.GetAgent)
	agents.Patch("/:agentId", handlers.RenameAgent)
	agents.Delete("/:agentId", handlers.DeleteAgent)
	agents.Post("/:agentId/update", handlers.UpdateAgent)
	agents.Get("/:agentId/logs", handlers.GetAgentLogs)
	agents.Post("/:agentId/upgrade-nginx", handlers.UpgradeAgentNginx)
	agents.Post("/:agentId/deploy", handlers.DeployAllToAgent)
	agents.Post("/:agentId/reload", handlers.ReloadAgent)
	agents.Get("/:agentId/metrics", handlers.GetAgentMetrics)
	agents.Get("/:agentId/metrics/live", handlers.GetAgentMetricsLive)
	agents.Post("/:agentId/metrics-interval", handlers.SetMetricsInterval)

	// CrowdSec routes
	crowdsec := agents.Group("/:agentId/crowdsec")
	crowdsec.Get("/status", handlers.CrowdSecStatus)
	crowdsec.Post("/install", handlers.CrowdSecInstall)
	crowdsec.Post("/uninstall", handlers.CrowdSecUninstall)
	crowdsec.Get("/decisions", handlers.CrowdSecListDecisions)
	crowdsec.Post("/decisions", handlers.CrowdSecAddDecision)
	crowdsec.Delete("/decisions/:decisionId", handlers.CrowdSecDeleteDecision)
	crowdsec.Get("/alerts", handlers.CrowdSecListAlerts)
	crowdsec.Delete("/alerts/:alertId", handlers.CrowdSecDeleteAlert)
	crowdsec.Get("/collections", handlers.CrowdSecListCollections)
	crowdsec.Post("/collections", handlers.CrowdSecInstallCollection)
	crowdsec.Delete("/collections/*", handlers.CrowdSecRemoveCollection)
	crowdsec.Get("/bouncers", handlers.CrowdSecListBouncers)
	crowdsec.Post("/bouncers", handlers.CrowdSecInstallBouncer)
	crowdsec.Delete("/bouncers/:name", handlers.CrowdSecRemoveBouncer)
	crowdsec.Get("/metrics", handlers.CrowdSecGetMetrics)
	crowdsec.Get("/whitelist", handlers.CrowdSecListWhitelist)
	crowdsec.Post("/whitelist", handlers.CrowdSecAddWhitelist)
	crowdsec.Delete("/whitelist/*", handlers.CrowdSecRemoveWhitelist)
	crowdsec.Get("/ban-duration", handlers.CrowdSecGetBanDuration)
	crowdsec.Put("/ban-duration", handlers.CrowdSecSetBanDuration)

	// Admin routes (protected, admin only)
	admin := api.Group("/admin")
	admin.Use(middleware.Auth, middleware.AdminOnly)
	admin.Get("/stats", handlers.AdminGetStats)
	admin.Get("/stats/storage", handlers.AdminGetUserStorage)
	admin.Get("/stats/health", handlers.AdminGetDashboardHealth)
	admin.Get("/stats/system", handlers.AdminGetSystemHealth)
	admin.Get("/users", handlers.AdminListUsers)
	admin.Get("/users/:id", handlers.AdminGetUser)
	admin.Patch("/users/:id", handlers.AdminUpdateUser)
	admin.Post("/users/:id/suspend", handlers.AdminSuspendUser)
	admin.Post("/users/:id/unsuspend", handlers.AdminUnsuspendUser)
	admin.Post("/users/:id/password-reset", handlers.AdminForcePasswordReset)
	admin.Get("/users/:id/assignments", handlers.AdminGetUserAssignments)
	admin.Post("/users/:id/assignments/agents", handlers.AdminAssignAgentToUser)
	admin.Delete("/users/:id/assignments/agents/:agentId", handlers.AdminRemoveAgentFromUser)
	admin.Post("/users/:id/assignments/providers", handlers.AdminAssignProviderToUser)
	admin.Delete("/users/:id/assignments/providers/:providerId", handlers.AdminRemoveProviderFromUser)
	admin.Get("/agents", handlers.AdminListAgents)
	admin.Get("/certificates", handlers.AdminListCertificates)
	admin.Get("/hosts", handlers.AdminListHosts)
	admin.Get("/metrics", handlers.AdminGetPlatformMetrics)
	admin.Get("/alerts", handlers.AdminListAlerts)
	admin.Get("/alerts/stats", handlers.AdminAlertStats)
	admin.Get("/settings", handlers.AdminGetSettings)
	admin.Put("/settings", handlers.AdminUpdateSettings)

	// Alert routes (protected, viewers blocked from writes)
	alerts := api.Group("/alerts")
	alerts.Use(middleware.Auth, middleware.FlatRateLimit, middleware.RejectViewer)
	alerts.Get("/rules", handlers.ListAlertRules)
	alerts.Post("/rules", handlers.CreateAlertRule)
	alerts.Patch("/rules/:id", handlers.UpdateAlertRule)
	alerts.Delete("/rules/:id", handlers.DeleteAlertRule)
	alerts.Get("/channels", handlers.ListChannels)
	alerts.Post("/channels", handlers.CreateChannel)
	alerts.Patch("/channels/:id", handlers.UpdateChannel)
	alerts.Delete("/channels/:id", handlers.DeleteChannel)
	alerts.Post("/channels/:id/test", handlers.TestChannel)
	alerts.Get("/history", handlers.ListAlertHistory)
	alerts.Post("/history/:id/resolve", handlers.ResolveAlert)
	alerts.Post("/quick-setup", handlers.QuickSetupAlerts)

	// Global metrics routes (protected)
	metrics := api.Group("/metrics")
	metrics.Use(middleware.Auth, middleware.FlatRateLimit)
	metrics.Get("/", handlers.GetGlobalMetrics)
	metrics.Get("/visitors", handlers.GetVisitorIPs)
	metrics.Get("/blocked", handlers.GetRecentBlocked)
	metrics.Get("/logs", handlers.GetNginxLogs)
	metrics.Get("/export", handlers.ExportMetrics)

	// Hosts routes (protected, viewers blocked from writes)
	hosts := api.Group("/hosts")
	hosts.Use(middleware.Auth, middleware.FlatRateLimit, middleware.RejectViewer)
	hosts.Get("/all", handlers.ListAllHosts)
	hosts.Get("/:providerId/configs", handlers.ListHostConfigs)
	hosts.Get("/:providerId/configs/:id", handlers.GetHostConfig)
	hosts.Post("/:providerId/configs", handlers.CreateHostConfig)
	hosts.Patch("/:providerId/configs/:id", handlers.UpdateHostConfig)
	hosts.Delete("/:providerId/configs/:id", handlers.DeleteHostConfig)
	hosts.Get("/:providerId/configs/:id/backups", handlers.ListHostBackups)
	hosts.Get("/:providerId/configs/:id/backups/:filename", handlers.GetHostBackupContent)
	hosts.Post("/:providerId/configs/:id/backups/restore", handlers.RestoreHostBackup)

	// DNS routes (protected, viewers blocked from writes)
	dns := api.Group("/dns")
	dns.Use(middleware.Auth, middleware.FlatRateLimit, middleware.RejectViewer)
	dns.Post("/providers", handlers.AddDNSProvider)
	dns.Get("/providers", handlers.ListDNSProviders)
	dns.Delete("/providers/:id", handlers.DeleteDNSProvider)
	dns.Post("/export", handlers.ExportBackup)
	dns.Post("/import", handlers.ImportBackup)
	dns.Get("/providers/:id/records", handlers.ListDNSRecords)
	dns.Post("/providers/:id/records", handlers.CreateDNSRecord)
	dns.Patch("/providers/:id/records/:recordId", handlers.UpdateDNSRecord)
	dns.Delete("/providers/:id/records/:recordId", handlers.DeleteDNSRecord)
	dns.Patch("/providers/:id/records/:recordId/agent", handlers.AssignDNSRecordAgent)
	dns.Post("/providers/:id/records/:recordId/ddns-sync", handlers.DDNSSyncRecord)
	dns.Post("/providers/:id/sync", handlers.SyncDNSRecords)

	// Certificate routes (protected, viewers blocked from writes)
	certs := api.Group("/certificates")
	certs.Use(middleware.Auth, middleware.FlatRateLimit, middleware.RejectViewer)
	certs.Get("/", handlers.ListCertificates)
	certs.Post("/", limiter.New(limiter.Config{
		Max:        20,
		Expiration: 1 * time.Hour,
		KeyGenerator: func(c *fiber.Ctx) string {
			userID, _ := c.Locals("user_id").(int)
			return fmt.Sprintf("cert_issue_%d", userID)
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Certificate issuance rate limit exceeded (20 per hour). Try again later.",
			})
		},
	}), handlers.IssueCertificate)
	certs.Get("/:id", handlers.GetCertificate)
	certs.Post("/:id/retry", handlers.RetryCertificate)
	certs.Delete("/:id", handlers.DeleteCertificate)

	// Agent update routes (public)
	agent := api.Group("/agent")
	agent.Get("/version", handlers.GetAgentVersion)
	agent.Get("/download/:filename", handlers.DownloadAgent)
	agent.Get("/checksum/:filename", handlers.GetAgentChecksum)

	// Install script
	app.Get("/install.sh", func(c *fiber.Ctx) error {
		return c.SendFile("./install.sh")
	})

	// WebSocket route for agents
	app.Use("/ws/agent", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws/agent", middleware.AgentAuth, websocket.New(handlers.AgentWebSocket))

	// Health check (liveness)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "proxera-api",
		})
	})

	// Readiness check (deep health — verifies database connectivity)
	app.Get("/ready", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		start := time.Now()
		err := database.DB.Ping(ctx)
		latencyMs := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "error",
				"checks": fiber.Map{
					"database": fiber.Map{
						"status": "error",
						"error":  err.Error(),
					},
				},
			})
		}

		return c.JSON(fiber.Map{
			"status": "ok",
			"checks": fiber.Map{
				"database": fiber.Map{
					"status":     "ok",
					"latency_ms": latencyMs,
				},
			},
		})
	})
}
