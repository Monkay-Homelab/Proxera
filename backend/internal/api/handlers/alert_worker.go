package handlers

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"time"

	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/email"
	"github.com/proxera/backend/internal/models"
	"github.com/proxera/backend/internal/notifications"
)

// StartAlertWorker runs periodic alert checks every 5 minutes.
func StartAlertWorker() {
	// Wait briefly for DB connections to settle
	time.Sleep(10 * time.Second)

	runAlertChecks()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		runAlertChecks()
	}
}

func runAlertChecks() {
	checkStaleAgents()
	checkCertExpiryAlerts()
}

// checkStaleAgents detects agents that went offline without a clean disconnect.
func checkStaleAgents() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT a.agent_id, a.user_id, a.name, a.last_seen, COALESCE(a.wan_ip, '')
		FROM agents a
		WHERE a.last_seen < NOW() - INTERVAL '90 seconds'
		  AND a.status != 'offline'
	`)
	if err != nil {
		log.Printf("[AlertWorker] Failed to query stale agents: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var agentID string
		var userID int
		var agentName, wanIP string
		var lastSeen time.Time
		if err := rows.Scan(&agentID, &userID, &agentName, &lastSeen, &wanIP); err != nil {
			continue
		}

		// Mark agent offline
		UpdateAgentStatus(agentID, "offline")

		// Trigger alert
		triggerAgentOfflineAlert(agentID, userID, agentName, wanIP, lastSeen)
	}
}

// triggerAgentOfflineAlert checks user's alert rules and dispatches agent offline alerts.
func triggerAgentOfflineAlert(agentID string, userID int, agentName, wanIP string, lastSeen time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, config, cooldown_minutes
		FROM alert_rules
		WHERE user_id = $1 AND alert_type = 'agent_offline' AND enabled = true
	`, userID)
	if err != nil {
		log.Printf("[AlertWorker] Failed to query agent_offline rules for user %d: %v", userID, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, cooldownMinutes int
		var configRaw json.RawMessage
		if err := rows.Scan(&ruleID, &configRaw, &cooldownMinutes); err != nil {
			continue
		}

		// Parse config to check agent_ids
		var config struct {
			AgentIDs []string `json:"agent_ids"`
		}
		json.Unmarshal(configRaw, &config)

		matched := false
		for _, id := range config.AgentIDs {
			if id == "all" || id == agentID {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		alert := email.BuildAgentOfflineAlert(agentName, agentID, wanIP, lastSeen)
		notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
	}
}

// triggerAgentOnlineResolution resolves open agent_offline alerts when an agent reconnects.
func triggerAgentOnlineResolution(agentID string, userID int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	notifications.Resolve(ctx, userID, "agent_offline", "agent_id", agentID)
}

// checkCertExpiryAlerts scans for certificates approaching expiration.
func checkCertExpiryAlerts() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT c.id, c.user_id, c.domain, c.expires_at
		FROM certificates c
		WHERE c.expires_at IS NOT NULL
		  AND c.certificate_pem IS NOT NULL
		  AND c.status != 'error'
		ORDER BY c.user_id
	`)
	if err != nil {
		log.Printf("[AlertWorker] Failed to query certs for expiry alerts: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var certID, userID int
		var domain string
		var expiresAt time.Time
		if err := rows.Scan(&certID, &userID, &domain, &expiresAt); err != nil {
			continue
		}

		daysRemaining := int(time.Until(expiresAt).Hours() / 24)

		// Check user's cert_expiry rules
		ruleRows, err := database.DB.Query(ctx, `
			SELECT id, config, cooldown_minutes
			FROM alert_rules
			WHERE user_id = $1 AND alert_type = 'cert_expiry' AND enabled = true
		`, userID)
		if err != nil {
			continue
		}

		for ruleRows.Next() {
			var ruleID, cooldownMinutes int
			var configRaw json.RawMessage
			if err := ruleRows.Scan(&ruleID, &configRaw, &cooldownMinutes); err != nil {
				continue
			}

			var config struct {
				WarnDays []int `json:"warn_days"`
			}
			json.Unmarshal(configRaw, &config)
			if len(config.WarnDays) == 0 {
				config.WarnDays = []int{30, 7, 1}
			}
			// Sort ascending so the tightest matching threshold fires first
			sort.Ints(config.WarnDays)

			for _, threshold := range config.WarnDays {
				if daysRemaining <= threshold {
					alert := email.BuildCertExpiryAlert(domain, certID, expiresAt, daysRemaining)
					notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
					break // Only alert at the most urgent threshold
				}
			}
		}
		ruleRows.Close()
	}
}

// triggerCertRenewalFailedAlert dispatches alerts when certificate renewal fails.
func triggerCertRenewalFailedAlert(userID int, certID int, domain string, expiresAt time.Time, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, cooldown_minutes
		FROM alert_rules
		WHERE user_id = $1 AND alert_type = 'cert_renewal_failed' AND enabled = true
	`, userID)
	if err != nil {
		log.Printf("[AlertWorker] Failed to query cert_renewal_failed rules for user %d: %v", userID, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, cooldownMinutes int
		if err := rows.Scan(&ruleID, &cooldownMinutes); err != nil {
			continue
		}

		alert := email.BuildCertRenewalFailedAlert(domain, certID, expiresAt, reason)
		notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
	}
}

// evaluateHighLatencyAlerts checks metrics for high P95 latency.
func evaluateHighLatencyAlerts(agentID string, userID int, buckets []models.IncomingMetricsBucket) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, config, cooldown_minutes
		FROM alert_rules
		WHERE user_id = $1 AND alert_type = 'high_latency' AND enabled = true
	`, userID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, cooldownMinutes int
		var configRaw json.RawMessage
		if err := rows.Scan(&ruleID, &configRaw, &cooldownMinutes); err != nil {
			continue
		}

		var config struct {
			ThresholdMs   float64  `json:"threshold_ms"`
			Domains       []string `json:"domains"`
			WindowMinutes int      `json:"window_minutes"`
		}
		json.Unmarshal(configRaw, &config)
		if config.ThresholdMs <= 0 {
			config.ThresholdMs = 500
		}
		if config.WindowMinutes <= 0 {
			config.WindowMinutes = 1
		}

		// Collect per-domain max P95 from current batch
		type domainLatency struct {
			maxP95 float64
			count  int
		}
		domainStats := make(map[string]*domainLatency)
		for _, b := range buckets {
			if b.RequestCount == 0 {
				continue
			}

			matchDomain := false
			for _, d := range config.Domains {
				if d == "all" || d == b.Domain {
					matchDomain = true
					break
				}
			}
			if !matchDomain && len(config.Domains) > 0 {
				continue
			}

			stats, ok := domainStats[b.Domain]
			if !ok {
				stats = &domainLatency{}
				domainStats[b.Domain] = stats
			}
			if b.LatencyP95Ms > stats.maxP95 {
				stats.maxP95 = b.LatencyP95Ms
			}
			stats.count++
		}

		for domain, stats := range domainStats {
			p95 := stats.maxP95

			// For window > 1 minute, query DB for average P95
			if config.WindowMinutes > 1 {
				var dbP95 float64
				database.DB.QueryRow(ctx, `
					SELECT COALESCE(AVG(latency_p95_ms), 0)
					FROM metrics
					WHERE agent_id = $1 AND domain = $2 AND time > NOW() - ($3 || ' minutes')::interval
				`, agentID, domain, config.WindowMinutes).Scan(&dbP95)
				if dbP95 > 0 {
					p95 = dbP95
				}
			}

			if p95 > config.ThresholdMs {
				alert := email.BuildHighLatencyAlert(domain, p95, config.ThresholdMs, config.WindowMinutes)
				notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
			} else {
				notifications.Resolve(ctx, userID, "high_latency", "domain", domain)
			}
		}
	}
}

// evaluateTrafficSpikeAlerts checks metrics for traffic spikes.
func evaluateTrafficSpikeAlerts(agentID string, userID int, buckets []models.IncomingMetricsBucket) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, config, cooldown_minutes
		FROM alert_rules
		WHERE user_id = $1 AND alert_type = 'traffic_spike' AND enabled = true
	`, userID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, cooldownMinutes int
		var configRaw json.RawMessage
		if err := rows.Scan(&ruleID, &configRaw, &cooldownMinutes); err != nil {
			continue
		}

		var config struct {
			Multiplier      float64  `json:"multiplier"`
			BaselineMinutes int      `json:"baseline_minutes"`
			Domains         []string `json:"domains"`
		}
		json.Unmarshal(configRaw, &config)
		if config.Multiplier <= 0 {
			config.Multiplier = 3.0
		}
		if config.BaselineMinutes <= 0 {
			config.BaselineMinutes = 60
		}

		// Aggregate current request counts per domain
		domainCounts := make(map[string]int64)
		for _, b := range buckets {
			if b.RequestCount == 0 {
				continue
			}
			matchDomain := false
			for _, d := range config.Domains {
				if d == "all" || d == b.Domain {
					matchDomain = true
					break
				}
			}
			if !matchDomain && len(config.Domains) > 0 {
				continue
			}
			domainCounts[b.Domain] += b.RequestCount
		}

		for domain, currentCount := range domainCounts {
			// Query baseline average request count per minute
			var baselineAvg float64
			database.DB.QueryRow(ctx, `
				SELECT COALESCE(AVG(request_count), 0)
				FROM metrics
				WHERE agent_id = $1 AND domain = $2 AND time > NOW() - ($3 || ' minutes')::interval
			`, agentID, domain, config.BaselineMinutes).Scan(&baselineAvg)

			if baselineAvg <= 0 {
				continue // No baseline data yet
			}

			currentRPS := float64(currentCount) / 60.0
			baselineRPS := baselineAvg / 60.0
			spikePct := (float64(currentCount)/baselineAvg - 1) * 100

			if float64(currentCount) > baselineAvg*config.Multiplier {
				alert := email.BuildTrafficSpikeAlert(domain, currentRPS, baselineRPS, spikePct)
				notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
			} else {
				notifications.Resolve(ctx, userID, "traffic_spike", "domain", domain)
			}
		}
	}
}

