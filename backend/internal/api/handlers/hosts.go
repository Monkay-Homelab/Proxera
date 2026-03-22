package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
)

// normalizeUpstreamURL prepends http:// if no scheme is present (e.g. "192.168.1.1:8080" → "http://192.168.1.1:8080")
func normalizeUpstreamURL(u string) string {
	if !strings.Contains(u, "://") {
		return "http://" + u
	}
	return u
}

var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

type HostConfigResponse struct {
	ID            int                 `json:"id"`
	ProviderID    int                 `json:"provider_id"`
	Domain        string              `json:"domain"`
	UpstreamURL   string              `json:"upstream_url"`
	SSL           bool                `json:"ssl"`
	WebSocket     bool                `json:"websocket"`
	AgentID       *int                `json:"agent_id"`
	CertificateID *int                `json:"certificate_id"`
	Config        *HostAdvancedConfig `json:"config,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

type HostConfigRequest struct {
	Domain        string              `json:"domain"`
	UpstreamURL   string              `json:"upstream_url"`
	SSL           bool                `json:"ssl"`
	WebSocket     bool                `json:"websocket"`
	AgentID       *int                `json:"agent_id"`
	CertificateID *int                `json:"certificate_id"`
	Config        *HostAdvancedConfig `json:"config,omitempty"`
}

func verifyProviderOwnership(providerID, userID int, role string) (bool, error) {
	if role == "admin" {
		var exists bool
		err := database.DB.QueryRow(
			context.Background(),
			`SELECT EXISTS(SELECT 1 FROM dns_providers WHERE id = $1)`,
			providerID,
		).Scan(&exists)
		return exists, err
	}
	var exists bool
	err := database.DB.QueryRow(
		context.Background(),
		`SELECT EXISTS(
			SELECT 1 FROM dns_providers WHERE id = $1 AND user_id = $2
			UNION
			SELECT 1 FROM user_dns_providers WHERE dns_provider_id = $1 AND user_id = $2
		)`,
		providerID, userID,
	).Scan(&exists)
	return exists, err
}

func scanHostConfig(configBytes []byte) *HostAdvancedConfig {
	if len(configBytes) == 0 || string(configBytes) == "{}" {
		return nil
	}
	var cfg HostAdvancedConfig
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil
	}
	// Redact password from API responses (password is only needed during deploy)
	if cfg.BasicAuth != nil && cfg.BasicAuth.Password != "" {
		cfg.BasicAuth.Password = "********"
	}
	return &cfg
}

// ListAllHosts returns all hosts across all providers for the current user
func ListAllHosts(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	var rows interface {
		Next() bool
		Scan(dest ...interface{}) error
		Close()
		Err() error
	}
	var err error
	if role == "admin" {
		rows, err = database.DB.Query(
			context.Background(),
			`SELECT h.id, h.provider_id, h.domain, h.upstream_url, h.ssl, h.websocket, h.agent_id, h.certificate_id, h.config, h.created_at, h.updated_at
			 FROM hosts h ORDER BY h.domain`,
		)
	} else {
		rows, err = database.DB.Query(
			context.Background(),
			`SELECT h.id, h.provider_id, h.domain, h.upstream_url, h.ssl, h.websocket, h.agent_id, h.certificate_id, h.config, h.created_at, h.updated_at
			 FROM hosts h
			 WHERE h.user_id = $1
			    OR h.provider_id IN (SELECT dns_provider_id FROM user_dns_providers WHERE user_id = $1)
			 ORDER BY h.domain`,
			userID,
		)
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch hosts"})
	}
	defer rows.Close()

	configs := []HostConfigResponse{}
	for rows.Next() {
		var h HostConfigResponse
		var configBytes []byte
		if err := rows.Scan(&h.ID, &h.ProviderID, &h.Domain, &h.UpstreamURL, &h.SSL, &h.WebSocket, &h.AgentID, &h.CertificateID, &configBytes, &h.CreatedAt, &h.UpdatedAt); err != nil {
			slog.Error("scan error", "component", "hosts", "error", err)
			continue
		}
		h.Config = scanHostConfig(configBytes)
		configs = append(configs, h)
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating hosts", "component", "hosts", "error", err)
	}

	return c.JSON(configs)
}

func ListHostConfigs(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("providerId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}

	exists, err := verifyProviderOwnership(providerID, userID, role)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	rows, err := database.DB.Query(
		context.Background(),
		`SELECT id, provider_id, domain, upstream_url, ssl, websocket, agent_id, certificate_id, config, created_at, updated_at
		 FROM hosts WHERE provider_id = $1 AND user_id = $2 ORDER BY domain`,
		providerID, userID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch host configs"})
	}
	defer rows.Close()

	configs := []HostConfigResponse{}
	for rows.Next() {
		var h HostConfigResponse
		var configBytes []byte
		if err := rows.Scan(&h.ID, &h.ProviderID, &h.Domain, &h.UpstreamURL, &h.SSL, &h.WebSocket, &h.AgentID, &h.CertificateID, &configBytes, &h.CreatedAt, &h.UpdatedAt); err != nil {
			slog.Error("scan error", "component", "hosts", "error", err)
			continue
		}
		h.Config = scanHostConfig(configBytes)
		configs = append(configs, h)
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating host configs", "component", "hosts", "error", err)
	}

	return c.JSON(configs)
}

func GetHostConfig(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("providerId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}
	configID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid config ID"})
	}

	exists, err := verifyProviderOwnership(providerID, userID, role)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var h HostConfigResponse
	var configBytes []byte
	err = database.DB.QueryRow(
		context.Background(),
		`SELECT id, provider_id, domain, upstream_url, ssl, websocket, agent_id, certificate_id, config, created_at, updated_at
		 FROM hosts WHERE id = $1 AND provider_id = $2 AND user_id = $3`,
		configID, providerID, userID,
	).Scan(&h.ID, &h.ProviderID, &h.Domain, &h.UpstreamURL, &h.SSL, &h.WebSocket, &h.AgentID, &h.CertificateID, &configBytes, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Host config not found"})
	}
	h.Config = scanHostConfig(configBytes)

	return c.JSON(h)
}

func CreateHostConfig(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("providerId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}

	exists, err := verifyProviderOwnership(providerID, userID, role)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var req HostConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Domain == "" || req.UpstreamURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Domain and upstream_url are required"})
	}
	if len(req.Domain) > 253 || !domainRegex.MatchString(req.Domain) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid domain format"})
	}
	req.UpstreamURL = normalizeUpstreamURL(req.UpstreamURL)
	if _, err := url.ParseRequestURI(req.UpstreamURL); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid upstream URL format"})
	}

	var configJSON []byte
	if req.Config != nil {
		var marshalErr error
		configJSON, marshalErr = json.Marshal(req.Config)
		if marshalErr != nil {
			slog.Error("failed to marshal config", "component", "hosts", "error", marshalErr)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid config"})
		}
	} else {
		configJSON = []byte("{}")
	}

	var h HostConfigResponse
	var configBytes []byte
	err = database.DB.QueryRow(
		context.Background(),
		`INSERT INTO hosts (user_id, provider_id, domain, upstream_url, ssl, websocket, agent_id, certificate_id, config)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, provider_id, domain, upstream_url, ssl, websocket, agent_id, certificate_id, config, created_at, updated_at`,
		userID, providerID, req.Domain, req.UpstreamURL, req.SSL, req.WebSocket, req.AgentID, req.CertificateID, configJSON,
	).Scan(&h.ID, &h.ProviderID, &h.Domain, &h.UpstreamURL, &h.SSL, &h.WebSocket, &h.AgentID, &h.CertificateID, &configBytes, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		slog.Error("create host config failed", "component", "hosts", "domain", req.Domain, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create host config"})
	}
	h.Config = scanHostConfig(configBytes)

	// Auto-deploy to agent if assigned
	var deployError string
	if h.AgentID != nil {
		if err := deployHostToAgent(userID, *h.AgentID, h.Domain, h.UpstreamURL, h.SSL, h.WebSocket, h.CertificateID, h.Config); err != nil {
			deployError = err.Error()
			slog.Error("auto-deploy failed for host", "component", "hosts", "domain", h.Domain, "error", err)
		}
	}

	response := fiber.Map{
		"id":             h.ID,
		"provider_id":    h.ProviderID,
		"domain":         h.Domain,
		"upstream_url":   h.UpstreamURL,
		"ssl":            h.SSL,
		"websocket":      h.WebSocket,
		"agent_id":       h.AgentID,
		"certificate_id": h.CertificateID,
		"config":         h.Config,
		"created_at":     h.CreatedAt,
		"updated_at":     h.UpdatedAt,
	}
	if deployError != "" {
		response["deploy_error"] = deployError
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

func UpdateHostConfig(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("providerId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}
	configID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid config ID"})
	}

	exists, err := verifyProviderOwnership(providerID, userID, role)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	var req HostConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Domain == "" || req.UpstreamURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Domain and upstream_url are required"})
	}
	if len(req.Domain) > 253 || !domainRegex.MatchString(req.Domain) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid domain format"})
	}
	req.UpstreamURL = normalizeUpstreamURL(req.UpstreamURL)
	if _, err := url.ParseRequestURI(req.UpstreamURL); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid upstream URL format"})
	}

	var configJSON []byte
	if req.Config != nil {
		var marshalErr error
		configJSON, marshalErr = json.Marshal(req.Config)
		if marshalErr != nil {
			slog.Error("failed to marshal config", "component", "hosts", "error", marshalErr)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid config"})
		}
	} else {
		configJSON = []byte("{}")
	}

	var h HostConfigResponse
	var configBytes []byte
	err = database.DB.QueryRow(
		context.Background(),
		`UPDATE hosts SET domain = $1, upstream_url = $2, ssl = $3, websocket = $4, agent_id = $5, certificate_id = $6, config = $7, updated_at = NOW()
		 WHERE id = $8 AND provider_id = $9 AND user_id = $10
		 RETURNING id, provider_id, domain, upstream_url, ssl, websocket, agent_id, certificate_id, config, created_at, updated_at`,
		req.Domain, req.UpstreamURL, req.SSL, req.WebSocket, req.AgentID, req.CertificateID, configJSON, configID, providerID, userID,
	).Scan(&h.ID, &h.ProviderID, &h.Domain, &h.UpstreamURL, &h.SSL, &h.WebSocket, &h.AgentID, &h.CertificateID, &configBytes, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Host config not found"})
	}
	h.Config = scanHostConfig(configBytes)

	// Auto-deploy to agent if assigned
	var deployError string
	if h.AgentID != nil {
		if err := deployHostToAgent(userID, *h.AgentID, h.Domain, h.UpstreamURL, h.SSL, h.WebSocket, h.CertificateID, h.Config); err != nil {
			deployError = err.Error()
			slog.Error("auto-deploy failed for host", "component", "hosts", "domain", h.Domain, "error", err)
		}
	}

	response := fiber.Map{
		"id":             h.ID,
		"provider_id":    h.ProviderID,
		"domain":         h.Domain,
		"upstream_url":   h.UpstreamURL,
		"ssl":            h.SSL,
		"websocket":      h.WebSocket,
		"agent_id":       h.AgentID,
		"certificate_id": h.CertificateID,
		"config":         h.Config,
		"created_at":     h.CreatedAt,
		"updated_at":     h.UpdatedAt,
	}
	if deployError != "" {
		response["deploy_error"] = deployError
	}

	return c.JSON(response)
}

func DeleteHostConfig(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)
	providerID, err := strconv.Atoi(c.Params("providerId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}
	configID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid config ID"})
	}

	exists, err := verifyProviderOwnership(providerID, userID, role)
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "DNS provider not found"})
	}

	// Query host's agent_id and domain before deletion for auto-remove
	var hostDomain string
	var hostAgentID *int
	_ = database.DB.QueryRow(
		context.Background(),
		`SELECT domain, agent_id FROM hosts WHERE id = $1 AND provider_id = $2 AND user_id = $3`,
		configID, providerID, userID,
	).Scan(&hostDomain, &hostAgentID)

	result, err := database.DB.Exec(
		context.Background(),
		`DELETE FROM hosts WHERE id = $1 AND provider_id = $2 AND user_id = $3`,
		configID, providerID, userID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete host config"})
	}

	if result.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Host config not found"})
	}

	// Auto-remove from agent if was assigned
	if hostAgentID != nil && hostDomain != "" {
		if err := removeHostFromAgent(*hostAgentID, hostDomain); err != nil {
			slog.Error("auto-remove failed for host", "component", "hosts", "domain", hostDomain, "error", err)
		}
	}

	return c.JSON(fiber.Map{"message": "Host config deleted"})
}
