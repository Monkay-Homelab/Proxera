package metrics

import (
	"math/rand"
	"time"
)

const (
	// maxLatencySamples is the reservoir sampling cap for latency values per bucket
	maxLatencySamples = 10000
	// maxUniqueIPs is the maximum number of unique IPs tracked per bucket
	maxUniqueIPs = 10000
)

// LogEntry represents a single parsed nginx access log line
type LogEntry struct {
	RemoteAddr           string
	Timestamp            time.Time
	Method               string
	Path                 string
	Status               int
	BodyBytesSent        int64
	RequestTime          float64 // seconds
	UpstreamResponseTime float64 // seconds, -1 if not available
	UpstreamStatus       int     // 0 if not available
	UpstreamCacheStatus  string  // HIT, MISS, EXPIRED, etc. or "" if not available
	RequestLength        int64
	Connection           int64
	ConnectionRequests   int64
}

// MetricsBucket holds aggregated metrics for a 1-minute window per domain
type MetricsBucket struct {
	Timestamp       time.Time `json:"timestamp"`
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
	UniqueIPs       int                `json:"unique_ips"`
	IPRequestCounts map[string]int64  `json:"ip_request_counts,omitempty"`
	ConnectionCount int64              `json:"connection_count"`
}

// bucketAccumulator holds raw data for an in-progress bucket before finalization
type bucketAccumulator struct {
	domain           string
	requestCount     int64
	bytesSent        int64
	bytesReceived    int64
	status2xx        int64
	status3xx        int64
	status4xx        int64
	status5xx        int64
	totalLatencyMs   float64
	latencies        []float64
	latencySeen      int64 // total latency samples seen (for reservoir sampling)
	totalUpstreamMs  float64
	upstreamCount    int64
	totalRequestSize int64
	totalRespSize    int64
	cacheHits        int64
	cacheMisses      int64
	uniqueIPs        map[string]int64
	ipOverflow       bool // true when uniqueIPs hit maxUniqueIPs
	connections      map[int64]struct{}
}

func newBucketAccumulator(domain string) *bucketAccumulator {
	return &bucketAccumulator{
		domain:      domain,
		uniqueIPs:   make(map[string]int64),
		connections: make(map[int64]struct{}),
	}
}

// addLatency uses reservoir sampling to keep at most maxLatencySamples values
func (acc *bucketAccumulator) addLatency(latencyMs float64) {
	acc.latencySeen++
	if len(acc.latencies) < maxLatencySamples {
		acc.latencies = append(acc.latencies, latencyMs)
	} else {
		// Reservoir sampling: replace a random element with probability maxLatencySamples/latencySeen
		j := rand.Int63n(acc.latencySeen)
		if j < maxLatencySamples {
			acc.latencies[j] = latencyMs
		}
	}
}

// addIP tracks a unique IP, stopping new entries after maxUniqueIPs
func (acc *bucketAccumulator) addIP(ip string) {
	if _, exists := acc.uniqueIPs[ip]; exists {
		acc.uniqueIPs[ip]++
		return
	}
	if len(acc.uniqueIPs) >= maxUniqueIPs {
		acc.ipOverflow = true
		return
	}
	acc.uniqueIPs[ip] = 1
}

// LogPosition tracks the read position for a single log file
type LogPosition struct {
	Inode  uint64 `json:"inode"`
	Offset int64  `json:"offset"`
}
