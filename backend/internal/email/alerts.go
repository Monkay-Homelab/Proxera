package email

import (
	"fmt"
	"strings"
	"time"

	"github.com/proxera/backend/internal/models"
	"github.com/proxera/backend/internal/settings"
)

// SendAlertEmail sends an alert notification email.
func SendAlertEmail(to string, alert models.AlertPayload) error {
	emoji := severityEmoji(alert.Severity)
	subject := fmt.Sprintf("[Proxera] %s %s", emoji, alert.Title)

	panelURL := settings.Get("PUBLIC_SITE_URL", "http://localhost:8080")

	body := formatAlertBody(alert, panelURL)
	return sendMail(to, subject, body)
}

func severityEmoji(severity string) string {
	switch severity {
	case "critical":
		return "\xF0\x9F\x9A\xA8" // 🚨
	case "warning":
		return "\xE2\x9A\xA0\xEF\xB8\x8F" // ⚠️
	default:
		return "\xE2\x84\xB9\xEF\xB8\x8F" // ℹ️
	}
}

func formatAlertBody(alert models.AlertPayload, panelURL string) string {
	var sb strings.Builder

	sb.WriteString(alert.Message)
	sb.WriteString("\n\n---\n")
	sb.WriteString(fmt.Sprintf("Alert Type: %s\n", alert.AlertType))
	sb.WriteString(fmt.Sprintf("Severity: %s\n", alert.Severity))
	sb.WriteString(fmt.Sprintf("Time: %s\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("\nManage your alerts: %s/alerts\n", panelURL))
	sb.WriteString("\n— Proxera")

	return sb.String()
}

// BuildAgentOfflineAlert creates an alert payload for an agent going offline.
func BuildAgentOfflineAlert(agentName, agentID, ipAddress string, lastSeen time.Time) models.AlertPayload {
	ago := time.Since(lastSeen).Truncate(time.Second)
	message := fmt.Sprintf(`Your agent "%s" has gone offline.

Last seen: %s ago (%s)
Agent ID: %s`,
		agentName,
		ago,
		lastSeen.UTC().Format("2006-01-02 15:04:05 UTC"),
		agentID,
	)
	if ipAddress != "" {
		message += fmt.Sprintf("\nIP Address: %s", ipAddress)
	}
	message += "\n\nThis usually means the server is unreachable or the agent process has stopped.\nCheck your server status and restart the agent if needed."

	return models.AlertPayload{
		AlertType: "agent_offline",
		Severity:  "critical",
		Title:     fmt.Sprintf("Agent '%s' went offline", agentName),
		Message:   message,
		Metadata: map[string]any{
			"agent_id":   agentID,
			"agent_name": agentName,
			"last_seen":  lastSeen.UTC().Format(time.RFC3339),
		},
	}
}

// BuildAgentOnlineAlert creates an alert payload for an agent coming back online.
func BuildAgentOnlineAlert(agentName, agentID string) models.AlertPayload {
	return models.AlertPayload{
		AlertType: "agent_offline",
		Severity:  "info",
		Title:     fmt.Sprintf("Agent '%s' is back online", agentName),
		Message:   fmt.Sprintf("Your agent \"%s\" has reconnected and is back online.", agentName),
		Metadata: map[string]any{
			"agent_id":   agentID,
			"agent_name": agentName,
		},
	}
}

// BuildCertExpiryAlert creates an alert payload for an expiring certificate.
func BuildCertExpiryAlert(domain string, certID int, expiresAt time.Time, daysRemaining int) models.AlertPayload {
	severity := "warning"
	if daysRemaining <= 1 {
		severity = "critical"
	}

	message := fmt.Sprintf(`Your SSL certificate for %s expires in %d day(s).

Domain: %s
Expires: %s
Status: Proxera will attempt auto-renewal.

If auto-renewal fails, you'll receive a follow-up alert.`,
		domain, daysRemaining, domain,
		expiresAt.UTC().Format("2006-01-02 15:04:05 UTC"),
	)

	return models.AlertPayload{
		AlertType: "cert_expiry",
		Severity:  severity,
		Title:     fmt.Sprintf("Certificate for %s expires in %d day(s)", domain, daysRemaining),
		Message:   message,
		Metadata: map[string]any{
			"cert_id":        certID,
			"domain":         domain,
			"expires_at":     expiresAt.UTC().Format(time.RFC3339),
			"days_remaining": daysRemaining,
		},
	}
}

// BuildCertRenewalFailedAlert creates an alert payload for a failed certificate renewal.
func BuildCertRenewalFailedAlert(domain string, certID int, expiresAt time.Time, reason string) models.AlertPayload {
	expiresStr := "N/A"
	if !expiresAt.IsZero() {
		expiresStr = expiresAt.UTC().Format("2006-01-02 15:04:05 UTC")
	}

	message := fmt.Sprintf(`Auto-renewal failed for your certificate on %s.

Domain: %s
Error: %s
Expires: %s

Action required:
1. Check your DNS provider API key in the panel
2. Re-issue the certificate manually if needed`,
		domain, domain, reason, expiresStr,
	)

	return models.AlertPayload{
		AlertType: "cert_renewal_failed",
		Severity:  "critical",
		Title:     fmt.Sprintf("Certificate renewal failed for %s", domain),
		Message:   message,
		Metadata: map[string]any{
			"cert_id": certID,
			"domain":  domain,
			"reason":  reason,
		},
	}
}

// BuildHighLatencyAlert creates an alert payload for high P95 latency.
func BuildHighLatencyAlert(domain string, p95Ms, thresholdMs float64, windowMinutes int) models.AlertPayload {
	severity := "warning"
	if p95Ms > thresholdMs*2 {
		severity = "critical"
	}

	message := fmt.Sprintf(`High latency detected on %s.

P95 Latency: %.0f ms (threshold: %.0f ms)
Window: Last %d minute(s)

Check your upstream server performance and recent deployments.`,
		domain, p95Ms, thresholdMs, windowMinutes,
	)

	return models.AlertPayload{
		AlertType: "high_latency",
		Severity:  severity,
		Title:     fmt.Sprintf("High latency on %s (%.0f ms)", domain, p95Ms),
		Message:   message,
		Metadata: map[string]any{
			"domain":         domain,
			"latency_p95_ms": p95Ms,
			"threshold_ms":   thresholdMs,
			"window_minutes": windowMinutes,
		},
	}
}

// BuildTrafficSpikeAlert creates an alert payload for traffic spikes.
func BuildTrafficSpikeAlert(domain string, currentRPS, baselineRPS, spikePct float64) models.AlertPayload {
	severity := "warning"
	if spikePct > 500 {
		severity = "critical"
	}

	message := fmt.Sprintf(`Traffic spike detected on %s.

Current: %.1f req/s (baseline: %.1f req/s)
Spike: %.0f%% above normal

This may indicate a DDoS attack, viral traffic, or a bot crawling your site.`,
		domain, currentRPS, baselineRPS, spikePct,
	)

	return models.AlertPayload{
		AlertType: "traffic_spike",
		Severity:  severity,
		Title:     fmt.Sprintf("Traffic spike on %s (%.0f%% above normal)", domain, spikePct),
		Message:   message,
		Metadata: map[string]any{
			"domain":       domain,
			"current_rps":  currentRPS,
			"baseline_rps": baselineRPS,
			"spike_pct":    spikePct,
		},
	}
}

// BuildHostDownAlert creates an alert payload for a host returning 100% errors.
func BuildHostDownAlert(domain string, status5xx, totalRequests int64, windowMinutes int) models.AlertPayload {
	message := fmt.Sprintf(`Host appears down: %s is returning only errors.

5xx Responses: %d of %d total requests
Window: Last %d minute(s)

Check your upstream server — it may be crashed, misconfigured, or unreachable.`,
		domain, status5xx, totalRequests, windowMinutes,
	)

	return models.AlertPayload{
		AlertType: "host_down",
		Severity:  "critical",
		Title:     fmt.Sprintf("Host down: %s (100%% errors)", domain),
		Message:   message,
		Metadata: map[string]any{
			"domain":         domain,
			"status_5xx":     status5xx,
			"total_requests": totalRequests,
			"window_minutes": windowMinutes,
		},
	}
}

// BuildBandwidthThresholdAlert creates an alert payload for bandwidth threshold exceeded.
func BuildBandwidthThresholdAlert(domain string, bytesUsed, thresholdBytes int64, periodHours int) models.AlertPayload {
	severity := "warning"
	if bytesUsed > thresholdBytes*2 {
		severity = "critical"
	}

	usedGB := float64(bytesUsed) / (1024 * 1024 * 1024)
	threshGB := float64(thresholdBytes) / (1024 * 1024 * 1024)

	message := fmt.Sprintf(`Bandwidth threshold exceeded on %s.

Used: %.2f GB (threshold: %.2f GB)
Period: Last %d hour(s)

Consider enabling caching, optimizing assets, or upgrading your plan.`,
		domain, usedGB, threshGB, periodHours,
	)

	return models.AlertPayload{
		AlertType: "bandwidth_threshold",
		Severity:  severity,
		Title:     fmt.Sprintf("Bandwidth threshold on %s (%.2f GB)", domain, usedGB),
		Message:   message,
		Metadata: map[string]any{
			"domain":          domain,
			"bytes_used":      bytesUsed,
			"threshold_bytes": thresholdBytes,
			"period_hours":    periodHours,
		},
	}
}

// BuildErrorRateAlert creates an alert payload for high server error (5xx) rates.
func BuildErrorRateAlert(domain string, errorRate, threshold float64, windowMinutes int, serverErrors, totalRequests int64) models.AlertPayload {
	severity := "warning"
	if errorRate > threshold*2 {
		severity = "critical"
	}

	message := fmt.Sprintf(`High server error rate detected on %s.

Error Rate: %.1f%% (threshold: %.1f%%)
Window: Last %d minute(s)
Server Errors (5xx): %d of %d total requests

Check your upstream server health and recent deployments.`,
		domain, errorRate, threshold, windowMinutes, serverErrors, totalRequests,
	)

	return models.AlertPayload{
		AlertType: "error_rate",
		Severity:  severity,
		Title:     fmt.Sprintf("High error rate on %s (%.1f%%)", domain, errorRate),
		Message:   message,
		Metadata: map[string]any{
			"domain":         domain,
			"error_rate":     errorRate,
			"threshold":      threshold,
			"window_minutes": windowMinutes,
			"server_errors":  serverErrors,
			"total_requests": totalRequests,
		},
	}
}
