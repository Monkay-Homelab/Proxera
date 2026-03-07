package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/email"
	"github.com/proxera/backend/internal/models"
	"github.com/proxera/backend/internal/settings"
	"golang.org/x/crypto/bcrypt"
)

func emailVerificationEnabled() bool {
	return settings.Get("ENABLE_EMAIL_VERIFICATION", "false") == "true"
}

func generateVerificationToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("crypto/rand.Read failed: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// RegistrationStatus returns whether registration is open, invite-only, or disabled.
func RegistrationStatus(c *fiber.Ctx) error {
	var mode string
	err := database.DB.QueryRow(context.Background(),
		`SELECT value FROM system_settings WHERE key = 'registration_mode'`,
	).Scan(&mode)
	if err != nil {
		mode = "open"
	}

	// If disabled, check if any users exist — if none, allow registration for first admin
	if mode == "disabled" {
		var count int
		database.DB.QueryRow(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&count)
		if count == 0 {
			mode = "open"
		}
	}

	return c.JSON(fiber.Map{"mode": mode})
}

// Register creates a new user account
func Register(c *fiber.Ctx) error {
	var req models.RegisterRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate input
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email, name, and password are required",
		})
	}

	// Validate email format
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid email address",
		})
	}

	// Validate password strength
	if len(req.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password must be at least 8 characters",
		})
	}
	if len(req.Password) > 72 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Password must be 72 characters or less",
		})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// Check registration mode
	var registrationMode string
	err = database.DB.QueryRow(context.Background(),
		`SELECT value FROM system_settings WHERE key = 'registration_mode'`,
	).Scan(&registrationMode)
	if err != nil {
		registrationMode = "open" // default to open if not configured
	}

	switch registrationMode {
	case "disabled":
		var userCount int
		err = database.DB.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM users`,
		).Scan(&userCount)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to check registration status",
			})
		}
		if userCount > 0 {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Registration is currently disabled",
			})
		}
	case "invite":
		var inviteCode string
		err = database.DB.QueryRow(context.Background(),
			`SELECT value FROM system_settings WHERE key = 'invite_code'`,
		).Scan(&inviteCode)
		if err != nil || req.InviteCode != inviteCode {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Invalid or missing invite code",
			})
		}
	case "open":
		// allow all
	default:
		// treat unknown modes as open
	}

	// Create user
	var user models.User
	query := `
		INSERT INTO users (email, name, password, role)
		VALUES ($1, $2, $3, CASE WHEN (SELECT COUNT(*) FROM users) = 0 THEN 'admin' ELSE 'member' END)
		RETURNING id, email, name, role, COALESCE(email_verified, false), created_at, updated_at
	`

	err = database.DB.QueryRow(
		context.Background(),
		query,
		req.Email,
		req.Name,
		string(hashedPassword),
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Email already registered",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	// Send verification email if enabled
	if emailVerificationEnabled() {
		token, tokenErr := generateVerificationToken()
		if tokenErr != nil {
			log.Printf("Failed to generate verification token for user %d: %v", user.ID, tokenErr)
		} else {
			expires := time.Now().Add(SessionExpiry)

			_, err := database.DB.Exec(context.Background(),
				`UPDATE users SET verification_token = $1, verification_token_expires = $2 WHERE id = $3`,
				token, expires, user.ID,
			)
			if err != nil {
				log.Printf("Failed to save verification token for user %d: %v", user.ID, err)
			} else {
				if err := email.SendVerificationEmail(user.Email, user.Name, token); err != nil {
					log.Printf("Failed to send verification email to %s: %v", user.Email, err)
				}
			}
		}
	}

	// If this is the first admin, trigger local agent registration
	if user.Role == "admin" {
		go TryRegisterLocalAgent()
	}

	// Generate JWT token
	jwtToken, err := generateJWT(user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate token",
		})
	}

	resp := fiber.Map{
		"token": jwtToken,
		"user":  user,
	}
	if emailVerificationEnabled() {
		resp["verification_required"] = true
		resp["message"] = "Account created. Please check your email to verify your account."
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}

// Login authenticates a user
func Login(c *fiber.Ctx) error {
	var req models.LoginRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email and password are required",
		})
	}

	// Validate email format
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid email address",
		})
	}

	// Find user
	var user models.User
	var hashedPassword string
	query := `
		SELECT id, email, name, password, role, COALESCE(email_verified, false), created_at, updated_at
		FROM users
		WHERE email = $1
	`

	err := database.DB.QueryRow(context.Background(), query, req.Email).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&hashedPassword,
		&user.Role,
		&user.EmailVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid email or password",
		})
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password))
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid email or password",
		})
	}

	// Check email verification if enabled
	if emailVerificationEnabled() && !user.EmailVerified {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":                 "Email not verified. Please check your inbox for the verification link.",
			"verification_required": true,
		})
	}

	// Generate JWT token
	token, err := generateJWT(user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate token",
		})
	}

	return c.JSON(models.AuthResponse{
		Token: token,
		User:  user,
	})
}

// VerifyEmail verifies a user's email address via token
func VerifyEmail(c *fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing verification token",
		})
	}

	var userID int
	var expires time.Time
	err := database.DB.QueryRow(context.Background(),
		`SELECT id, verification_token_expires FROM users WHERE verification_token = $1`,
		token,
	).Scan(&userID, &expires)

	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid or expired verification token",
		})
	}

	if time.Now().After(expires) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Verification token has expired. Please request a new one.",
		})
	}

	_, err = database.DB.Exec(context.Background(),
		`UPDATE users SET email_verified = true, verification_token = NULL, verification_token_expires = NULL WHERE id = $1`,
		userID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to verify email",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Email verified successfully. You can now log in.",
	})
}

// ResendVerification resends the verification email
func ResendVerification(c *fiber.Ctx) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil || req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	var userID int
	var name string
	var verified bool
	err := database.DB.QueryRow(context.Background(),
		`SELECT id, name, COALESCE(email_verified, false) FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &name, &verified)

	if err != nil {
		// Don't reveal whether the email exists
		return c.JSON(fiber.Map{
			"message": "If that email is registered, a verification link has been sent.",
		})
	}

	if verified {
		return c.JSON(fiber.Map{
			"message": "Email is already verified.",
		})
	}

	token, tokenErr := generateVerificationToken()
	if tokenErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate verification token",
		})
	}
	expires := time.Now().Add(SessionExpiry)

	_, err = database.DB.Exec(context.Background(),
		`UPDATE users SET verification_token = $1, verification_token_expires = $2 WHERE id = $3`,
		token, expires, userID,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate verification token",
		})
	}

	if err := email.SendVerificationEmail(req.Email, name, token); err != nil {
		log.Printf("Failed to send verification email to %s: %v", req.Email, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to send verification email",
		})
	}

	return c.JSON(fiber.Map{
		"message": "If that email is registered, a verification link has been sent.",
	})
}

// Logout handles user logout (stateless — client clears token)
func Logout(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

// generateJWT creates a JWT token for a user
func generateJWT(userID int) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		Issuer:    "proxera-api",
		Audience:  jwt.ClaimStrings{"proxera-panel"},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(JWTTokenExpiry)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}
