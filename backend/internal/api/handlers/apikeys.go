package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
)

type apiKeyResponse struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	LastUsed  *time.Time `json:"last_used_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// ListAPIKeys returns all API keys for the authenticated user
func ListAPIKeys(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	rows, err := database.DB.Query(context.Background(),
		`SELECT id, name, key_prefix, last_used_at, expires_at, created_at
		 FROM user_api_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list API keys"})
	}
	defer rows.Close()

	keys := []apiKeyResponse{}
	for rows.Next() {
		var k apiKeyResponse
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.LastUsed, &k.ExpiresAt, &k.CreatedAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}
	return c.JSON(keys)
}

// CreateAPIKey generates a new API key
func CreateAPIKey(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)

	var body struct {
		Name      string `json:"name"`
		ExpiresIn string `json:"expires_in"` // "30d", "90d", "365d", "never"
	}
	if err := c.BodyParser(&body); err != nil || body.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name is required"})
	}

	// Generate key: pxk_ + 48 random hex chars
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate key"})
	}
	key := "pxk_" + hex.EncodeToString(raw)
	prefix := key[:12]

	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	var expiresAt *time.Time
	if body.ExpiresIn != "" && body.ExpiresIn != "never" {
		var d time.Duration
		switch body.ExpiresIn {
		case "30d":
			d = 30 * 24 * time.Hour
		case "90d":
			d = 90 * 24 * time.Hour
		case "365d":
			d = 365 * 24 * time.Hour
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid expiration"})
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	var id int64
	err := database.DB.QueryRow(context.Background(),
		`INSERT INTO user_api_keys (user_id, name, key_hash, key_prefix, expires_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, body.Name, keyHash, prefix, expiresAt,
	).Scan(&id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create API key"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":         id,
		"name":       body.Name,
		"key":        key,
		"prefix":     prefix,
		"expires_at": expiresAt,
	})
}

// DeleteAPIKey revokes an API key
func DeleteAPIKey(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	keyID := c.Params("keyId")

	result, err := database.DB.Exec(context.Background(),
		`DELETE FROM user_api_keys WHERE id = $1 AND user_id = $2`, keyID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete API key"})
	}
	if result.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "API key not found"})
	}

	return c.JSON(fiber.Map{"message": "API key revoked"})
}

