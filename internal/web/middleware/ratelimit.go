package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// cloudflareRanges are Cloudflare's published edge IP ranges (see
// https://www.cloudflare.com/ips-v4 and https://www.cloudflare.com/ips-v6).
// They're hard-coded rather than fetched at runtime to avoid making an
// external HTTP call a dependency of every request; Cloudflare updates these
// infrequently, but the list should be refreshed periodically against the
// URLs above.
var cloudflareRanges = mustParseCIDRs([]string{
	// IPv4
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	// IPv6
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
})

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("middleware: invalid Cloudflare CIDR " + c + ": " + err.Error())
		}
		nets = append(nets, n)
	}
	return nets
}

// isCloudflareIP reports whether ip falls within one of Cloudflare's
// published edge IP ranges.
func isCloudflareIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range cloudflareRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// remoteAddrIP returns the IP portion of r.RemoteAddr, stripping the port
// and any IPv6 brackets. Falls back to the raw value if it has no port.
func remoteAddrIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

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

// ClientIP extracts the real client IP from the request. CF-Connecting-IP
// and X-Forwarded-For are client-controllable headers, so they're only
// trusted when the request's immediate peer (r.RemoteAddr) is a known
// Cloudflare edge IP — otherwise anyone reaching the origin directly (or a
// hop before Cloudflare) could forge them to get a fresh rate-limit bucket
// on every request. When the peer isn't Cloudflare, RemoteAddr alone is
// used as the client IP.
func ClientIP(r *http.Request) string {
	peer := remoteAddrIP(r.RemoteAddr)

	if isCloudflareIP(net.ParseIP(peer)) {
		if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			if i := strings.Index(ip, ","); i != -1 {
				return strings.TrimSpace(ip[:i])
			}
			return strings.TrimSpace(ip)
		}
	}

	return peer
}

// TrustedProxyPeer reports whether the request's immediate peer
// (r.RemoteAddr) is a known Cloudflare edge IP — the same guard ClientIP
// uses before trusting a client-controllable forwarding header. Callers
// that trust another proxy-set header (e.g. X-Real-Host for site routing)
// should check this first for the same reason.
func TrustedProxyPeer(r *http.Request) bool {
	return isCloudflareIP(net.ParseIP(remoteAddrIP(r.RemoteAddr)))
}
