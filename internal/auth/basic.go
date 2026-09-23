package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/lecritus/easy-webdav/internal/store"
)

type basicEntry struct {
	UserID   int64
	Username string
	Expires  time.Time
}
type BasicAuthenticator struct {
	Store   *store.Store
	Limiter *LoginLimiter
	mu      sync.Mutex
	cache   map[string]basicEntry
	TTL     time.Duration
}

func NewBasicAuthenticator(s *store.Store) *BasicAuthenticator {
	return &BasicAuthenticator{Store: s, Limiter: NewLoginLimiter(), cache: map[string]basicEntry{}, TTL: 60 * time.Second}
}
func (a *BasicAuthenticator) ClearUser(id int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for key, item := range a.cache {
		if item.UserID == id {
			delete(a.cache, key)
		}
	}
}
func (a *BasicAuthenticator) Authenticate(r *http.Request) (store.User, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return store.User{}, false
	}
	keyBytes := sha256.Sum256([]byte(username + ":" + password))
	key := hex.EncodeToString(keyBytes[:])
	a.mu.Lock()
	item, found := a.cache[key]
	if found && time.Now().Before(item.Expires) {
		a.mu.Unlock()
		u, err := a.Store.UserByID(r.Context(), item.UserID)
		return u, err == nil && !u.Disabled
	}
	if found {
		delete(a.cache, key)
	}
	a.mu.Unlock()
	u, err := a.Store.UserByName(r.Context(), username)
	if err != nil || u.Disabled || !CheckPassword(u.PasswordHash, password) {
		return store.User{}, false
	}
	a.mu.Lock()
	a.cache[key] = basicEntry{UserID: u.ID, Username: u.Username, Expires: time.Now().Add(a.TTL)}
	a.mu.Unlock()
	return u, true
}
func (a *BasicAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Limiter != nil {
			if allowed, retry := a.Limiter.Allow(Meta(r).IP); !allowed {
				RateLimited(w, retry)
				return
			}
		}
		u, ok := a.Authenticate(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="easy-webdav"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(contextWithUser(r.Context(), u)))
	})
}
