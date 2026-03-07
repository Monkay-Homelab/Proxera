package handlers

import "time"

// WebSocket connection timeouts
const (
	// WSPongTimeout is how long to wait for a pong before considering the connection dead.
	WSPongTimeout = 90 * time.Second

	// WSPingInterval is how often the server sends pings to agents.
	WSPingInterval = 30 * time.Second

	// WSWriteDeadline is the write deadline for WebSocket control messages.
	WSWriteDeadline = 10 * time.Second
)

// Agent command response timeouts — grouped by expected duration.
const (
	// CmdTimeoutFast is for quick read-only commands (status, info, list, logs).
	CmdTimeoutFast = 10 * time.Second

	// CmdTimeoutDefault is for standard commands (deploy host, remove host, install collection).
	CmdTimeoutDefault = 15 * time.Second

	// CmdTimeoutMedium is for commands that may take a bit longer (reload, get whitelist).
	CmdTimeoutMedium = 30 * time.Second

	// CmdTimeoutSlow is for commands that involve package operations or large data transfers.
	CmdTimeoutSlow = 60 * time.Second

	// CmdTimeoutLong is for commands that may take several minutes (enroll, full metrics).
	CmdTimeoutLong = 120 * time.Second
)

// Agent offline detection
const (
	// AgentOfflineThreshold is how long since last heartbeat before marking an agent offline.
	AgentOfflineThreshold = 90 * time.Second
)

// HTTP client timeouts
const (
	// HTTPClientTimeout is the default timeout for outbound HTTP requests.
	HTTPClientTimeout = 10 * time.Second

	// HTTPClientTimeoutLong is for longer outbound HTTP requests (DNS sync, etc.).
	HTTPClientTimeoutLong = 15 * time.Second
)

// Authentication durations
const (
	// JWTTokenExpiry is how long a JWT access token is valid.
	JWTTokenExpiry = 4 * time.Hour

	// SessionExpiry is how long a session/refresh token is valid.
	SessionExpiry = 24 * time.Hour

	// CertExpiryWarning is how far in advance to warn about expiring certificates.
	CertExpiryWarning = 30 * 24 * time.Hour
)
