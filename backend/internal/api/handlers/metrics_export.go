package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/proxera/backend/internal/database"
)

// ExportMetrics streams metrics data as CSV or JSON with a download header.
// GET /api/metrics/export?range=24h&agent=...&domain=...&format=csv|json
func ExportMetrics(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(int)
	role, _ := c.Locals("user_role").(string)

	// Fetch agents: admins see all, others see owned + assigned
	var agentQuery string
	var agentArgs []interface{}
	if role == "admin" {
		agentQuery = `SELECT agent_id, name FROM agents ORDER BY name`
	} else {
		agentQuery = `SELECT agent_id, name FROM agents WHERE user_id = $1
			UNION
			SELECT a.agent_id, a.name FROM agents a
			JOIN user_agents ua ON ua.agent_id = a.id
			WHERE ua.user_id = $1
			ORDER BY name`
		agentArgs = append(agentArgs, userID)
	}

	agentRows, err := database.DB.Query(context.Background(), agentQuery, agentArgs...)
	if err != nil {
		log.Printf("[export] Failed to query agents: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query agents"})
	}
	defer agentRows.Close()

	type agentMeta struct {
		ID   string
		Name string
	}
	var allAgents []agentMeta
	var allAgentIDs []string
	for agentRows.Next() {
		var a agentMeta
		if err := agentRows.Scan(&a.ID, &a.Name); err == nil {
			allAgents = append(allAgents, a)
			allAgentIDs = append(allAgentIDs, a.ID)
		}
	}
	if err := agentRows.Err(); err != nil {
		log.Printf("[export] Error iterating agents: %v", err)
	}

	// Optionally filter to a single agent
	agentFilter := c.Query("agent", "")
	queryAgentIDs := allAgentIDs
	if agentFilter != "" {
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
		return c.Status(fiber.StatusNoContent).Send(nil)
	}

	rangeParam := c.Query("range", "24h")
	domain := c.Query("domain", "")
	format := c.Query("format", "csv") // "csv" or "json"

	rc := parseRange(rangeParam)
	buckets, summary, _, err := queryMetrics(queryAgentIDs, rc, domain)
	if err != nil {
		log.Printf("[export] Failed to query metrics: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to query metrics"})
	}

	// Build a human-readable filename
	ts := time.Now().UTC().Format("20060102_150405")
	agentSuffix := "all"
	if agentFilter != "" {
		for _, a := range allAgents {
			if a.ID == agentFilter {
				agentSuffix = sanitizeFilename(a.Name)
				break
			}
		}
	}
	domainSuffix := ""
	if domain != "" {
		domainSuffix = "_" + sanitizeFilename(domain)
	}
	baseFilename := fmt.Sprintf("proxera_metrics_%s_%s%s_%s", agentSuffix, rangeParam, domainSuffix, ts)

	if format == "json" {
		filename := baseFilename + ".json"
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Set("Content-Type", "application/json")

		out := fiber.Map{
			"exported_at": time.Now().UTC().Format(time.RFC3339),
			"range":       rangeParam,
			"agent":       agentFilter,
			"domain":      domain,
			"summary":     summary,
			"buckets":     buckets,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode JSON"})
		}
		return c.Send(data)
	}

	// Default: CSV
	filename := baseFilename + ".csv"
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Set("Content-Type", "text/csv; charset=utf-8")

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Header row
	w.Write([]string{
		"time", "domain",
		"request_count", "bytes_sent", "bytes_received",
		"status_2xx", "status_3xx", "status_4xx", "status_5xx",
		"avg_latency_ms", "latency_p50_ms", "latency_p95_ms", "latency_p99_ms",
		"avg_upstream_ms", "avg_request_size_bytes", "avg_response_size_bytes",
		"cache_hits", "cache_misses", "unique_ips", "connection_count",
	})

	for _, b := range buckets {
		w.Write([]string{
			b.Time.UTC().Format(time.RFC3339),
			b.Domain,
			fmt.Sprintf("%d", b.RequestCount),
			fmt.Sprintf("%d", b.BytesSent),
			fmt.Sprintf("%d", b.BytesReceived),
			fmt.Sprintf("%d", b.Status2xx),
			fmt.Sprintf("%d", b.Status3xx),
			fmt.Sprintf("%d", b.Status4xx),
			fmt.Sprintf("%d", b.Status5xx),
			fmt.Sprintf("%.3f", b.AvgLatencyMs),
			fmt.Sprintf("%.3f", b.LatencyP50Ms),
			fmt.Sprintf("%.3f", b.LatencyP95Ms),
			fmt.Sprintf("%.3f", b.LatencyP99Ms),
			fmt.Sprintf("%.3f", b.AvgUpstreamMs),
			fmt.Sprintf("%.1f", b.AvgRequestSize),
			fmt.Sprintf("%.1f", b.AvgResponseSize),
			fmt.Sprintf("%d", b.CacheHits),
			fmt.Sprintf("%d", b.CacheMisses),
			fmt.Sprintf("%d", b.UniqueIPs),
			fmt.Sprintf("%d", b.ConnectionCount),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to write CSV"})
	}

	return c.Send(buf.Bytes())
}

// sanitizeFilename strips characters unsuitable for a filename.
func sanitizeFilename(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out = append(out, c)
		} else if c == '.' || c == ' ' {
			out = append(out, '_')
		}
	}
	return string(out)
}
