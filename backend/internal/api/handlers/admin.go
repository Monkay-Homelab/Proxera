package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/email"
	"github.com/proxera/backend/internal/reqstats"
	"github.com/proxera/backend/internal/settings"
)

// AdminGetStats returns platform-wide statistics.
func AdminGetStats(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var userCount, domainCount, hostCount int
	var storageBytes int64

	database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount)
	database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM dns_records").Scan(&domainCount)
	database.DB.QueryRow(ctx, "SELECT COUNT(*) FROM hosts").Scan(&hostCount)
	database.DB.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&storageBytes)

	var fs syscall.Statfs_t
	var diskTotal, diskUsed uint64
	if err := syscall.Statfs("/", &fs); err == nil {
		diskTotal = fs.Blocks * uint64(fs.Bsize)
		diskUsed = diskTotal - fs.Bfree*uint64(fs.Bsize)
	}

	return c.JSON(fiber.Map{
		"user_count":         userCount,
		"domain_count":       domainCount,
		"host_count":         hostCount,
		"db_size_bytes":      storageBytes,
		"disk_total_bytes":   diskTotal,
		"disk_used_bytes":    diskUsed,
	})
}

// AdminListUsers returns all users with their agent/host counts.
func AdminListUsers(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT u.id, u.email, u.name, COALESCE(u.role, 'member'), COALESCE(u.email_verified, false),
		       u.created_at, u.updated_at,
		       COALESCE(a.cnt, 0), COALESCE(h.cnt, 0)
		FROM users u
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM agents GROUP BY user_id) a ON a.user_id = u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM hosts GROUP BY user_id) h ON h.user_id = u.id
		ORDER BY u.created_at DESC
	`

	rows, err := database.DB.Query(ctx, query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch users",
		})
	}
	defer rows.Close()

	type adminUser struct {
		ID            int    `json:"id"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		Role          string `json:"role"`
		EmailVerified bool   `json:"email_verified"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		AgentCount    int    `json:"agent_count"`
		HostCount     int    `json:"host_count"`
	}

	var users []adminUser
	for rows.Next() {
		var u adminUser
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified,
			&createdAt, &updatedAt, &u.AgentCount, &u.HostCount); err != nil {
			continue
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		u.UpdatedAt = updatedAt.Format(time.RFC3339)
		users = append(users, u)
	}

	if users == nil {
		users = []adminUser{}
	}

	return c.JSON(fiber.Map{
		"users": users,
	})
}

// AdminGetUser returns a single user by ID with agent/host counts.
func AdminGetUser(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT u.id, u.email, u.name, COALESCE(u.role, 'member'), COALESCE(u.email_verified, false),
		       u.created_at, u.updated_at,
		       COALESCE(a.cnt, 0), COALESCE(h.cnt, 0),
		       COALESCE(u.suspended, false), u.suspended_at, COALESCE(u.suspended_reason, '')
		FROM users u
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM agents GROUP BY user_id) a ON a.user_id = u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM hosts GROUP BY user_id) h ON h.user_id = u.id
		WHERE u.id = $1
	`

	var u struct {
		ID              int     `json:"id"`
		Email           string  `json:"email"`
		Name            string  `json:"name"`
		Role            string  `json:"role"`
		EmailVerified   bool    `json:"email_verified"`
		CreatedAt       string  `json:"created_at"`
		UpdatedAt       string  `json:"updated_at"`
		AgentCount      int     `json:"agent_count"`
		HostCount       int     `json:"host_count"`
		Suspended       bool    `json:"suspended"`
		SuspendedAt     *string `json:"suspended_at"`
		SuspendedReason string  `json:"suspended_reason"`
	}
	var createdAt, updatedAt time.Time
	var suspendedAt *time.Time

	err = database.DB.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified,
		&createdAt, &updatedAt, &u.AgentCount, &u.HostCount,
		&u.Suspended, &suspendedAt, &u.SuspendedReason,
	)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	u.CreatedAt = createdAt.Format(time.RFC3339)
	u.UpdatedAt = updatedAt.Format(time.RFC3339)
	if suspendedAt != nil {
		s := suspendedAt.Format(time.RFC3339)
		u.SuspendedAt = &s
	}

	return c.JSON(u)
}