// evaluateHostDownAlerts checks metrics for hosts returning 100% errors.
func evaluateHostDownAlerts(agentID string, userID int, buckets []models.IncomingMetricsBucket) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, config, cooldown_minutes
		FROM alert_rules
		WHERE user_id = $1 AND alert_type = 'host_down' AND enabled = true
	`, userID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, cooldownMinutes int
		var configRaw json.RawMessage
		if err := rows.Scan(&ruleID, &configRaw, &cooldownMinutes); err != nil {
			continue
		}

		var config struct {
			Domains       []string `json:"domains"`
			WindowMinutes int      `json:"window_minutes"`
		}
		json.Unmarshal(configRaw, &config)
		if config.WindowMinutes <= 0 {
			config.WindowMinutes = 1
		}

		// Aggregate per domain
		type domainStat struct{ errors5xx, total int64 }
		domainStats := make(map[string]*domainStat)
		for _, b := range buckets {
			if b.RequestCount == 0 {
				continue
			}
			matchDomain := false
			for _, d := range config.Domains {
				if d == "all" || d == b.Domain {
					matchDomain = true
					break
				}
			}
			if !matchDomain && len(config.Domains) > 0 {
				continue
			}

			stats, ok := domainStats[b.Domain]
			if !ok {
				stats = &domainStat{}
				domainStats[b.Domain] = stats
			}
			stats.errors5xx += b.Status5xx
			stats.total += b.RequestCount
		}

		for domain, stats := range domainStats {
			errors5xx, total := stats.errors5xx, stats.total

			// For window > 1 minute, add historical data
			if config.WindowMinutes > 1 {
				var db5xx, dbTotal int64
				database.DB.QueryRow(ctx, `
					SELECT COALESCE(SUM(status_5xx), 0), COALESCE(SUM(request_count), 0)
					FROM metrics
					WHERE agent_id = $1 AND domain = $2 AND time > NOW() - ($3 || ' minutes')::interval
				`, agentID, domain, config.WindowMinutes).Scan(&db5xx, &dbTotal)
				errors5xx += db5xx
				total += dbTotal
			}

			// Must have minimum requests to avoid false positives on low-traffic domains
			if total < 5 {
				continue
			}

			// Host is "down" if 100% of requests are 5xx (no 2xx/3xx at all)
			if errors5xx == total {
				alert := email.BuildHostDownAlert(domain, errors5xx, total, config.WindowMinutes)
				notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
			} else {
				notifications.Resolve(ctx, userID, "host_down", "domain", domain)
			}
		}
	}
}

// evaluateBandwidthAlerts checks metrics for bandwidth threshold exceeding.
func evaluateBandwidthAlerts(agentID string, userID int, buckets []models.IncomingMetricsBucket) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, config, cooldown_minutes
		FROM alert_rules
		WHERE user_id = $1 AND alert_type = 'bandwidth_threshold' AND enabled = true
	`, userID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, cooldownMinutes int
		var configRaw json.RawMessage
		if err := rows.Scan(&ruleID, &configRaw, &cooldownMinutes); err != nil {
			continue
		}

		var config struct {
			ThresholdGB float64  `json:"threshold_gb"`
			PeriodHours int      `json:"period_hours"`
			Domains     []string `json:"domains"`
		}
		json.Unmarshal(configRaw, &config)
		if config.ThresholdGB <= 0 {
			config.ThresholdGB = 10.0
		}
		if config.PeriodHours <= 0 {
			config.PeriodHours = 1
		}

		thresholdBytes := int64(config.ThresholdGB * 1024 * 1024 * 1024)

		// Collect domains from current batch
		matchedDomains := make(map[string]bool)
		for _, b := range buckets {
			matchDomain := false
			for _, d := range config.Domains {
				if d == "all" || d == b.Domain {
					matchDomain = true
					break
				}
			}
			if !matchDomain && len(config.Domains) > 0 {
				continue
			}
			matchedDomains[b.Domain] = true
		}

		for domain := range matchedDomains {
			var bytesUsed int64
			database.DB.QueryRow(ctx, `
				SELECT COALESCE(SUM(bytes_sent + bytes_received), 0)
				FROM metrics
				WHERE agent_id = $1 AND domain = $2 AND time > NOW() - ($3 || ' hours')::interval
			`, agentID, domain, config.PeriodHours).Scan(&bytesUsed)

			if bytesUsed > thresholdBytes {
				alert := email.BuildBandwidthThresholdAlert(domain, bytesUsed, thresholdBytes, config.PeriodHours)
				notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
			} else {
				notifications.Resolve(ctx, userID, "bandwidth_threshold", "domain", domain)
			}
		}
	}
}

