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

// Allow reports whether an attempt from ip may proceed right now. It only
// inspects state; failures must be recorded with Fail and successes clear
// the streak with Reset.
func (l *LoginLimiter) Allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[ip]
	if entry.started.IsZero() || time.Since(entry.started) >= l.window || entry.count < l.limit {
		return true, 0
	}
	return false, l.window - time.Since(entry.started)
}

// Fail records a failed attempt, opening a new window when the previous one
// has expired.
func (l *LoginLimiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry := l.entries[ip]
	if entry.started.IsZero() || now.Sub(entry.started) >= l.window {
		l.entries[ip] = limiterEntry{started: now, count: 1}
		return
	}
	entry.count++
	l.entries[ip] = entry
}

// Reset clears the failure streak for ip after a successful authentication.
func (l *LoginLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
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