// AdminGetUserStorage returns per-user database footprint estimates.
func AdminGetUserStorage(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	query := `
		SELECT
			u.id, u.name, u.email, COALESCE(u.role, 'member') as role,
			COALESCE(ac.cnt, 0) AS agent_count,
			COALESCE(hc.cnt, 0) AS host_count,
			COALESCE(dc.cnt, 0) AS dns_count,
			COALESCE(cc.cnt, 0) AS cert_count,
			COALESCE(mc.rows, 0) AS metrics_rows,
			COALESCE(mc.total_bytes, 0) AS total_bytes,
			(COALESCE(mc.rows, 0) * 200 + COALESCE(hc.cnt, 0) * 500
			 + COALESCE(dc.cnt, 0) * 300 + COALESCE(cc.cnt, 0) * 2048) AS est_storage
		FROM users u
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM agents GROUP BY user_id) ac ON ac.user_id = u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM hosts GROUP BY user_id) hc ON hc.user_id = u.id
		LEFT JOIN (
			SELECT dp.user_id, COUNT(*) cnt
			FROM dns_records dr
			JOIN dns_providers dp ON dr.dns_provider_id = dp.id
			GROUP BY dp.user_id
		) dc ON dc.user_id = u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM certificates GROUP BY user_id) cc ON cc.user_id = u.id
		LEFT JOIN (
			SELECT a.user_id, COUNT(*) rows, COALESCE(SUM(m.bytes_sent + m.bytes_received), 0) total_bytes
			FROM metrics m
			JOIN agents a ON m.agent_id = a.agent_id
			GROUP BY a.user_id
		) mc ON mc.user_id = u.id
		ORDER BY COALESCE(mc.rows, 0) DESC
	`

	rows, err := database.DB.Query(ctx, query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch storage stats",
		})
	}
	defer rows.Close()

	type userStorage struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		AgentCount  int    `json:"agent_count"`
		HostCount   int    `json:"host_count"`
		DNSCount    int    `json:"dns_count"`
		CertCount   int    `json:"cert_count"`
		MetricsRows int64  `json:"metrics_rows"`
		TotalBytes  int64  `json:"total_bytes"`
		EstStorage  int64  `json:"est_storage"`
	}

	var users []userStorage
	for rows.Next() {
		var u userStorage
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role,
			&u.AgentCount, &u.HostCount, &u.DNSCount, &u.CertCount,
			&u.MetricsRows, &u.TotalBytes, &u.EstStorage); err != nil {
			continue
		}
		users = append(users, u)
	}

	if users == nil {
		users = []userStorage{}
	}

	return c.JSON(fiber.Map{
		"users": users,
	})
}

