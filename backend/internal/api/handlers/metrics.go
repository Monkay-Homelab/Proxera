package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
	"github.com/proxera/backend/internal/models"
)

// InsertMetricsBuckets batch-inserts metric buckets into the TimescaleDB metrics table
func InsertMetricsBuckets(agentID string, buckets []models.IncomingMetricsBucket) error {
	if len(buckets) == 0 {
		return nil
	}

	// Build batch INSERT
	var sb strings.Builder
	sb.WriteString(`INSERT INTO metrics (
		time, agent_id, domain, request_count, bytes_sent, bytes_received,
		status_2xx, status_3xx, status_4xx, status_5xx,
		avg_latency_ms, latency_p50_ms, latency_p95_ms, latency_p99_ms,
		avg_upstream_ms, avg_request_size, avg_response_size,
		cache_hits, cache_misses, unique_ips, connection_count
	) VALUES `)

	if len(buckets) > 10000 {
		return fmt.Errorf("too many metric buckets: %d", len(buckets))
	}
	args := make([]interface{}, 0, len(buckets)*21)
	for i, b := range buckets {
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * 21
		sb.WriteString(fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6,
			base+7, base+8, base+9, base+10, base+11, base+12,
			base+13, base+14, base+15, base+16, base+17,
			base+18, base+19, base+20, base+21,
		))

		ts, err := time.Parse(time.RFC3339, b.Timestamp)
		if err != nil {
			ts, err = time.Parse(time.RFC3339Nano, b.Timestamp)
			if err != nil {
				ts = time.Now().UTC()
			}
		}

		args = append(args,
			ts, agentID, b.Domain, b.RequestCount, b.BytesSent, b.BytesReceived,
			b.Status2xx, b.Status3xx, b.Status4xx, b.Status5xx,
			b.AvgLatencyMs, b.LatencyP50Ms, b.LatencyP95Ms, b.LatencyP99Ms,
			b.AvgUpstreamMs, b.AvgRequestSize, b.AvgResponseSize,
			b.CacheHits, b.CacheMisses, b.UniqueIPs, b.ConnectionCount,
		)
	}

	_, err := database.DB.Exec(context.Background(), sb.String(), args...)
	return err
}

