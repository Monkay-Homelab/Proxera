package settings

import (
	"testing"
	"time"

	"github.com/proxera/backend/internal/database"
)

func TestGet_DefaultWhenNoDB(t *testing.T) {
	// database.DB is nil by default in test context (no connection established).
	// With no env var set, Get should return the provided default value.
	if database.DB != nil {
		t.Fatal("expected database.DB to be nil in test context")
	}

	got := Get("TEST_SETTING_NONEXISTENT", "my-default")
	if got != "my-default" {
		t.Errorf("Get() = %q, want %q", got, "my-default")
	}
}

func TestGet_EnvVarOverridesDefault(t *testing.T) {
	// When database.DB is nil but an env var is set, the env var takes precedence
	// over the default value.
	t.Setenv("TEST_SETTING_FROM_ENV", "env-value")

	got := Get("TEST_SETTING_FROM_ENV", "fallback-default")
	if got != "env-value" {
		t.Errorf("Get() = %q, want %q", got, "env-value")
	}
}

func TestGet_EmptyEnvVarFallsToDefault(t *testing.T) {
	// An empty env var is treated the same as unset — the default should be returned.
	t.Setenv("TEST_SETTING_EMPTY", "")

	got := Get("TEST_SETTING_EMPTY", "should-use-default")
	if got != "should-use-default" {
		t.Errorf("Get() = %q, want %q", got, "should-use-default")
	}
}

func TestCache_HitReturnsCachedValue(t *testing.T) {
	// Manually populate the cache and verify Get() returns the cached value
	// without needing a database connection.
	cache.set("CACHED_KEY", "cached-value")
	t.Cleanup(func() { cache.invalidate("CACHED_KEY") })

	got := Get("CACHED_KEY", "default-value")
	if got != "cached-value" {
		t.Errorf("Get() = %q, want %q (expected cache hit)", got, "cached-value")
	}
}

func TestCache_MissQueriesDbOrFallback(t *testing.T) {
	// With no cache entry and no DB, Get() should fall through to the default.
	cache.invalidate("UNCACHED_KEY")

	got := Get("UNCACHED_KEY", "fallback")
	if got != "fallback" {
		t.Errorf("Get() = %q, want %q (expected cache miss -> default)", got, "fallback")
	}
}

func TestCache_ExpiredEntryIsNotReturned(t *testing.T) {
	// Manually insert a cache entry that is already expired.
	cache.mu.Lock()
	cache.entries["EXPIRED_KEY"] = cachedEntry{
		value:     "stale-value",
		expiresAt: time.Now().Add(-1 * time.Second),
	}
	cache.mu.Unlock()
	t.Cleanup(func() { cache.invalidate("EXPIRED_KEY") })

	got := Get("EXPIRED_KEY", "fresh-default")
	if got != "fresh-default" {
		t.Errorf("Get() = %q, want %q (expected expired entry to be skipped)", got, "fresh-default")
	}
}

func TestInvalidate_RemovesKey(t *testing.T) {
	cache.set("TO_INVALIDATE", "some-value")

	// Verify it is cached.
	if val, ok := cache.get("TO_INVALIDATE"); !ok || val != "some-value" {
		t.Fatal("expected cache entry to exist before invalidation")
	}

	Invalidate("TO_INVALIDATE")

	if _, ok := cache.get("TO_INVALIDATE"); ok {
		t.Error("expected cache entry to be removed after Invalidate()")
	}
}

func TestInvalidateAll_ClearsEntireCache(t *testing.T) {
	cache.set("KEY_A", "val-a")
	cache.set("KEY_B", "val-b")

	InvalidateAll()

	if _, ok := cache.get("KEY_A"); ok {
		t.Error("expected KEY_A to be cleared after InvalidateAll()")
	}
	if _, ok := cache.get("KEY_B"); ok {
		t.Error("expected KEY_B to be cleared after InvalidateAll()")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	// Verify that concurrent reads and writes do not cause data races.
	// Run with -race to detect issues.
	const iterations = 100
	done := make(chan struct{})

	// Writer goroutine.
	go func() {
		for i := 0; i < iterations; i++ {
			cache.set("RACE_KEY", "value")
			cache.invalidate("RACE_KEY")
		}
		done <- struct{}{}
	}()

	// Reader goroutine.
	go func() {
		for i := 0; i < iterations; i++ {
			cache.get("RACE_KEY")
		}
		done <- struct{}{}
	}()

	// InvalidateAll goroutine.
	go func() {
		for i := 0; i < iterations; i++ {
			cache.invalidateAll()
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	<-done
}
