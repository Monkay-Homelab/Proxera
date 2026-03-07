package metrics

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const positionsFile = "/var/lib/proxera/log_positions.json"

// Collector reads nginx access logs incrementally and aggregates into metric buckets
type Collector struct {
	logDir    string
	positions map[string]LogPosition // filepath -> position
	mu        sync.Mutex

	// In-progress buckets: key is "domain|minute_timestamp"
	buckets map[string]*bucketAccumulator
	// Completed buckets ready to flush
	completed []MetricsBucket
}

// NewCollector creates a new metrics collector for the given nginx log directory
func NewCollector(logDir string) *Collector {
	c := &Collector{
		logDir:    logDir,
		positions: make(map[string]LogPosition),
		buckets:   make(map[string]*bucketAccumulator),
	}
	c.loadPositions()
	return c
}

// Collect reads new lines from all domain access logs and the global access log,
// parses them, and aggregates into 1-minute buckets.
func (c *Collector) Collect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find all proxera domain access logs
	pattern := filepath.Join(c.logDir, "*_access.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("metrics: failed to glob log files: %v", err)
		return
	}

	// Also check global access.log
	globalLog := filepath.Join(c.logDir, "access.log")
	if _, err := os.Stat(globalLog); err == nil {
		files = append(files, globalLog)
	}

	for _, file := range files {
		c.collectFile(file)
	}

	// Move completed buckets (older than 1 minute) to the completed list
	cutoff := time.Now().UTC().Truncate(time.Minute)
	for key, acc := range c.buckets {
		// Extract the minute timestamp from the key
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		minuteStr := parts[1]
		minuteTime, err := time.Parse(time.RFC3339, minuteStr)
		if err != nil {
			continue
		}
		if minuteTime.Before(cutoff) {
			c.completed = append(c.completed, finalizeBucket(minuteTime, acc))
			delete(c.buckets, key)
		}
	}

	c.savePositions()
}

// Flush returns all completed metric buckets and clears the completed list.
func (c *Collector) Flush() []MetricsBucket {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.completed) == 0 {
		return nil
	}

	result := c.completed
	c.completed = nil
	return result
}

// FlushAll flushes everything including in-progress buckets (for on-demand requests)
func (c *Collector) FlushAll() []MetricsBucket {
	c.mu.Lock()
	defer c.mu.Unlock()

	var result []MetricsBucket
	result = append(result, c.completed...)
	c.completed = nil

	for key, acc := range c.buckets {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		minuteTime, err := time.Parse(time.RFC3339, parts[1])
		if err != nil {
			continue
		}
		result = append(result, finalizeBucket(minuteTime, acc))
		delete(c.buckets, key)
	}

	return result
}

func (c *Collector) collectFile(filePath string) {
	// Determine domain from filename
	domain := domainFromFilename(filePath)

	// Get file info to check inode
	info, err := os.Stat(filePath)
	if err != nil {
		return
	}
	inode := fileInode(info)

	// Check if log was rotated (inode changed) or truncated (size < offset)
	pos, exists := c.positions[filePath]
	if exists && pos.Inode != inode {
		// Log rotated, reset
		pos = LogPosition{Inode: inode, Offset: 0}
	} else if exists && info.Size() < pos.Offset {
		// File truncated (same inode but smaller), reset
		pos = LogPosition{Inode: inode, Offset: 0}
	} else if !exists {
		pos = LogPosition{Inode: inode, Offset: 0}
	}

	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	// Seek to last known position
	if pos.Offset > 0 {
		newOffset, err := f.Seek(pos.Offset, io.SeekStart)
		if err != nil || newOffset != pos.Offset {
			f.Seek(0, io.SeekStart)
			pos.Offset = 0
		}
	}

	scanner := bufio.NewScanner(f)
	// Increase buffer for potentially long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		entry, err := ParseLine(line)
		if err != nil || entry == nil {
			continue
		}

		c.addEntry(domain, entry)
	}

	// Update position
	currentOffset, _ := f.Seek(0, io.SeekCurrent)
	pos.Offset = currentOffset
	c.positions[filePath] = pos
}