// AdminUpdateUser updates a user's name, email, role, and email_verified.
func AdminUpdateUser(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var body struct {
		Name          string `json:"name"`
		Email         string `json:"email"`
		Role          string `json:"role"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if body.Role != "admin" && body.Role != "member" && body.Role != "viewer" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid role"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = database.DB.Exec(ctx,
		`UPDATE users SET name=$1, email=$2, role=$3, email_verified=$4, updated_at=NOW() WHERE id=$5`,
		body.Name, body.Email, body.Role, body.EmailVerified, id,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user"})
	}

	return AdminGetUser(c)
}

// AdminGetDashboardHealth returns agent/cert health, recent signups, and traffic sparkline.
func AdminGetDashboardHealth(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var agentTotal, agentOnline int
	database.DB.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE last_seen > NOW() - INTERVAL '90 seconds')
		FROM agents
	`).Scan(&agentTotal, &agentOnline)

	var certTotal, certValid, certExpiring, certExpired int
	database.DB.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE expires_at > NOW() + INTERVAL '30 days'),
		       COUNT(*) FILTER (WHERE expires_at > NOW() AND expires_at <= NOW() + INTERVAL '30 days'),
		       COUNT(*) FILTER (WHERE expires_at <= NOW())
		FROM certificates
	`).Scan(&certTotal, &certValid, &certExpiring, &certExpired)

	type recentUser struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		CreatedAt string `json:"created_at"`
	}
	var recentUsers []recentUser
	signupRows, err := database.DB.Query(ctx,
		`SELECT id, name, email, COALESCE(role, 'member') as role, created_at FROM users ORDER BY created_at DESC LIMIT 10`)
	if err == nil {
		defer signupRows.Close()
		for signupRows.Next() {
			var u recentUser
			var createdAt time.Time
			if err := signupRows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &createdAt); err != nil {
				continue
			}
			u.CreatedAt = createdAt.Format(time.RFC3339)
			recentUsers = append(recentUsers, u)
		}
	}
	if recentUsers == nil {
		recentUsers = []recentUser{}
	}

	type sparkPoint struct {
		Time     string `json:"time"`
		Requests int64  `json:"requests"`
	}
	var sparkline []sparkPoint
	sparkRows, err := database.DB.Query(ctx, `
		SELECT time_bucket('1 hour', time) AS bucket, COALESCE(SUM(request_count), 0)
		FROM metrics
		WHERE time > NOW() - INTERVAL '24 hours'
		GROUP BY bucket
		ORDER BY bucket
	`)
	if err == nil {
		defer sparkRows.Close()
		for sparkRows.Next() {
			var p sparkPoint
			var t time.Time
			if err := sparkRows.Scan(&t, &p.Requests); err != nil {
				continue
			}
			p.Time = t.Format(time.RFC3339)
			sparkline = append(sparkline, p)
		}
	}
	if sparkline == nil {
		sparkline = []sparkPoint{}
	}

	return c.JSON(fiber.Map{
		"agents": fiber.Map{
			"total":   agentTotal,
			"online":  agentOnline,
			"offline": agentTotal - agentOnline,
		},
		"certificates": fiber.Map{
			"total":    certTotal,
			"valid":    certValid,
			"expiring": certExpiring,
			"expired":  certExpired,
		},
		"recent_signups": recentUsers,
		"sparkline":      sparkline,
	})
}

// AdminListAgents returns all agents across all users with computed online/offline status.
func AdminListAgents(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT a.id, a.agent_id, a.name,
		       COALESCE(a.version, ''), COALESCE(a.os, ''), COALESCE(a.arch, ''),
		       a.last_seen, COALESCE(a.wan_ip, ''),
		       COALESCE(a.nginx_version, ''), a.crowdsec_installed, a.host_count,
		       u.id, u.name, u.email
		FROM agents a
		JOIN users u ON a.user_id = u.id
		ORDER BY a.last_seen DESC
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch agents"})
	}
	defer rows.Close()

	type adminAgent struct {
		ID                int    `json:"id"`
		AgentID           string `json:"agent_id"`
		Name              string `json:"name"`
		Status            string `json:"status"`
		Version           string `json:"version"`
		OS                string `json:"os"`
		Arch              string `json:"arch"`
		LastSeen          string `json:"last_seen"`
		WanIP             string `json:"wan_ip"`
		NginxVersion      string `json:"nginx_version"`
		CrowdSecInstalled bool   `json:"crowdsec_installed"`
		HostCount         int    `json:"host_count"`
		UserID            int    `json:"user_id"`
		UserName          string `json:"user_name"`
		UserEmail         string `json:"user_email"`
	}

	var agents []adminAgent
	for rows.Next() {
		var a adminAgent
		var lastSeen time.Time
		if err := rows.Scan(&a.ID, &a.AgentID, &a.Name,
			&a.Version, &a.OS, &a.Arch,
			&lastSeen, &a.WanIP,
			&a.NginxVersion, &a.CrowdSecInstalled, &a.HostCount,
			&a.UserID, &a.UserName, &a.UserEmail); err != nil {
			continue
		}
		a.LastSeen = lastSeen.Format(time.RFC3339)
		if time.Since(lastSeen) > 90*time.Second {
			a.Status = "offline"
		} else {
			a.Status = "online"
		}
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []adminAgent{}
	}

	return c.JSON(fiber.Map{"agents": agents})
}

// AdminListCertificates returns all certificates across all users.
func AdminListCertificates(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT c.id, c.domain, COALESCE(c.san, ''), c.status,
		       c.issued_at, c.expires_at, c.created_at,
		       u.id, u.name, u.email
		FROM certificates c
		JOIN users u ON c.user_id = u.id
		ORDER BY c.expires_at ASC NULLS LAST
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch certificates"})
	}
	defer rows.Close()

	type adminCert struct {
		ID        int    `json:"id"`
		Domain    string `json:"domain"`
		SAN       string `json:"san"`
		Status    string `json:"status"`
		IssuedAt  string `json:"issued_at"`
		ExpiresAt string `json:"expires_at"`
		CreatedAt string `json:"created_at"`
		UserID    int    `json:"user_id"`
		UserName  string `json:"user_name"`
		UserEmail string `json:"user_email"`
	}

	var certs []adminCert
	for rows.Next() {
		var cert adminCert
		var issuedAt, expiresAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&cert.ID, &cert.Domain, &cert.SAN, &cert.Status,
			&issuedAt, &expiresAt, &createdAt,
			&cert.UserID, &cert.UserName, &cert.UserEmail); err != nil {
			continue
		}
		if issuedAt != nil {
			cert.IssuedAt = issuedAt.Format(time.RFC3339)
		}
		if expiresAt != nil {
			cert.ExpiresAt = expiresAt.Format(time.RFC3339)
		}
		cert.CreatedAt = createdAt.Format(time.RFC3339)
		certs = append(certs, cert)
	}
	if certs == nil {
		certs = []adminCert{}
	}

	return c.JSON(fiber.Map{"certificates": certs})
}

// AdminListHosts returns all hosts across all users.
func AdminListHosts(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT h.id, h.domain, h.upstream_url, h.ssl, h.websocket, h.updated_at,
		       u.id, u.name,
		       COALESCE(a.name, ''), COALESCE(a.agent_id, '')
		FROM hosts h
		JOIN users u ON h.user_id = u.id
		LEFT JOIN agents a ON h.agent_id = a.id
		ORDER BY h.updated_at DESC
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch hosts"})
	}
	defer rows.Close()

	type adminHost struct {
		ID          int    `json:"id"`
		Domain      string `json:"domain"`
		UpstreamURL string `json:"upstream_url"`
		SSL         bool   `json:"ssl"`
		WebSocket   bool   `json:"websocket"`
		UpdatedAt   string `json:"updated_at"`
		UserID      int    `json:"user_id"`
		UserName    string `json:"user_name"`
		AgentName   string `json:"agent_name"`
		AgentID     string `json:"agent_id"`
	}

	var hosts []adminHost
	for rows.Next() {
		var h adminHost
		var updatedAt time.Time
		if err := rows.Scan(&h.ID, &h.Domain, &h.UpstreamURL, &h.SSL, &h.WebSocket, &updatedAt,
			&h.UserID, &h.UserName,
			&h.AgentName, &h.AgentID); err != nil {
			continue
		}
		h.UpdatedAt = updatedAt.Format(time.RFC3339)
		hosts = append(hosts, h)
	}
	if hosts == nil {
		hosts = []adminHost{}
	}

	return c.JSON(fiber.Map{"hosts": hosts})
}

// AdminGetPlatformMetrics returns aggregate traffic stats and top domains.
func AdminGetPlatformMetrics(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rangeParam := c.Query("range", "24h")

	// All interpolated values are from this controlled switch — no user input in SQL.
	var duration, table, timeCol string
	switch rangeParam {
	case "1h":
		duration, table, timeCol = "1 hour", "metrics", "time"
	case "6h":
		duration, table, timeCol = "6 hours", "metrics", "time"
	case "24h":
		duration, table, timeCol = "24 hours", "metrics", "time"
	case "7d":
		duration, table, timeCol = "7 days", "metrics", "time"
	case "30d":
		duration, table, timeCol = "30 days", "metrics_15min", "bucket"
	default:
		duration, table, timeCol = "24 hours", "metrics", "time"
		rangeParam = "24h"
	}

	var totalRequests, bytesSent, bytesRecv, s2xx, s3xx, s4xx, s5xx int64
	database.DB.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(bytes_sent), 0),
		       COALESCE(SUM(bytes_received), 0),
		       COALESCE(SUM(status_2xx), 0), COALESCE(SUM(status_3xx), 0),
		       COALESCE(SUM(status_4xx), 0), COALESCE(SUM(status_5xx), 0)
		FROM %s WHERE %s > NOW() - INTERVAL '%s'
	`, table, timeCol, duration)).Scan(&totalRequests, &bytesSent, &bytesRecv, &s2xx, &s3xx, &s4xx, &s5xx)

	var errorRate float64
	if totalRequests > 0 {
		errorRate = float64(s4xx+s5xx) / float64(totalRequests) * 100
	}

	type domainRow struct {
		Domain   string `json:"domain"`
		Requests int64  `json:"requests"`
		Bytes    int64  `json:"bytes"`
	}
	var domains []domainRow
	domainRows, err := database.DB.Query(ctx, fmt.Sprintf(`
		SELECT domain, SUM(request_count) AS reqs, SUM(bytes_sent + bytes_received) AS bytes
		FROM %s WHERE %s > NOW() - INTERVAL '%s'
		GROUP BY domain ORDER BY reqs DESC LIMIT 20
	`, table, timeCol, duration))
	if err == nil {
		defer domainRows.Close()
		for domainRows.Next() {
			var d domainRow
			if err := domainRows.Scan(&d.Domain, &d.Requests, &d.Bytes); err != nil {
				continue
			}
			domains = append(domains, d)
		}
	}
	if domains == nil {
		domains = []domainRow{}
	}

	return c.JSON(fiber.Map{
		"range": rangeParam,
		"summary": fiber.Map{
			"total_requests": totalRequests,
			"bytes_sent":     bytesSent,
			"bytes_received": bytesRecv,
			"status_2xx":     s2xx,
			"status_3xx":     s3xx,
			"status_4xx":     s4xx,
			"status_5xx":     s5xx,
			"error_rate":     errorRate,
		},
		"top_domains": domains,
	})
}

