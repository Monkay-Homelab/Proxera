package models

import (
	"time"
)

type User struct {
	ID            int       `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Password      string    `json:"-"` // Never include password in JSON
	Role          string    `json:"role"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type RegisterRequest struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
