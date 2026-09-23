package auth

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

type limiterEntry struct {
	started time.Time
	count   int
}
type LoginLimiter struct {
	mu      sync.Mutex
	entries map[string]limiterEntry
	limit   int
	window  time.Duration
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{entries: map[string]limiterEntry{}, limit: 10, window: 5 * time.Minute}
}
func (l *LoginLimiter) Allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry := l.entries[ip]
	if entry.started.IsZero() || now.Sub(entry.started) >= l.window {
		l.entries[ip] = limiterEntry{started: now, count: 1}
		return true, 0
	}
	if entry.count >= l.limit {
		return false, l.window - now.Sub(entry.started)
	}
	entry.count++
	l.entries[ip] = entry
	return true, 0
}
func RateLimited(w http.ResponseWriter, retry time.Duration) {
	seconds := int(retry.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	http.Error(w, "too many requests", http.StatusTooManyRequests)
}

func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-Requested-With") != "" || r.Header.Get("X-CSRF-Token") != "" {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "csrf validation failed", http.StatusForbidden)
	})
}
