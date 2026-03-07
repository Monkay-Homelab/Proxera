package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type userWindow struct {
	Count   int
	ResetAt time.Time
}

var (
	windows   = make(map[int]*userWindow)
	windowsMu sync.Mutex
)

func init() {
	// Prune expired windows every 10 minutes
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			windowsMu.Lock()
			now := time.Now()
			for uid, w := range windows {
				if now.After(w.ResetAt) {
					delete(windows, uid)
				}
			}
			windowsMu.Unlock()
		}
	}()
}

// FlatRateLimit enforces a flat per-user API rate limit (10,000/hr). Admin role is unlimited.
func FlatRateLimit(c *fiber.Ctx) error {
	role, _ := c.Locals("user_role").(string)
	if role == "admin" {
		return c.Next()
	}

	userID, _ := c.Locals("user_id").(int)
	const limit = 10000

	windowsMu.Lock()
	w, ok := windows[userID]
	now := time.Now()
	if !ok || now.After(w.ResetAt) {
		w = &userWindow{Count: 0, ResetAt: now.Add(1 * time.Hour)}
		windows[userID] = w
	}
	w.Count++
	count := w.Count
	windowsMu.Unlock()

	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	c.Set("X-RateLimit-Limit", strconv.Itoa(limit))
	c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

	if count > limit {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error": "API rate limit exceeded.",
		})
	}
	return c.Next()
}
