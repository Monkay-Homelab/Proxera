package metrics

import (
	"testing"
	"time"
)

func TestParseLine_ValidComplete(t *testing.T) {
	line := `203.0.113.45 - admin [08/Feb/2026:12:01:00 +0000] "GET /api/data HTTP/1.1" 200 1234 0.015 0.012 200 HIT 456 123 1`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry, got nil")
	}

	if entry.RemoteAddr != "203.0.113.45" {
		t.Errorf("RemoteAddr = %q, want %q", entry.RemoteAddr, "203.0.113.45")
	}

	expectedTime := time.Date(2026, time.February, 8, 12, 1, 0, 0, time.UTC)
	if !entry.Timestamp.Equal(expectedTime) {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, expectedTime)
	}

	if entry.Method != "GET" {
		t.Errorf("Method = %q, want %q", entry.Method, "GET")
	}
	if entry.Path != "/api/data" {
		t.Errorf("Path = %q, want %q", entry.Path, "/api/data")
	}
	if entry.Status != 200 {
		t.Errorf("Status = %d, want %d", entry.Status, 200)
	}
	if entry.BodyBytesSent != 1234 {
		t.Errorf("BodyBytesSent = %d, want %d", entry.BodyBytesSent, 1234)
	}
	if entry.RequestTime != 0.015 {
		t.Errorf("RequestTime = %f, want %f", entry.RequestTime, 0.015)
	}
	if entry.UpstreamResponseTime != 0.012 {
		t.Errorf("UpstreamResponseTime = %f, want %f", entry.UpstreamResponseTime, 0.012)
	}
	if entry.UpstreamStatus != 200 {
		t.Errorf("UpstreamStatus = %d, want %d", entry.UpstreamStatus, 200)
	}
	if entry.UpstreamCacheStatus != "HIT" {
		t.Errorf("UpstreamCacheStatus = %q, want %q", entry.UpstreamCacheStatus, "HIT")
	}
	if entry.RequestLength != 456 {
		t.Errorf("RequestLength = %d, want %d", entry.RequestLength, 456)
	}
	if entry.Connection != 123 {
		t.Errorf("Connection = %d, want %d", entry.Connection, 123)
	}
	if entry.ConnectionRequests != 1 {
		t.Errorf("ConnectionRequests = %d, want %d", entry.ConnectionRequests, 1)
	}
}

func TestParseLine_DashUpstream(t *testing.T) {
	line := `10.0.0.1 - - [15/Mar/2026:08:30:00 +0000] "POST /submit HTTP/1.1" 502 0 1.500 - - MISS 128 999 3`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry, got nil")
	}

	if entry.UpstreamResponseTime != -1 {
		t.Errorf("UpstreamResponseTime = %f, want %f (dash should yield -1)", entry.UpstreamResponseTime, -1.0)
	}
	if entry.UpstreamStatus != 0 {
		t.Errorf("UpstreamStatus = %d, want %d (dash should yield 0)", entry.UpstreamStatus, 0)
	}
	// Verify other fields still parse correctly alongside dashes
	if entry.UpstreamCacheStatus != "MISS" {
		t.Errorf("UpstreamCacheStatus = %q, want %q", entry.UpstreamCacheStatus, "MISS")
	}
	if entry.RequestTime != 1.5 {
		t.Errorf("RequestTime = %f, want %f", entry.RequestTime, 1.5)
	}
}

func TestParseLine_DashCacheStatus(t *testing.T) {
	line := `192.168.1.100 - - [01/Jan/2026:00:00:00 +0000] "GET /health HTTP/2.0" 200 12 0.001 0.001 200 - 64 50 1`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry, got nil")
	}

	if entry.UpstreamCacheStatus != "" {
		t.Errorf("UpstreamCacheStatus = %q, want %q (dash should yield empty string)", entry.UpstreamCacheStatus, "")
	}
	// Verify upstream response time and status are still parsed when present
	if entry.UpstreamResponseTime != 0.001 {
		t.Errorf("UpstreamResponseTime = %f, want %f", entry.UpstreamResponseTime, 0.001)
	}
	if entry.UpstreamStatus != 200 {
		t.Errorf("UpstreamStatus = %d, want %d", entry.UpstreamStatus, 200)
	}
}

