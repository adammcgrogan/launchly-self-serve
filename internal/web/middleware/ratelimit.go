package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter is a fixed-window, in-memory rate limiter keyed by an
// arbitrary string. Stale windows are cleaned up periodically so memory
// doesn't grow unboundedly.
//
// State is per-process: each instance tracks its own counters, so if the
// app is ever scaled to more than one instance behind a load balancer, the
// effective limit becomes (limit * instance count) with no warning. This
// is a deliberate tradeoff for a single-instance deployment — if/when this
// app runs on multiple instances, replace this with a shared store (e.g.
// Redis or Postgres-backed) instead of adding synchronization here.
type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rateWindow
	limit   int
	window  time.Duration
}

type rateWindow struct {
	count int
	reset time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{windows: make(map[string]*rateWindow), limit: limit, window: window}
	go rl.cleanup()
	return rl
}

// Allow returns true if the key is within its rate limit, false if it should be rejected.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	w, ok := rl.windows[key]
	if !ok || now.After(w.reset) {
		rl.windows[key] = &rateWindow{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, w := range rl.windows {
			if now.After(w.reset) {
				delete(rl.windows, k)
			}
		}
		rl.mu.Unlock()
	}
}

// ClientIP extracts the real client IP from the request, respecting Cloudflare headers.
func ClientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if i := strings.Index(ip, ","); i != -1 {
			return strings.TrimSpace(ip[:i])
		}
		return strings.TrimSpace(ip)
	}
	if i := strings.LastIndex(r.RemoteAddr, ":"); i != -1 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}
