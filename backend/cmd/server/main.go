package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/proxera/backend/internal/api"
	"github.com/proxera/backend/internal/api/handlers"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/logging"
	"github.com/proxera/backend/internal/notifications"
)

func main() {
	logging.Setup()

	// Load environment variables - try multiple paths
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

	// Start WebSocket hub for agent connections
	handlers.StartHub()

	// Start background jobs with panic recovery
	var safeGo func(name string, fn func())
	safeGo = func(name string, fn func()) {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] %s crashed: %v — restarting in 5s", name, r)
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

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Proxera API",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    10 * 1024 * 1024, // 10MB
	})

	// Panic recovery — prevents a single panic from crashing the server
	app.Use(fiberrecover.New())

	// Setup routes
	api.SetupRoutes(app)

	// Get port from env or use default
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	// Start server
	host := os.Getenv("API_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	log.Printf("Server starting on %s\n", addr)

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received %s, shutting down gracefully...", sig)
		if err := app.Shutdown(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	if err := app.Listen(addr); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