// AdminListAlerts returns platform-wide recent alerts across all users.
func AdminListAlerts(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit < 1 || limit > 500 {
		limit = 100
	}

	rows, err := database.DB.Query(ctx, `
		SELECT ah.id, ah.user_id, ah.rule_id, ah.alert_type, ah.severity,
		       ah.title, ah.message, ah.metadata, ah.resolved, ah.resolved_at, ah.created_at,
		       u.name, u.email
		FROM alert_history ah
		JOIN users u ON ah.user_id = u.id
		ORDER BY ah.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch alerts"})
	}
	defer rows.Close()

	type adminAlert struct {
		ID         int64           `json:"id"`
		UserID     int             `json:"user_id"`
		RuleID     *int            `json:"rule_id"`
		AlertType  string          `json:"alert_type"`
		Severity   string          `json:"severity"`
		Title      string          `json:"title"`
		Message    string          `json:"message"`
		Metadata   json.RawMessage `json:"metadata"`
		Resolved   bool            `json:"resolved"`
		ResolvedAt *string         `json:"resolved_at"`
		CreatedAt  string          `json:"created_at"`
		UserName   string          `json:"user_name"`
		UserEmail  string          `json:"user_email"`
	}

	var alerts []adminAlert
	for rows.Next() {
		var a adminAlert
		var resolvedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &a.UserID, &a.RuleID, &a.AlertType, &a.Severity,
			&a.Title, &a.Message, &a.Metadata, &a.Resolved, &resolvedAt, &createdAt,
			&a.UserName, &a.UserEmail); err != nil {
			continue
		}
		a.CreatedAt = createdAt.Format(time.RFC3339)
		if resolvedAt != nil {
			s := resolvedAt.Format(time.RFC3339)
			a.ResolvedAt = &s
		}
		alerts = append(alerts, a)
	}
	if alerts == nil {
		alerts = []adminAlert{}
	}

	var total int
	database.DB.QueryRow(ctx, `SELECT COUNT(*) FROM alert_history`).Scan(&total)

	return c.JSON(fiber.Map{
		"alerts": alerts,
		"total":  total,
	})
}

// AdminAlertStats returns alert KPI data for the admin dashboard.
func AdminAlertStats(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var activeAlerts, triggers24h, triggers7d, usersWithActive int

	database.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM alert_history WHERE resolved = false`).Scan(&activeAlerts)
	database.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM alert_history WHERE created_at > NOW() - INTERVAL '24 hours'`).Scan(&triggers24h)
	database.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM alert_history WHERE created_at > NOW() - INTERVAL '7 days'`).Scan(&triggers7d)
	database.DB.QueryRow(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM alert_history WHERE resolved = false`).Scan(&usersWithActive)

	return c.JSON(fiber.Map{
		"active_alerts":            activeAlerts,
		"triggers_24h":             triggers24h,
		"triggers_7d":              triggers7d,
		"users_with_active_alerts": usersWithActive,
	})
}

// AdminSuspendUser suspends a user account.
// POST /api/admin/users/:id/suspend
// Body: { "reason": "..." }
func AdminSuspendUser(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var req struct {
		Reason string `json:"reason"`
	}
	c.BodyParser(&req)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := database.DB.Exec(ctx,
		`UPDATE users SET suspended = true, suspended_at = NOW(), suspended_reason = $1 WHERE id = $2`,
		req.Reason, id,
	)
	if err != nil || tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	return AdminGetUser(c)
}

// AdminUnsuspendUser lifts a user account suspension.
// POST /api/admin/users/:id/unsuspend
func AdminUnsuspendUser(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := database.DB.Exec(ctx,
		`UPDATE users SET suspended = false, suspended_at = NULL, suspended_reason = '' WHERE id = $1`,
		id,
	)
	if err != nil || tag.RowsAffected() == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	return AdminGetUser(c)
}

// AdminForcePasswordReset generates a password reset token and emails it to the user.
// POST /api/admin/users/:id/password-reset
func AdminForcePasswordReset(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var userEmail, userName string
	err = database.DB.QueryRow(ctx,
		`SELECT email, name FROM users WHERE id = $1`, id,
	).Scan(&userEmail, &userName)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	// Generate secure random token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}
	token := hex.EncodeToString(b)

	// Invalidate any existing tokens for this user and insert the new one
	database.DB.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_id = $1`, id)
	_, err = database.DB.Exec(ctx,
		`INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES ($1, $2, NOW() + INTERVAL '1 hour')`,
		id, token,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to store reset token"})
	}

	siteURL := settings.Get("PUBLIC_SITE_URL", "http://localhost:8080")
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", siteURL, token)

	if err := email.SendPasswordResetEmail(userEmail, userName, resetURL); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send reset email: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Password reset email sent"})
}

