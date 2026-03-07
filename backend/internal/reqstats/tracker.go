package reqstats

import (
	"sort"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// minuteBucket holds counters for a single UTC minute.
type minuteBucket struct {
	Minute      time.Time
	Requests    int64
	Errors      int64   // 4xx + 5xx
	Latencies   []int64 // ms — capped at 2000 samples/min
}

// RequestTracker collects per-minute request statistics in a 60-slot ring buffer.
type RequestTracker struct {
	mu        sync.Mutex
	buckets   [60]minuteBucket
	StartedAt time.Time
}

// GlobalRequestTracker is the singleton used by the middleware and admin handler.
var GlobalRequestTracker = &RequestTracker{StartedAt: time.Now()}

// Record registers one completed request.
func (rt *RequestTracker) Record(statusCode int, latencyMs int64) {
	now := time.Now().UTC().Truncate(time.Minute)
	slot := int(now.Unix()/60) % 60

	rt.mu.Lock()
	defer rt.mu.Unlock()

	b := &rt.buckets[slot]
	if !b.Minute.Equal(now) {
		// New minute — reset slot
		*b = minuteBucket{Minute: now}
	}
	b.Requests++
	if statusCode >= 400 {
		b.Errors++
	}
	if len(b.Latencies) < 2000 {
		b.Latencies = append(b.Latencies, latencyMs)
	}
}

// Stats returns aggregate stats for the last n complete minutes (1 ≤ n ≤ 59).
func (rt *RequestTracker) Stats(minutes int) (requests, errors int64, avgMs float64, p95Ms float64) {
	if minutes < 1 {
		minutes = 1
	}
	if minutes > 59 {
		minutes = 59
	}

	cutoff := time.Now().UTC().Truncate(time.Minute).Add(-time.Duration(minutes) * time.Minute)

	rt.mu.Lock()
	defer rt.mu.Unlock()

	var allLatencies []int64
	var totalLatency int64

	for i := range rt.buckets {
		b := &rt.buckets[i]
		if b.Minute.IsZero() || !b.Minute.After(cutoff) {
			continue
		}
		requests += b.Requests
		errors += b.Errors
		totalLatency += func() int64 {
			var s int64
			for _, l := range b.Latencies {
				s += l
			}
			return s
		}()
		allLatencies = append(allLatencies, b.Latencies...)
	}

	if requests > 0 && len(allLatencies) > 0 {
		avgMs = float64(totalLatency) / float64(len(allLatencies))
	}

	if len(allLatencies) > 0 {
		sorted := make([]int64, len(allLatencies))
		copy(sorted, allLatencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		idx := int(float64(len(sorted)) * 0.95)
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		p95Ms = float64(sorted[idx])
	}
	return
}

// TrackRequests is Fiber middleware that feeds GlobalRequestTracker.
func TrackRequests() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		latencyMs := time.Since(start).Milliseconds()
		GlobalRequestTracker.Record(c.Response().StatusCode(), latencyMs)
		return err
	}
}