// evaluateErrorRateAlerts checks metrics for high error rates.
func evaluateErrorRateAlerts(agentID string, userID int, buckets []models.IncomingMetricsBucket) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `
		SELECT id, config, cooldown_minutes
		FROM alert_rules
		WHERE user_id = $1 AND alert_type = 'error_rate' AND enabled = true
	`, userID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, cooldownMinutes int
		var configRaw json.RawMessage
		if err := rows.Scan(&ruleID, &configRaw, &cooldownMinutes); err != nil {
			continue
		}

		var config struct {
			ThresholdPct  float64  `json:"threshold_pct"`
			Domains       []string `json:"domains"`
			WindowMinutes int      `json:"window_minutes"`
			MinRequests   int64    `json:"min_requests"`
		}
		json.Unmarshal(configRaw, &config)
		if config.ThresholdPct <= 0 {
			config.ThresholdPct = 5.0
		}
		if config.WindowMinutes <= 0 {
			config.WindowMinutes = 1
		}
		if config.MinRequests <= 0 {
			config.MinRequests = 10
		}

		// Aggregate metrics per domain from current batch (5xx only — 4xx are client errors)
		type domainStat struct{ errors5xx, total int64 }
		domainStats := make(map[string]*domainStat)
		for _, b := range buckets {
			total := b.RequestCount
			if total == 0 {
				continue
			}

			// Check if this domain matches the rule
			matchDomain := false
			for _, d := range config.Domains {
				if d == "all" || d == b.Domain {
					matchDomain = true
					break
				}
			}
			if !matchDomain && len(config.Domains) > 0 {
				continue
			}

			stats, ok := domainStats[b.Domain]
			if !ok {
				stats = &domainStat{}
				domainStats[b.Domain] = stats
			}
			stats.errors5xx += b.Status5xx
			stats.total += total
		}

		for domain, stats := range domainStats {
			errors5xx, total := stats.errors5xx, stats.total
			if total == 0 {
				continue
			}

			// For window > 1 minute, query DB for historical data
			if config.WindowMinutes > 1 {
				var dbErrors, dbTotal int64
				database.DB.QueryRow(ctx, `
					SELECT COALESCE(SUM(status_5xx), 0), COALESCE(SUM(request_count), 0)
					FROM metrics
					WHERE agent_id = $1 AND domain = $2 AND time > NOW() - ($3 || ' minutes')::interval
				`, agentID, domain, config.WindowMinutes).Scan(&dbErrors, &dbTotal)
				errors5xx += dbErrors
				total += dbTotal
			}

			// Skip low-traffic domains to avoid false positives from bots/scanners
			if total < config.MinRequests {
				continue
			}

			errorRate := float64(errors5xx) / float64(total) * 100
			if errorRate > config.ThresholdPct {
				alert := email.BuildErrorRateAlert(domain, errorRate, config.ThresholdPct,
					config.WindowMinutes, errors5xx, total)
				notifications.Dispatch(ctx, userID, ruleID, cooldownMinutes, alert)
			} else {
				// Error rate below threshold — resolve any open alerts
				notifications.Resolve(ctx, userID, "error_rate", "domain", domain)
			}
		}
	}
}

