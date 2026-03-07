package metrics

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// proxera_metrics log format:
// $remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent
// $request_time $upstream_response_time $upstream_status $upstream_cache_status
// $request_length $connection $connection_requests
//
// Example:
// 203.0.113.45 - - [08/Feb/2026:12:01:00 +0000] "GET /api/data HTTP/1.1" 200 1234 0.015 0.012 200 - 456 123 1

var logLineRegex = regexp.MustCompile(
	`^(\S+) - \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) (\S+) (\S+) (\S+) (\S+) (\S+) (\S+) (\S+)$`,
)

// ParseLine parses a single proxera_metrics format log line
func ParseLine(line string) (*LogEntry, error) {
	matches := logLineRegex.FindStringSubmatch(strings.TrimSpace(line))
	if matches == nil {
		return nil, nil // not a matching line, skip silently
	}

	entry := &LogEntry{
		RemoteAddr: matches[1],
	}

	// Parse timestamp: 08/Feb/2026:12:01:00 +0000
	t, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[2])
	if err != nil {
		return nil, nil
	}
	entry.Timestamp = t

	entry.Method = matches[3]
	entry.Path = matches[4]

	if v, err := strconv.Atoi(matches[5]); err == nil {
		entry.Status = v
	}
	if v, err := strconv.ParseInt(matches[6], 10, 64); err == nil {
		entry.BodyBytesSent = v
	}
	if v, err := strconv.ParseFloat(matches[7], 64); err == nil {
		entry.RequestTime = v
	}

	// Upstream response time can be "-"
	entry.UpstreamResponseTime = -1
	if matches[8] != "-" {
		if v, err := strconv.ParseFloat(matches[8], 64); err == nil {
			entry.UpstreamResponseTime = v
		}
	}

	// Upstream status can be "-"
	if matches[9] != "-" {
		if v, err := strconv.Atoi(matches[9]); err == nil {
			entry.UpstreamStatus = v
		}
	}

	// Upstream cache status can be "-"
	if matches[10] != "-" {
		entry.UpstreamCacheStatus = matches[10]
	}

	if v, err := strconv.ParseInt(matches[11], 10, 64); err == nil {
		entry.RequestLength = v
	}
	if v, err := strconv.ParseInt(matches[12], 10, 64); err == nil {
		entry.Connection = v
	}
	if v, err := strconv.ParseInt(matches[13], 10, 64); err == nil {
		entry.ConnectionRequests = v
	}

	return entry, nil
}
