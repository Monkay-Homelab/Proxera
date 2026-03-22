package settings

import (
	"sync"
	"time"
)

// cacheTTL is the time-to-live for cached settings entries.
const cacheTTL = 30 * time.Second

// cachedEntry holds a cached value and its expiration time.
type cachedEntry struct {
	value     string
	expiresAt time.Time
}

// settingsCache provides an in-memory cache for settings values,
// reducing database round-trips for frequently accessed keys.
type settingsCache struct {
	mu      sync.RWMutex
	entries map[string]cachedEntry
}

// cache is the package-level singleton used by Get().
var cache = settingsCache{
	entries: make(map[string]cachedEntry),
}

// get returns the cached value for key if it exists and has not expired.
// The second return value indicates whether a valid cache entry was found.
func (c *settingsCache) get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.value, true
}

// set stores a value in the cache with the standard TTL.
func (c *settingsCache) set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cachedEntry{
		value:     value,
		expiresAt: time.Now().Add(cacheTTL),
	}
}

// invalidate removes a single key from the cache.
func (c *settingsCache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// invalidateAll clears the entire cache.
func (c *settingsCache) invalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]cachedEntry)
}

// Invalidate removes a single key from the settings cache, forcing
// the next Get() call for that key to query the database.
func Invalidate(key string) {
	cache.invalidate(key)
}

// InvalidateAll clears the entire settings cache, forcing all
// subsequent Get() calls to query the database.
func InvalidateAll() {
	cache.invalidateAll()
}