// AdminGetSystemHealth returns backend runtime health: uptime, memory, goroutines, request stats.
// GET /api/admin/stats/system
func AdminGetSystemHealth(c *fiber.Ctx) error {
	// Runtime memory stats
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	// Request stats from ring buffer — last 1 and 5 minutes
	req1, err1, avg1, p95_1 := reqstats.GlobalRequestTracker.Stats(1)
	req5, err5, avg5, p95_5 := reqstats.GlobalRequestTracker.Stats(5)

	// Error rates
	var errRate1, errRate5 float64
	if req1 > 0 {
		errRate1 = float64(err1) / float64(req1) * 100
	}
	if req5 > 0 {
		errRate5 = float64(err5) / float64(req5) * 100
	}

	// Disk stats
	var fs syscall.Statfs_t
	var diskTotal, diskUsed uint64
	if err := syscall.Statfs("/", &fs); err == nil {
		diskTotal = fs.Blocks * uint64(fs.Bsize)
		diskUsed = diskTotal - fs.Bfree*uint64(fs.Bsize)
	}

	uptimeSecs := int64(time.Since(reqstats.GlobalRequestTracker.StartedAt).Seconds())

	// DB pool stats
	poolStats := database.DB.Stat()

	return c.JSON(fiber.Map{
		"uptime_seconds": uptimeSecs,
		"goroutines":     runtime.NumGoroutine(),
		"memory": fiber.Map{
			"alloc_mb":       float64(mem.Alloc) / 1024 / 1024,
			"sys_mb":         float64(mem.Sys) / 1024 / 1024,
			"heap_alloc_mb":  float64(mem.HeapAlloc) / 1024 / 1024,
			"heap_in_use_mb": float64(mem.HeapInuse) / 1024 / 1024,
			"gc_runs":        mem.NumGC,
		},
		"disk": fiber.Map{
			"total_bytes": diskTotal,
			"used_bytes":  diskUsed,
		},
		"db_pool": fiber.Map{
			"total":    poolStats.TotalConns(),
			"idle":     poolStats.IdleConns(),
			"acquired": poolStats.AcquiredConns(),
		},
		"requests": fiber.Map{
			"last_1m": fiber.Map{
				"count":      req1,
				"errors":     err1,
				"error_rate": errRate1,
				"avg_ms":     avg1,
				"p95_ms":     p95_1,
			},
			"last_5m": fiber.Map{
				"count":      req5,
				"errors":     err5,
				"error_rate": errRate5,
				"avg_ms":     avg5,
				"p95_ms":     p95_5,
			},
		},
	})
}
