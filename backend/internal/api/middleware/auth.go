package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/proxera/backend/internal/database"
)

// jwtSecret is lazily cached to ensure it's read after godotenv.Load() in main().
var jwtSecret []byte

func getJWTSecret() []byte {
	if jwtSecret == nil {
		jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	}
	return jwtSecret
}

// Auth middleware verifies JWT tokens
func Auth(c *fiber.Ctx) error {
	// Get token from Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authorization header",
		})
	}

	// Extract token from "Bearer <token>"
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid authorization format",
		})
	}

	tokenString := parts[1]

	// Check if this is a user API key (pxk_ prefix)
	if strings.HasPrefix(tokenString, "pxk_") {
		userID, err := authenticateAPIKey(tokenString)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		c.Locals("user_id", userID)
		var role string
		var suspended bool
		err = database.DB.QueryRow(context.Background(),
			"SELECT COALESCE(role, 'member'), COALESCE(suspended, false) FROM users WHERE id = $1", userID,
		).Scan(&role, &suspended)
		if err != nil {
			role = "member"
		}
		if suspended {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Account suspended. Contact your administrator for assistance.",
			})
		}
		c.Locals("user_role", role)
		return c.Next()
	}

	// Parse and validate token with issuer and audience checks
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid signing method")
		}
		return getJWTSecret(), nil
	}, jwt.WithIssuer("proxera-api"), jwt.WithAudience("proxera-panel"))

	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Extract user ID from claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid token claims",
		})
	}

	// Try "sub" claim first (new format), fallback to "user_id" (legacy)
	var userID int
	if sub, ok := claims["sub"].(string); ok {
		userID, err = strconv.Atoi(sub)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid user ID in token",
			})
		}
	} else if uid, ok := claims["user_id"].(float64); ok {
		userID = int(uid)
	} else {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid user ID in token",
		})
	}

	// Store user ID in context for handlers
	c.Locals("user_id", userID)

	// Load user role and suspension status
	var role string
	var suspended bool
	err = database.DB.QueryRow(context.Background(),
		"SELECT COALESCE(role, 'member'), COALESCE(suspended, false) FROM users WHERE id = $1", userID,
	).Scan(&role, &suspended)
	if err != nil {
		role = "member"
	}
	if suspended {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Account suspended. Contact your administrator for assistance.",
		})
	}
	c.Locals("user_role", role)

	return c.Next()
}

// authenticateAPIKey validates a user API key and returns the user ID.
func authenticateAPIKey(key string) (int, error) {
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	var userID int
	var expiresAt *time.Time
	err := database.DB.QueryRow(context.Background(),
		`SELECT user_id, expires_at FROM user_api_keys WHERE key_hash = $1`, keyHash,
	).Scan(&userID, &expiresAt)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "Invalid API key")
	}

	if expiresAt != nil && time.Now().After(*expiresAt) {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "API key expired")
	}

	// Update last_used_at in background
	go func() {
		database.DB.Exec(context.Background(),
			`UPDATE user_api_keys SET last_used_at = NOW() WHERE key_hash = $1`, keyHash)
	}()

	return userID, nil
}