func TestParseLine_InvalidFormat(t *testing.T) {
	line := "this is garbage that does not match the log format at all"

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil entry for garbage input, got %+v", entry)
	}
}

func TestParseLine_EmptyString(t *testing.T) {
	entry, err := ParseLine("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil entry for empty string, got %+v", entry)
	}
}

func TestParseLine_PartialLine(t *testing.T) {
	// Truncated line — missing the last several fields
	line := `203.0.113.45 - - [08/Feb/2026:12:01:00 +0000] "GET /api/data HTTP/1.1" 200`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil entry for partial line, got %+v", entry)
	}
}

func TestParseLine_LargeBodyBytes(t *testing.T) {
	// 5 GB body — tests int64 parsing well beyond int32 range
	line := `10.0.0.1 - - [01/Jun/2026:10:00:00 +0000] "GET /download/large HTTP/1.1" 200 5368709120 2.500 1.200 200 MISS 256 400 1`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry, got nil")
	}

	var want int64 = 5368709120 // 5 * 1024^3
	if entry.BodyBytesSent != want {
		t.Errorf("BodyBytesSent = %d, want %d", entry.BodyBytesSent, want)
	}
}

func TestParseLine_ZeroLatency(t *testing.T) {
	line := `10.0.0.1 - - [01/Jun/2026:10:00:00 +0000] "GET /cached HTTP/1.1" 304 0 0.000 0.000 304 HIT 100 200 5`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry, got nil")
	}

	if entry.RequestTime != 0.0 {
		t.Errorf("RequestTime = %f, want 0.000", entry.RequestTime)
	}
	if entry.UpstreamResponseTime != 0.0 {
		t.Errorf("UpstreamResponseTime = %f, want 0.000", entry.UpstreamResponseTime)
	}
}

func TestParseLine_TimestampParsing(t *testing.T) {
	line := `10.0.0.1 - - [25/Dec/2025:23:59:59 +0530] "GET / HTTP/1.1" 200 100 0.010 0.005 200 - 50 10 1`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry, got nil")
	}

	// +0530 is IST (India Standard Time), 5 hours 30 minutes ahead of UTC
	loc := time.FixedZone("IST", 5*3600+30*60)
	expected := time.Date(2025, time.December, 25, 23, 59, 59, 0, loc)

	if !entry.Timestamp.Equal(expected) {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, expected)
	}

	// Also verify the UTC conversion is correct
	expectedUTC := time.Date(2025, time.December, 25, 18, 29, 59, 0, time.UTC)
	if !entry.Timestamp.UTC().Equal(expectedUTC) {
		t.Errorf("Timestamp.UTC() = %v, want %v", entry.Timestamp.UTC(), expectedUTC)
	}
}

func TestParseLine_IPv6Address(t *testing.T) {
	line := `2001:db8::1 - - [10/Mar/2026:14:30:00 +0000] "DELETE /api/resource/42 HTTP/1.1" 204 0 0.050 0.045 204 - 128 300 2`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry, got nil")
	}

	if entry.RemoteAddr != "2001:db8::1" {
		t.Errorf("RemoteAddr = %q, want %q", entry.RemoteAddr, "2001:db8::1")
	}
	if entry.Method != "DELETE" {
		t.Errorf("Method = %q, want %q", entry.Method, "DELETE")
	}
	if entry.Path != "/api/resource/42" {
		t.Errorf("Path = %q, want %q", entry.Path, "/api/resource/42")
	}
	if entry.Status != 204 {
		t.Errorf("Status = %d, want %d", entry.Status, 204)
	}
}
