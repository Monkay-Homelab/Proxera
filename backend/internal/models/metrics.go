package models

import "time"

// MetricsBucket represents a single time-series metrics data point
type MetricsBucket struct {
	Time            time.Time `json:"time"`
	AgentID         string    `json:"agent_id"`
	Domain          string    `json:"domain"`
	RequestCount    int64     `json:"request_count"`
	BytesSent       int64     `json:"bytes_sent"`
	BytesReceived   int64     `json:"bytes_received"`
	Status2xx       int64     `json:"status_2xx"`
	Status3xx       int64     `json:"status_3xx"`
	Status4xx       int64     `json:"status_4xx"`
	Status5xx       int64     `json:"status_5xx"`
	AvgLatencyMs    float64   `json:"avg_latency_ms"`
	LatencyP50Ms    float64   `json:"latency_p50_ms"`
	LatencyP95Ms    float64   `json:"latency_p95_ms"`
	LatencyP99Ms    float64   `json:"latency_p99_ms"`
	AvgUpstreamMs   float64   `json:"avg_upstream_ms"`
	AvgRequestSize  float64   `json:"avg_request_size"`
	AvgResponseSize float64   `json:"avg_response_size"`
	CacheHits       int64     `json:"cache_hits"`
	CacheMisses     int64     `json:"cache_misses"`
	UniqueIPs       int       `json:"unique_ips"`
	ConnectionCount int64     `json:"connection_count"`
}

// MetricsSummary provides aggregate totals for a time range
type MetricsSummary struct {
	TotalRequests int64   `json:"total_requests"`
	TotalBytesSent int64  `json:"total_bytes_sent"`
	TotalBytesReceived int64 `json:"total_bytes_received"`
	Total2xx      int64   `json:"total_2xx"`
	Total3xx      int64   `json:"total_3xx"`
	Total4xx      int64   `json:"total_4xx"`
	Total5xx      int64   `json:"total_5xx"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	ErrorRate     float64 `json:"error_rate"`
}

// MetricsResponse is returned by the per-agent metrics API
type MetricsResponse struct {
	Buckets []MetricsBucket `json:"buckets"`
	Summary MetricsSummary  `json:"summary"`
	Domains []string        `json:"domains"`
}

// GlobalMetricsResponse is returned by the global metrics API
type GlobalMetricsResponse struct {
	Buckets []MetricsBucket `json:"buckets"`
	Summary MetricsSummary  `json:"summary"`
	Domains []string        `json:"domains"`
	Agents  []AgentSummary  `json:"agents"`
}

// AgentSummary is a lightweight agent descriptor for filter dropdowns
type AgentSummary struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}

// IncomingMetricsBucket is the struct used when unmarshalling metrics from agent reports
type IncomingMetricsBucket struct {
	Timestamp       string           `json:"timestamp"`
	Domain          string           `json:"domain"`
	RequestCount    int64            `json:"request_count"`
	BytesSent       int64            `json:"bytes_sent"`
	BytesReceived   int64            `json:"bytes_received"`
	Status2xx       int64            `json:"status_2xx"`
	Status3xx       int64            `json:"status_3xx"`
	Status4xx       int64            `json:"status_4xx"`
	Status5xx       int64            `json:"status_5xx"`
	AvgLatencyMs    float64          `json:"avg_latency_ms"`
	LatencyP50Ms    float64          `json:"latency_p50_ms"`
	LatencyP95Ms    float64          `json:"latency_p95_ms"`
	LatencyP99Ms    float64          `json:"latency_p99_ms"`
	AvgUpstreamMs   float64          `json:"avg_upstream_ms"`
	AvgRequestSize  float64          `json:"avg_request_size"`
	AvgResponseSize float64          `json:"avg_response_size"`
	CacheHits       int64            `json:"cache_hits"`
	CacheMisses     int64            `json:"cache_misses"`
	UniqueIPs       int              `json:"unique_ips"`
	ConnectionCount int64            `json:"connection_count"`
	IPRequestCounts map[string]int64 `json:"ip_request_counts,omitempty"`
}

// VisitorIP represents a visitor IP with request count and geo data
type VisitorIP struct {
	IPAddress    string `json:"ip_address"`
	RequestCount int64  `json:"request_count"`
	Country      string `json:"country"`
	CountryCode  string `json:"country_code"`
	City         string `json:"city"`
	Region       string `json:"region"`
}

// VisitorIPsResponse is the API response for the visitors endpoint
type VisitorIPsResponse struct {
	Visitors []VisitorIP `json:"visitors"`
	Total    int         `json:"total"`
}
