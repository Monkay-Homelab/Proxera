package handlers

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
)

// AdminGetUserAssignments returns the agents and DNS providers assigned to a user.
func AdminGetUserAssignments(c *fiber.Ctx) error {
	userID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Assigned agents
	type assignedAgent struct {
		AgentDBID int    `json:"id"`
		AgentID   string `json:"agent_id"`
		Name      string `json:"name"`
	}
	var agents []assignedAgent
	agentRows, err := database.DB.Query(ctx,
		`SELECT a.id, a.agent_id, a.name FROM user_agents ua JOIN agents a ON ua.agent_id = a.id WHERE ua.user_id = $1 ORDER BY a.name`,
		userID,
	)
	if err == nil {
		defer agentRows.Close()
		for agentRows.Next() {
			var a assignedAgent
			if err := agentRows.Scan(&a.AgentDBID, &a.AgentID, &a.Name); err == nil {
				agents = append(agents, a)
			}
		}
	}
	if agents == nil {
		agents = []assignedAgent{}
	}

	// Assigned DNS providers
	type assignedProvider struct {
		ID       int    `json:"id"`
		Provider string `json:"provider"`
		Domain   string `json:"domain"`
	}
	var providers []assignedProvider
	providerRows, err := database.DB.Query(ctx,
		`SELECT dp.id, dp.provider, COALESCE(dp.domain, '') FROM user_dns_providers udp JOIN dns_providers dp ON udp.dns_provider_id = dp.id WHERE udp.user_id = $1 ORDER BY dp.domain`,
		userID,
	)
	if err == nil {
		defer providerRows.Close()
		for providerRows.Next() {
			var p assignedProvider
			if err := providerRows.Scan(&p.ID, &p.Provider, &p.Domain); err == nil {
				providers = append(providers, p)
			}
		}
	}
	if providers == nil {
		providers = []assignedProvider{}
	}

	return c.JSON(fiber.Map{
		"agents":    agents,
		"providers": providers,
	})
}

// AdminAssignAgentToUser assigns an agent to a user.
func AdminAssignAgentToUser(c *fiber.Ctx) error {
	userID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var body struct {
		AgentID int `json:"agent_id"`
	}
	if err := c.BodyParser(&body); err != nil || body.AgentID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent_id is required"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = database.DB.Exec(ctx,
		`INSERT INTO user_agents (user_id, agent_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, body.AgentID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign agent"})
	}

	return c.JSON(fiber.Map{"message": "Agent assigned"})
}

// AdminRemoveAgentFromUser removes an agent assignment from a user.
func AdminRemoveAgentFromUser(c *fiber.Ctx) error {
	userID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	agentID, err := strconv.Atoi(c.Params("agentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid agent ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = database.DB.Exec(ctx,
		`DELETE FROM user_agents WHERE user_id = $1 AND agent_id = $2`,
		userID, agentID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to remove agent assignment"})
	}

	return c.JSON(fiber.Map{"message": "Agent assignment removed"})
}

// AdminAssignProviderToUser assigns a DNS provider to a user.
func AdminAssignProviderToUser(c *fiber.Ctx) error {
	userID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var body struct {
		ProviderID int `json:"provider_id"`
	}
	if err := c.BodyParser(&body); err != nil || body.ProviderID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "provider_id is required"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = database.DB.Exec(ctx,
		`INSERT INTO user_dns_providers (user_id, dns_provider_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, body.ProviderID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign provider"})
	}

	return c.JSON(fiber.Map{"message": "Provider assigned"})
}

// AdminRemoveProviderFromUser removes a DNS provider assignment from a user.
func AdminRemoveProviderFromUser(c *fiber.Ctx) error {
	userID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	providerID, err := strconv.Atoi(c.Params("providerId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid provider ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = database.DB.Exec(ctx,
		`DELETE FROM user_dns_providers WHERE user_id = $1 AND dns_provider_id = $2`,
		userID, providerID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to remove provider assignment"})
	}

	return c.JSON(fiber.Map{"message": "Provider assignment removed"})
}
