package notifications

import (
	"fmt"
	"sync"
	"time"
)

var (
	cooldowns   = make(map[string]time.Time)
	cooldownsMu sync.Mutex
)

// CooldownKey generates a unique key for deduplication.
// Format: "userID:alertType:qualifier"
func CooldownKey(userID int, alertType, qualifier string) string {
	return fmt.Sprintf("%d:%s:%s", userID, alertType, qualifier)
}

// InCooldown returns true if the key was triggered within cooldownMinutes.
func InCooldown(key string, cooldownMinutes int) bool {
	cooldownsMu.Lock()
	defer cooldownsMu.Unlock()
	if last, ok := cooldowns[key]; ok {
		if time.Since(last) < time.Duration(cooldownMinutes)*time.Minute {
			return true
		}
	}
	return false
}

// SetCooldown marks the key as triggered now.
func SetCooldown(key string) {
	cooldownsMu.Lock()
	cooldowns[key] = time.Now()
	cooldownsMu.Unlock()
}

// CheckAndSetCooldown atomically checks if a key is in cooldown and sets it if not.
// Returns true if the alert should be suppressed (in cooldown), false if it should fire.
func CheckAndSetCooldown(key string, cooldownMinutes int) bool {
	cooldownsMu.Lock()
	defer cooldownsMu.Unlock()
	if last, ok := cooldowns[key]; ok {
		if time.Since(last) < time.Duration(cooldownMinutes)*time.Minute {
			return true
		}
	}
	cooldowns[key] = time.Now()
	return false
}

// StartCooldownCleanup removes stale entries every 10 minutes.
func StartCooldownCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-24 * time.Hour)
		cooldownsMu.Lock()
		for k, v := range cooldowns {
			if v.Before(cutoff) {
				delete(cooldowns, k)
			}
		}
		cooldownsMu.Unlock()
	}
}
