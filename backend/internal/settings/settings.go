// Package settings provides a unified way to read configuration.
// Values are read from system_settings (DB) first, falling back to env vars.
package settings

import (
	"context"
	"os"
	"time"

	"github.com/proxera/backend/internal/database"
)

// Get returns a setting value. Checks the in-memory cache first, then DB,
// then env, then defaultVal. DB results are cached with a 30-second TTL.
func Get(key string, defaultVal string) string {
	// Check cache first.
	if val, ok := cache.get(key); ok {
		return val
	}

	if database.DB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var val string
		err := database.DB.QueryRow(ctx,
			`SELECT value FROM system_settings WHERE key = $1`, key,
		).Scan(&val)
		if err == nil && val != "" {
			cache.set(key, val)
			return val
		}
	}

	if v := os.Getenv(key); v != "" {
		return v
	}

	return defaultVal
}
