package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/agent/pkg/types"
	"github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/models"
)

// lookupAgentStringID converts an integer agent DB ID to the string agent_id used by the WebSocket hub
func lookupAgentStringID(agentDBID int) (string, error) {
	var agentStringID string
	err := database.DB.QueryRow(
		context.Background(),
		`SELECT agent_id FROM agents WHERE id = $1`,
		agentDBID,
	).Scan(&agentStringID)
	if err != nil {
		return "", fmt.Errorf("agent not found: %w", err)
	}
	return agentStringID, nil
}

// buildHostPayload builds the payload map for an apply_host command
func buildHostPayload(userID int, domain, upstreamURL string, ssl, websocket bool, certID *int, config *HostAdvancedConfig) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"domain":       domain,
		"upstream_url": upstreamURL,
		"ssl":          ssl,
		"websocket":    websocket,
	}

	if ssl && certID != nil {
		var certPEM, encPrivateKey, issuerPEM *string
		err := database.DB.QueryRow(
			context.Background(),
			`SELECT certificate_pem, private_key_pem, issuer_pem FROM certificates WHERE id = $1 AND user_id = $2`,
			*certID, userID,
		).Scan(&certPEM, &encPrivateKey, &issuerPEM)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch certificate: %w", err)
		}

		if certPEM != nil {
			payload["cert_pem"] = *certPEM
		}
		if issuerPEM != nil {
			payload["issuer_pem"] = *issuerPEM
		}
		if encPrivateKey != nil && *encPrivateKey != "" {
			decrypted, err := crypto.Decrypt(*encPrivateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt private key: %w", err)
			}
			payload["key_pem"] = decrypted
		}
	}

	// Pass advanced config through to agent
	if config != nil {
		configJSON, err := json.Marshal(config)
		if err == nil && string(configJSON) != "{}" {
			// Unmarshal to generic map so it merges cleanly into payload
			var configMap map[string]interface{}
			if json.Unmarshal(configJSON, &configMap) == nil {
				payload["config"] = configMap
			}
		}
	}

	return payload, nil
}

// payloadToHost converts a deploy payload map to a types.Host for local agent dispatch.
func payloadToHost(payload map[string]interface{}) types.Host {
	h := types.Host{
		Domain:      stringVal(payload, "domain"),
		UpstreamURL: stringVal(payload, "upstream_url"),
		SSL:         boolVal(payload, "ssl"),
		WebSocket:   boolVal(payload, "websocket"),
		CertPEM:     stringVal(payload, "cert_pem"),
		KeyPEM:      stringVal(payload, "key_pem"),
		IssuerPEM:   stringVal(payload, "issuer_pem"),
	}
	if configRaw, ok := payload["config"]; ok && configRaw != nil {
		configJSON, err := json.Marshal(configRaw)
		if err == nil {
			var cfg types.AdvancedConfig
			if json.Unmarshal(configJSON, &cfg) == nil {
				h.Config = &cfg
			}
		}
	}
	return h
}

func stringVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func boolVal(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// deployHostToAgent sends an apply_host command to the specified agent
func deployHostToAgent(userID, agentDBID int, domain, upstreamURL string, ssl, websocket bool, certID *int, config *HostAdvancedConfig) error {
	agentStringID, err := lookupAgentStringID(agentDBID)
	if err != nil {
		return err
	}

	payload, err := buildHostPayload(userID, domain, upstreamURL, ssl, websocket, certID, config)
	if err != nil {
		return err
	}

	// Local agent: dispatch directly
	if IsLocalAgent(agentStringID) {
		host := payloadToHost(payload)
		if err := localAgent.ApplyHost(host); err != nil {
			return fmt.Errorf("deploy failed: %w", err)
		}
		log.Printf("[Deploy] Host %s deployed to local agent", domain)
		return nil
	}

	command := models.AgentCommand{
		Type:    "apply_host",
		Payload: payload,
	}

	response, err := SendCommandAndWaitForResponse(agentStringID, command, CmdTimeoutDefault)
	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	log.Printf("[Deploy] Host %s deployed to agent %s: %s", domain, agentStringID, response)
	return nil
}

// removeHostFromAgent sends a remove_host command to the specified agent
func removeHostFromAgent(agentDBID int, domain string) error {
	agentStringID, err := lookupAgentStringID(agentDBID)
	if err != nil {
		return err
	}

	// Local agent: dispatch directly
	if IsLocalAgent(agentStringID) {
		if err := localAgent.RemoveHost(domain); err != nil {
			return fmt.Errorf("remove failed: %w", err)
		}
		log.Printf("[Deploy] Host %s removed from local agent", domain)
		return nil
	}

	command := models.AgentCommand{
		Type: "remove_host",
		Payload: map[string]interface{}{
			"domain": domain,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentStringID, command, CmdTimeoutDefault)
	if err != nil {
		return fmt.Errorf("remove failed: %w", err)
	}

	log.Printf("[Deploy] Host %s removed from agent %s: %s", domain, agentStringID, response)
	return nil
}

// DeployAllToAgent handles POST /api/agents/:agentId/deploy
func DeployAllToAgent(c *fiber.Ctx) error {
	agentID := c.Params("agentId")

	// Verify agent access and get DB ID
	var agentDBID int
	if err := verifyAgentAccessByID(c, agentID, &agentDBID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Agent not found"})
	}

	// Get all hosts assigned to this agent
	rows, err := database.DB.Query(
		context.Background(),
		`SELECT user_id, domain, upstream_url, ssl, websocket, certificate_id, config FROM hosts WHERE agent_id = $1`,
		agentDBID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch hosts"})
	}
	defer rows.Close()

	var hosts []map[string]interface{}
	for rows.Next() {
		var hostUserID int
		var domain, upstreamURL string
		var ssl, ws bool
		var certID *int
		var configBytes []byte
		if err := rows.Scan(&hostUserID, &domain, &upstreamURL, &ssl, &ws, &certID, &configBytes); err != nil {
			continue
		}

		var config *HostAdvancedConfig
		if configBytes != nil && len(configBytes) > 0 && string(configBytes) != "{}" {
			config = &HostAdvancedConfig{}
			if err := json.Unmarshal(configBytes, config); err != nil {
				log.Printf("[Deploy] Failed to parse config for %s: %v", domain, err)
				config = nil
			}
		}

		payload, err := buildHostPayload(hostUserID, domain, upstreamURL, ssl, ws, certID, config)
		if err != nil {
			log.Printf("[Deploy] Failed to build payload for %s: %v", domain, err)
			continue
		}
		hosts = append(hosts, payload)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Deploy] Error iterating hosts for agent %s: %v", agentID, err)
	}

	// Local agent: dispatch directly
	if IsLocalAgent(agentID) {
		var typedHosts []types.Host
		for _, payload := range hosts {
			typedHosts = append(typedHosts, payloadToHost(payload))
		}
		applied, err := localAgent.ApplyAll(typedHosts)
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": fmt.Sprintf("Deploy failed: %v", err),
			})
		}
		return c.JSON(fiber.Map{
			"message": fmt.Sprintf("Deployed %d host(s) to local agent", applied),
		})
	}

	command := models.AgentCommand{
		Type: "apply",
		Payload: map[string]interface{}{
			"hosts": hosts,
		},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutMedium)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fmt.Sprintf("Deploy failed: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"message": response,
	})
}

// ReloadAgent handles POST /api/agents/:agentId/reload
func ReloadAgent(c *fiber.Ctx) error {
	agentID := c.Params("agentId")

	// Verify agent access
	var id int
	if err := verifyAgentAccessByID(c, agentID, &id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Agent not found"})
	}

	// Local agent: dispatch directly
	if IsLocalAgent(agentID) {
		if err := localAgent.Reload(); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": fmt.Sprintf("Reload failed: %v", err),
			})
		}
		return c.JSON(fiber.Map{
			"message": "Nginx reloaded successfully",
		})
	}

	command := models.AgentCommand{
		Type:    "reload",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fmt.Sprintf("Reload failed: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"message": response,
	})
}