// InsertVisitorIPs batch-inserts visitor IP data from metrics buckets
func InsertVisitorIPs(agentID string, buckets []models.IncomingMetricsBucket) error {
	// Count total IP entries
	total := 0
	for _, b := range buckets {
		total += len(b.IPRequestCounts)
	}
	if total == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO visitor_ips (time, agent_id, domain, ip_address, request_count) VALUES `)

	args := make([]interface{}, 0, total*5)
	idx := 0
	for _, b := range buckets {
		if len(b.IPRequestCounts) == 0 {
			continue
		}

		ts, err := time.Parse(time.RFC3339, b.Timestamp)
		if err != nil {
			ts, err = time.Parse(time.RFC3339Nano, b.Timestamp)
			if err != nil {
				ts = time.Now().UTC()
			}
		}

		for ip, count := range b.IPRequestCounts {
			if idx > 0 {
				sb.WriteString(", ")
			}
			base := idx * 5
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d)",
				base+1, base+2, base+3, base+4, base+5))
			args = append(args, ts, agentID, b.Domain, ip, count)
			idx++
		}
	}

	_, err := database.DB.Exec(context.Background(), sb.String(), args...)
	return err
}

// rangeConfig holds parsed time range parameters for metrics queries.
type rangeConfig struct {
	Duration       time.Duration
	BucketInterval string
	HasTimebound   bool
	Source         string // "raw", "15min", "hourly"
}

// parseRange converts a range query parameter into duration, bucket interval, timebound flag, and source table.
func parseRange(rangeParam string) rangeConfig {
	return defaultRangeConfig(rangeParam)
}

// defaultRangeConfig maps range strings to their default source routing (Phase 1 behavior).
func defaultRangeConfig(rangeParam string) rangeConfig {
	switch rangeParam {
	case "1h":
		return rangeConfig{1 * time.Hour, "1 minute", true, "raw"}
	case "6h":
		return rangeConfig{6 * time.Hour, "5 minutes", true, "raw"}
	case "12h":
		return rangeConfig{12 * time.Hour, "10 minutes", true, "raw"}
	case "24h":
		return rangeConfig{24 * time.Hour, "15 minutes", true, "raw"}
	case "7d":
		return rangeConfig{7 * 24 * time.Hour, "1 hour", true, "raw"}
	case "30d":
		return rangeConfig{30 * 24 * time.Hour, "1 day", true, "hourly"}
	case "90d":
		return rangeConfig{90 * 24 * time.Hour, "1 day", true, "hourly"}
	case "all":
		return rangeConfig{0, "1 day", false, "hourly"}
	default:
		return rangeConfig{24 * time.Hour, "15 minutes", true, "raw"}
	}
}

// buildMetricsQuery returns the SELECT columns, FROM table, and time column for a metrics query.
func buildMetricsQuery(rc rangeConfig) (selectCols, fromTable, timeCol string) {
	sumCols := `SUM(request_count), SUM(bytes_sent), SUM(bytes_received),
		SUM(status_2xx), SUM(status_3xx), SUM(status_4xx), SUM(status_5xx)`
	tailCols := `SUM(cache_hits), SUM(cache_misses), SUM(unique_ips), SUM(connection_count)`

	switch rc.Source {
	case "15min":
		fromTable = "metrics_15min"
		timeCol = "bucket"
		avgCols := `SUM(latency_weight)/NULLIF(SUM(request_count),0),
			SUM(p50_weight)/NULLIF(SUM(request_count),0),
			SUM(p95_weight)/NULLIF(SUM(request_count),0),
			SUM(p99_weight)/NULLIF(SUM(request_count),0),
			SUM(upstream_weight)/NULLIF(SUM(request_count),0),
			SUM(req_size_weight)/NULLIF(SUM(request_count),0),
			SUM(res_size_weight)/NULLIF(SUM(request_count),0)`
		selectCols = sumCols + ", " + avgCols + ", " + tailCols
	case "hourly":
		fromTable = "metrics_hourly"
		timeCol = "bucket"
		avgCols := `SUM(latency_weight)/NULLIF(SUM(request_count),0),
			SUM(p50_weight)/NULLIF(SUM(request_count),0),
			SUM(p95_weight)/NULLIF(SUM(request_count),0),
			SUM(p99_weight)/NULLIF(SUM(request_count),0),
			SUM(upstream_weight)/NULLIF(SUM(request_count),0),
			SUM(req_size_weight)/NULLIF(SUM(request_count),0),
			SUM(res_size_weight)/NULLIF(SUM(request_count),0)`
		selectCols = sumCols + ", " + avgCols + ", " + tailCols
	default: // "raw"
		fromTable = "metrics"
		timeCol = "time"
		avgCols := `AVG(avg_latency_ms), AVG(latency_p50_ms), AVG(latency_p95_ms), AVG(latency_p99_ms),
			AVG(avg_upstream_ms), AVG(avg_request_size), AVG(avg_response_size)`
		selectCols = sumCols + ", " + avgCols + ", " + tailCols
	}
	return
}

// queryMetrics runs a time_bucket aggregation query and returns buckets, summary, and domain list.
func queryMetrics(agentIDs []string, rc rangeConfig, domain string) ([]models.MetricsBucket, models.MetricsSummary, []string, error) {
	selectCols, fromTable, timeCol := buildMetricsQuery(rc)

	query := fmt.Sprintf(`SELECT time_bucket('%s', %s) AS bucket, domain, %s
		FROM %s WHERE agent_id = ANY($1::text[])`,
		rc.BucketInterval, timeCol, selectCols, fromTable)

	args := []interface{}{agentIDs}
	paramIdx := 2

	if rc.HasTimebound {
		startTime := time.Now().UTC().Add(-rc.Duration)
		query += fmt.Sprintf(" AND %s >= $%d", timeCol, paramIdx)
		args = append(args, startTime)
		paramIdx++
	}

	if domain != "" {
		query += fmt.Sprintf(" AND domain = $%d", paramIdx)
		args = append(args, domain)
	}

	query += `
		GROUP BY bucket, domain
		ORDER BY bucket ASC
	`

	rows, err := database.DB.Query(context.Background(), query, args...)
	if err != nil {
		return nil, models.MetricsSummary{}, nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var buckets []models.MetricsBucket
	domainsMap := make(map[string]struct{})
	var summary models.MetricsSummary

	for rows.Next() {
		var b models.MetricsBucket
		err := rows.Scan(
			&b.Time, &b.Domain,
			&b.RequestCount, &b.BytesSent, &b.BytesReceived,
			&b.Status2xx, &b.Status3xx, &b.Status4xx, &b.Status5xx,
			&b.AvgLatencyMs, &b.LatencyP50Ms, &b.LatencyP95Ms, &b.LatencyP99Ms,
			&b.AvgUpstreamMs, &b.AvgRequestSize, &b.AvgResponseSize,
			&b.CacheHits, &b.CacheMisses, &b.UniqueIPs, &b.ConnectionCount,
		)
		if err != nil {
			log.Printf("Failed to scan metrics row: %v", err)
			continue
		}
		buckets = append(buckets, b)
		domainsMap[b.Domain] = struct{}{}

		summary.TotalRequests += b.RequestCount
		summary.TotalBytesSent += b.BytesSent
		summary.TotalBytesReceived += b.BytesReceived
		summary.Total2xx += b.Status2xx
		summary.Total3xx += b.Status3xx
		summary.Total4xx += b.Status4xx
		summary.Total5xx += b.Status5xx
	}
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating metrics rows: %v", err)
	}

	if summary.TotalRequests > 0 {
		totalErrors := summary.Total4xx + summary.Total5xx
		summary.ErrorRate = float64(totalErrors) / float64(summary.TotalRequests) * 100
	}

	if len(buckets) > 0 {
		var totalWeightedLatency float64
		var totalWeight int64
		for _, b := range buckets {
			totalWeightedLatency += b.AvgLatencyMs * float64(b.RequestCount)
			totalWeight += b.RequestCount
		}
		if totalWeight > 0 {
			summary.AvgLatencyMs = totalWeightedLatency / float64(totalWeight)
		}
	}

	var domains []string
	for d := range domainsMap {
		domains = append(domains, d)
	}

	return buckets, summary, domains, nil
}

// GetAgentMetrics returns aggregated metrics for an agent over a time range
func GetAgentMetrics(c *fiber.Ctx) error {
	agentID := c.Params("agentId")
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	// Verify agent access
	if role == "admin" {
		var exists bool
		err := database.DB.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM agents WHERE agent_id = $1)`, agentID,
		).Scan(&exists)
		if err != nil || !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Agent not found"})
		}
	} else {
		var exists bool
		err := database.DB.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM agents WHERE agent_id = $1 AND (user_id = $2 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $2)))`, agentID, userID,
		).Scan(&exists)
		if err != nil || !exists {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
		}
	}

	rc := parseRange(c.Query("range", "1h"))
	domain := c.Query("domain", "")

	buckets, summary, domains, err := queryMetrics([]string{agentID}, rc, domain)
	if err != nil {
		log.Printf("Failed to query metrics: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query metrics"})
	}

	return c.JSON(models.MetricsResponse{
		Buckets: buckets,
		Summary: summary,
		Domains: domains,
	})
}

// GetGlobalMetrics returns aggregated metrics across all (or filtered) agents
func GetGlobalMetrics(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	// Fetch agents based on role
	var agentQuery string
	var agentArgs []interface{}
	if role == "admin" {
		agentQuery = `SELECT agent_id, name, status FROM agents ORDER BY name`
	} else {
		agentQuery = `SELECT agent_id, name, status FROM agents WHERE user_id = $1 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $1) ORDER BY name`
		agentArgs = append(agentArgs, userID)
	}

	agentRows, err := database.DB.Query(context.Background(), agentQuery, agentArgs...)
	if err != nil {
		log.Printf("Failed to query agents: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query agents"})
	}
	defer agentRows.Close()

	var allAgents []models.AgentSummary
	var allAgentIDs []string
	for agentRows.Next() {
		var a models.AgentSummary
		if err := agentRows.Scan(&a.AgentID, &a.Name, &a.Status); err != nil {
			log.Printf("Failed to scan agent row: %v", err)
			continue
		}
		allAgents = append(allAgents, a)
		allAgentIDs = append(allAgentIDs, a.AgentID)
	}
	if err := agentRows.Err(); err != nil {
		log.Printf("Error iterating agents for global metrics: %v", err)
	}

	// Determine which agent IDs to query
	agentFilter := c.Query("agent", "")
	queryAgentIDs := allAgentIDs
	if agentFilter != "" {
		// Verify the requested agent belongs to accessible agents
		found := false
		for _, id := range allAgentIDs {
			if id == agentFilter {
				found = true
				break
			}
		}
		if !found {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
		}
		queryAgentIDs = []string{agentFilter}
	}

	if len(queryAgentIDs) == 0 {
		return c.JSON(models.GlobalMetricsResponse{
			Buckets: []models.MetricsBucket{},
			Summary: models.MetricsSummary{},
			Domains: []string{},
			Agents:  allAgents,
		})
	}

	rangeParam := c.Query("range", "24h")
	rc := parseRange(rangeParam)
	domain := c.Query("domain", "")

	buckets, summary, domains, err := queryMetrics(queryAgentIDs, rc, domain)
	if err != nil {
		log.Printf("Failed to query global metrics: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query metrics"})
	}

	log.Printf("[metrics] GET /api/metrics range=%s agent=%s domain=%s → %d buckets, %d domains, %d total_requests",
		rangeParam, agentFilter, domain, len(buckets), len(domains), summary.TotalRequests)

	return c.JSON(models.GlobalMetricsResponse{
		Buckets: buckets,
		Summary: summary,
		Domains: domains,
		Agents:  allAgents,
	})
}

// GetAgentMetricsLive sends a get_metrics command to the agent and returns live data
func GetAgentMetricsLive(c *fiber.Ctx) error {
	agentID := c.Params("agentId")
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	// Verify agent access
	if role == "admin" {
		var exists bool
		err := database.DB.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM agents WHERE agent_id = $1)`, agentID,
		).Scan(&exists)
		if err != nil || !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Agent not found"})
		}
	} else {
		var exists bool
		err := database.DB.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM agents WHERE agent_id = $1 AND (user_id = $2 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $2)))`, agentID, userID,
		).Scan(&exists)
		if err != nil || !exists {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
		}
	}

	// Local agent: metrics are collected internally, return from DB
	if IsLocalAgent(agentID) {
		rc := parseRange("1h")
		buckets, summary, _, err := queryMetrics([]string{agentID}, rc, "")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to get metrics: %v", err),
			})
		}
		result, _ := json.Marshal(map[string]interface{}{
			"buckets": buckets,
			"summary": summary,
		})
		return c.JSON(fiber.Map{
			"metrics": string(result),
		})
	}

	command := models.AgentCommand{
		Type:    "get_metrics",
		Payload: map[string]interface{}{},
	}

	response, err := SendCommandAndWaitForResponse(agentID, command, CmdTimeoutDefault)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to get live metrics: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"metrics": response,
	})
}

// GetVisitorIPs returns aggregated visitor IP data with geo info
func GetVisitorIPs(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	// Fetch agent IDs based on role
	var agentQuery string
	var agentArgs []interface{}
	if role == "admin" {
		agentQuery = `SELECT agent_id FROM agents`
	} else {
		agentQuery = `SELECT agent_id FROM agents WHERE user_id = $1 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $1)`
		agentArgs = append(agentArgs, userID)
	}

	agentRows, err := database.DB.Query(context.Background(), agentQuery, agentArgs...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query agents"})
	}
	defer agentRows.Close()

	var agentIDs []string
	for agentRows.Next() {
		var id string
		if err := agentRows.Scan(&id); err == nil {
			agentIDs = append(agentIDs, id)
		}
	}
	if err := agentRows.Err(); err != nil {
		log.Printf("[visitors] Error iterating agent rows: %v", err)
	}

	if len(agentIDs) == 0 {
		return c.JSON(models.VisitorIPsResponse{Visitors: []models.VisitorIP{}, Total: 0})
	}

	// Filter by agent if specified
	agentFilter := c.Query("agent", "")
	if agentFilter != "" {
		found := false
		for _, id := range agentIDs {
			if id == agentFilter {
				found = true
				break
			}
		}
		if !found {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
		}
		agentIDs = []string{agentFilter}
	}

	rc := parseRange(c.Query("range", "24h"))

	// Parse limit
	limit := c.QueryInt("limit", 100)
	if limit > 500 {
		limit = 500
	}
	if limit < 1 {
		limit = 100
	}

	// Build query - exclude private/local IPs
	query := `SELECT ip_address, SUM(request_count) as total_requests
		FROM visitor_ips
		WHERE agent_id = ANY($1::text[])
		AND ip_address NOT LIKE '10.%'
		AND ip_address NOT LIKE '172.16.%' AND ip_address NOT LIKE '172.17.%' AND ip_address NOT LIKE '172.18.%'
		AND ip_address NOT LIKE '172.19.%' AND ip_address NOT LIKE '172.2_.%' AND ip_address NOT LIKE '172.30.%'
		AND ip_address NOT LIKE '172.31.%'
		AND ip_address NOT LIKE '192.168.%'
		AND ip_address NOT LIKE '127.%'
		AND ip_address != '::1'`

	args := []interface{}{agentIDs}
	paramIdx := 2

	if rc.HasTimebound {
		startTime := time.Now().UTC().Add(-rc.Duration)
		query += fmt.Sprintf(" AND time >= $%d", paramIdx)
		args = append(args, startTime)
		paramIdx++
	}

	domain := c.Query("domain", "")
	if domain != "" {
		query += fmt.Sprintf(" AND domain = $%d", paramIdx)
		args = append(args, domain)
		paramIdx++
	}

	query += fmt.Sprintf(`
		GROUP BY ip_address
		ORDER BY total_requests DESC
		LIMIT $%d`, paramIdx)
	args = append(args, limit)

	rows, err := database.DB.Query(context.Background(), query, args...)
	if err != nil {
		log.Printf("[visitors] Query error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query visitors"})
	}
	defer rows.Close()

	var visitors []models.VisitorIP
	var ipList []string
	for rows.Next() {
		var v models.VisitorIP
		if err := rows.Scan(&v.IPAddress, &v.RequestCount); err != nil {
			log.Printf("[visitors] Scan error: %v", err)
			continue
		}
		visitors = append(visitors, v)
		ipList = append(ipList, v.IPAddress)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[visitors] Error iterating visitor rows: %v", err)
	}

	// Enrich with geo data
	if len(ipList) > 0 {
		geoData, err := LookupGeo(ipList)
		if err != nil {
			log.Printf("[visitors] Geo lookup error: %v", err)
		}
		if geoData != nil {
			for i, v := range visitors {
				if geo, ok := geoData[v.IPAddress]; ok {
					visitors[i].Country = geo.Country
					visitors[i].CountryCode = geo.CountryCode
					visitors[i].City = geo.City
					visitors[i].Region = geo.Region
				}
			}
		}
	}

	if visitors == nil {
		visitors = []models.VisitorIP{}
	}

	return c.JSON(models.VisitorIPsResponse{
		Visitors: visitors,
		Total:    len(visitors),
	})
}

// BlockedEntry represents a consolidated blocked IP from CrowdSec alerts
type BlockedEntry struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	ASName      string `json:"as_name"`
	ASNumber    string `json:"as_number"`
	EventsCount int    `json:"events_count"`
	LastSeen    string `json:"last_seen"`
	AgentName   string `json:"agent_name"`
	AgentID     string `json:"agent_id"`
}

// CrowdSecAlert mirrors the agent's alert JSON structure
type CrowdSecAlert struct {
	ID        int    `json:"id"`
	CreatedAt string `json:"created_at"`
	Scenario  string `json:"scenario"`
	Message   string `json:"message"`
	Source    struct {
		IP      string `json:"ip"`
		Scope   string `json:"scope"`
		Value   string `json:"value"`
		ASName  string `json:"as_name"`
		ASNum   string `json:"as_number"`
		Country string `json:"cn"`
	} `json:"source"`
	EventsCount int  `json:"events_count"`
	StartAt     string `json:"start_at"`
	StopAt      string `json:"stop_at"`
	Remediation bool   `json:"remediation"`
}

// GetRecentBlocked returns recent blocked connections aggregated from all agents with CrowdSec
func GetRecentBlocked(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	// Get agents with CrowdSec installed based on role
	var agentQuery string
	var agentArgs []interface{}
	if role == "admin" {
		agentQuery = `SELECT agent_id, name FROM agents WHERE crowdsec_installed = true`
	} else {
		agentQuery = `SELECT agent_id, name FROM agents WHERE (user_id = $1 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $1)) AND crowdsec_installed = true`
		agentArgs = append(agentArgs, userID)
	}

	rows, err := database.DB.Query(context.Background(), agentQuery, agentArgs...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query agents"})
	}
	defer rows.Close()

	type agentInfo struct {
		ID   string
		Name string
	}
	var agents []agentInfo
	for rows.Next() {
		var a agentInfo
		if err := rows.Scan(&a.ID, &a.Name); err == nil {
			agents = append(agents, a)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[blocked] Error iterating agent rows: %v", err)
	}

	if len(agents) == 0 {
		return c.JSON(fiber.Map{"blocked": []BlockedEntry{}, "total": 0})
	}

	// Filter by specific agent if requested
	agentFilter := c.Query("agent", "")

	// Query each connected agent for alerts in parallel
	var mu sync.Mutex
	ipMap := make(map[string]BlockedEntry)
	var wg sync.WaitGroup

	for _, agent := range agents {
		if agentFilter != "" && agent.ID != agentFilter {
			continue
		}
		wg.Add(1)
		go func(a agentInfo) {
			defer wg.Done()

			var alerts []CrowdSecAlert

			if IsLocalAgent(a.ID) {
				result, err := localAgent.CrowdSecListAlerts()
				if err != nil {
					return
				}
				alertJSON, err := json.Marshal(result)
				if err != nil {
					return
				}
				if err := json.Unmarshal(alertJSON, &alerts); err != nil {
					return
				}
			} else {
				command := models.AgentCommand{
					Type:    "crowdsec_alerts_list",
					Payload: map[string]interface{}{},
				}

				response, err := SendCommandAndWaitForResponse(a.ID, command, CmdTimeoutFast)
				if err != nil {
					return // Agent not connected or timed out
				}

				if err := json.Unmarshal([]byte(response), &alerts); err != nil {
					return
				}
			}

			mu.Lock()
			for _, alert := range alerts {
				if alert.Source.IP == "" {
					continue
				}
				if existing, ok := ipMap[alert.Source.IP]; ok {
					existing.EventsCount += alert.EventsCount
					if alert.CreatedAt > existing.LastSeen {
						existing.LastSeen = alert.CreatedAt
					}
					ipMap[alert.Source.IP] = existing
				} else {
					ipMap[alert.Source.IP] = BlockedEntry{
						IP:          alert.Source.IP,
						Country:     alert.Source.Country,
						CountryCode: alert.Source.Country,
						ASName:      alert.Source.ASName,
						ASNumber:    alert.Source.ASNum,
						EventsCount: alert.EventsCount,
						LastSeen:    alert.CreatedAt,
						AgentName:   a.Name,
						AgentID:     a.ID,
					}
				}
			}
			mu.Unlock()
		}(agent)
	}

	wg.Wait()

	// Convert map to slice
	allBlocked := make([]BlockedEntry, 0, len(ipMap))
	for _, entry := range ipMap {
		allBlocked = append(allBlocked, entry)
	}

	// Sort by last_seen descending (most recent first)
	sort.Slice(allBlocked, func(i, j int) bool {
		return allBlocked[i].LastSeen > allBlocked[j].LastSeen
	})

	// Limit results
	limit := c.QueryInt("limit", 50)
	if limit > 200 {
		limit = 200
	}
	if len(allBlocked) > limit {
		allBlocked = allBlocked[:limit]
	}

	if allBlocked == nil {
		allBlocked = []BlockedEntry{}
	}

	return c.JSON(fiber.Map{"blocked": allBlocked, "total": len(allBlocked)})
}

// GetNginxLogs returns raw nginx access logs from agents
func GetNginxLogs(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	// Get target agent - required param
	agentFilter := c.Query("agent", "")

	// Get agents based on role, optionally filtering
	var agentIDs []string
	var agentName string

	var agentQuery string
	var agentArgs []interface{}
	if role == "admin" {
		agentQuery = `SELECT agent_id, name FROM agents`
	} else {
		agentQuery = `SELECT agent_id, name FROM agents WHERE user_id = $1 OR id IN (SELECT agent_id FROM user_agents WHERE user_id = $1)`
		agentArgs = append(agentArgs, userID)
	}

	rows, err := database.DB.Query(context.Background(), agentQuery, agentArgs...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query agents"})
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			if agentFilter == "" || id == agentFilter {
				agentIDs = append(agentIDs, id)
				agentName = name
			}
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("[nginx-logs] Error iterating agent rows: %v", err)
	}

	if len(agentIDs) == 0 {
		return c.JSON(fiber.Map{"logs": "", "agent": ""})
	}

	// Use first matching agent
	targetAgent := agentIDs[0]
	lines := c.QueryInt("lines", 200)
	if lines > 1000 {
		lines = 1000
	}

	// Local agent: get logs directly
	if IsLocalAgent(targetAgent) {
		logs, err := localAgent.GetNginxLogs(lines)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"logs": logs, "agent": agentName})
	}

	command := models.AgentCommand{
		Type: "get_nginx_logs",
		Payload: map[string]interface{}{
			"lines": lines,
		},
	}

	response, err := SendCommandAndWaitForResponse(targetAgent, command, CmdTimeoutFast)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"logs": response, "agent": agentName})
}
