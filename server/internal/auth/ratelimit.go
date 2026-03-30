package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	apphttp "remoteaccess/server/internal/http"
)

type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
	limit   int
	window  time.Duration
}

type rateEntry struct {
	count     int
	windowEnd time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string]*rateEntry),
		limit:   limit,
		window:  window,
	}
}

func (r *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ip := clientIP(req)
		if !r.allow(ip) {
			apphttp.WriteError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (r *RateLimiter) allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok || now.After(entry.windowEnd) {
		r.entries[key] = &rateEntry{count: 1, windowEnd: now.Add(r.window)}
		return true
	}

	if entry.count >= r.limit {
		return false
	}
	entry.count++
	return true
}

func clientIP(req *http.Request) string {
	if req == nil {
		return ""
	}
	if xff := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			return first
		}
	}
	if xri := strings.TrimSpace(req.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