func (c *Collector) addEntry(domain string, entry *LogEntry) {
	minuteKey := entry.Timestamp.UTC().Truncate(time.Minute).Format(time.RFC3339)
	bucketKey := domain + "|" + minuteKey

	acc, ok := c.buckets[bucketKey]
	if !ok {
		acc = newBucketAccumulator(domain)
		c.buckets[bucketKey] = acc
	}

	acc.requestCount++
	acc.bytesSent += entry.BodyBytesSent
	acc.bytesReceived += entry.RequestLength

	switch {
	case entry.Status >= 200 && entry.Status < 300:
		acc.status2xx++
	case entry.Status >= 300 && entry.Status < 400:
		acc.status3xx++
	case entry.Status >= 400 && entry.Status < 500:
		acc.status4xx++
	case entry.Status >= 500:
		acc.status5xx++
	}

	// Skip WebSocket upgrades (status 101) from latency stats — they are
	// long-lived connections whose $request_time reflects the full session
	// duration (minutes/hours), not actual response latency.
	if entry.Status != 101 {
		latencyMs := entry.RequestTime * 1000
		acc.totalLatencyMs += latencyMs
		acc.addLatency(latencyMs)

		if entry.UpstreamResponseTime >= 0 {
			acc.totalUpstreamMs += entry.UpstreamResponseTime * 1000
			acc.upstreamCount++
		}
	}

	acc.totalRequestSize += entry.RequestLength
	acc.totalRespSize += entry.BodyBytesSent

	switch strings.ToUpper(entry.UpstreamCacheStatus) {
	case "HIT":
		acc.cacheHits++
	case "MISS", "EXPIRED", "STALE", "UPDATING", "REVALIDATED":
		acc.cacheMisses++
	}

	acc.addIP(entry.RemoteAddr)
	if entry.Connection > 0 {
		acc.connections[entry.Connection] = struct{}{}
	}
}

func finalizeBucket(ts time.Time, acc *bucketAccumulator) MetricsBucket {
	bucket := MetricsBucket{
		Timestamp:       ts,
		Domain:          acc.domain,
		RequestCount:    acc.requestCount,
		BytesSent:       acc.bytesSent,
		BytesReceived:   acc.bytesReceived,
		Status2xx:       acc.status2xx,
		Status3xx:       acc.status3xx,
		Status4xx:       acc.status4xx,
		Status5xx:       acc.status5xx,
		CacheHits:       acc.cacheHits,
		CacheMisses:     acc.cacheMisses,
		UniqueIPs:       len(acc.uniqueIPs),
		ConnectionCount: int64(len(acc.connections)),
	}

	if acc.requestCount > 0 {
		bucket.AvgLatencyMs = acc.totalLatencyMs / float64(acc.requestCount)
		bucket.AvgRequestSize = float64(acc.totalRequestSize) / float64(acc.requestCount)
		bucket.AvgResponseSize = float64(acc.totalRespSize) / float64(acc.requestCount)
	}

	if acc.upstreamCount > 0 {
		bucket.AvgUpstreamMs = acc.totalUpstreamMs / float64(acc.upstreamCount)
	}

	// Populate IP request counts (top 500 by request count)
	if len(acc.uniqueIPs) > 0 {
		if len(acc.uniqueIPs) <= 500 {
			bucket.IPRequestCounts = acc.uniqueIPs
		} else {
			type ipCount struct {
				ip    string
				count int64
			}
			sorted := make([]ipCount, 0, len(acc.uniqueIPs))
			for ip, count := range acc.uniqueIPs {
				sorted = append(sorted, ipCount{ip, count})
			}
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].count > sorted[j].count
			})
			bucket.IPRequestCounts = make(map[string]int64, 500)
			for i := 0; i < 500; i++ {
				bucket.IPRequestCounts[sorted[i].ip] = sorted[i].count
			}
		}
	}

	// Compute percentiles
	if len(acc.latencies) > 0 {
		sort.Float64s(acc.latencies)
		bucket.LatencyP50Ms = percentile(acc.latencies, 50)
		bucket.LatencyP95Ms = percentile(acc.latencies, 95)
		bucket.LatencyP99Ms = percentile(acc.latencies, 99)
	}

	return bucket
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := lower + 1
	if upper >= len(sorted) {
		upper = len(sorted) - 1
	}
	weight := rank - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func domainFromFilename(filePath string) string {
	base := filepath.Base(filePath)
	// Format: <sanitized_domain>_access.log  (e.g., app_example_com_access.log)
	// or access.log for global
	if base == "access.log" {
		return "_global"
	}
	name := strings.TrimSuffix(base, "_access.log")
	// Reverse the sanitizeDomain encoding:
	// sanitizeDomain: "_" → "__", then "." → "_"
	// Reverse: "__" → placeholder, "_" → ".", placeholder → "_"
	const placeholder = "\x00"
	s := strings.ReplaceAll(name, "__", placeholder)
	s = strings.ReplaceAll(s, "_", ".")
	return strings.ReplaceAll(s, placeholder, "_")
}

func (c *Collector) loadPositions() {
	data, err := os.ReadFile(positionsFile)
	if err != nil {
		return // No positions file yet
	}
	if err := json.Unmarshal(data, &c.positions); err != nil {
		log.Printf("metrics: failed to parse positions file %s: %v", positionsFile, err)
	}
}

func (c *Collector) savePositions() {
	data, err := json.Marshal(c.positions)
	if err != nil {
		log.Printf("metrics: failed to marshal positions: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(positionsFile), 0755); err != nil {
		log.Printf("metrics: failed to create positions directory: %v", err)
		return
	}
	if err := os.WriteFile(positionsFile, data, 0644); err != nil {
		log.Printf("metrics: failed to write positions file: %v", err)
	}
}
