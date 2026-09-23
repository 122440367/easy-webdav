package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lecritus/easy-webdav/internal/auth"
	"github.com/lecritus/easy-webdav/internal/store"
)

type AuthAPI struct {
	Store      *store.Store
	Sessions   auth.Sessions
	Limiter    *auth.LoginLimiter
	StorageDir string
	Basic      *auth.BasicAuthenticator
}

func (a *AuthAPI) Bootstrap(ctx context.Context, username, password string) error {
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("EW_ADMIN_USER and EW_ADMIN_PASSWORD must be provided together")
	}
	count, err := a.Store.AdminCount(ctx)
	if err != nil || count > 0 {
		return err
	}
	if err := ValidateUsername(username); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(a.StorageDir, username), 0750); err != nil {
		return err
	}
	_, err = a.Store.CreateUser(ctx, store.User{Username: username, PasswordHash: hash, Role: "admin", RootDir: username, Permission: "readwrite"})
	return err
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *AuthAPI) Login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		auth.WriteError(w, 400, "INVALID_JSON", "invalid request", nil)
		return
	}
	meta := auth.Meta(r)
	if a.Limiter != nil {
		if ok, retry := a.Limiter.Allow(meta.IP); !ok {
			auth.RateLimited(w, retry)
			return
		}
	}
	u, err := a.Store.UserByName(r.Context(), input.Username)
	if err != nil || u.Disabled || !auth.CheckPassword(u.PasswordHash, input.Password) {
		auth.WriteError(w, 401, "INVALID_CREDENTIALS", "invalid username or password", nil)
		return
	}
	cookie, err := a.Sessions.Create(r.Context(), u.ID, meta.Protocol == "https")
	if err != nil {
		auth.WriteError(w, 500, "SESSION_CREATE_FAILED", "could not create session", nil)
		return
	}
	http.SetCookie(w, cookie)
	writeJSON(w, 200, map[string]any{"user": publicUser(u), "insecure": meta.Insecure})
}

func (a *AuthAPI) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = a.Sessions.Delete(r.Context(), c.Value)
	}
	http.SetCookie(w, auth.ClearCookie())
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *AuthAPI) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		auth.WriteError(w, 401, "UNAUTHENTICATED", "authentication required", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"user": publicUser(u), "insecure": auth.Meta(r).Insecure})
}

func (a *AuthAPI) Password(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		auth.WriteError(w, 401, "UNAUTHENTICATED", "authentication required", nil)
		return
	}
	var input struct{ CurrentPassword, Password string }
	if json.NewDecoder(r.Body).Decode(&input) != nil || !auth.CheckPassword(u.PasswordHash, input.CurrentPassword) {
		auth.WriteError(w, 400, "INVALID_PASSWORD", "current password is invalid", nil)
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		auth.WriteError(w, 400, "WEAK_PASSWORD", err.Error(), nil)
		return
	}
	if err = a.Store.UpdateUserPassword(r.Context(), u.ID, hash); err != nil {
		auth.WriteError(w, 500, "PASSWORD_UPDATE_FAILED", "could not update password", nil)
		return
	}
	_ = a.Sessions.RevokeUser(r.Context(), u.ID)
	if a.Basic != nil {
		a.Basic.ClearUser(u.ID)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *AuthAPI) SetupStatus(w http.ResponseWriter, r *http.Request) {
	count, err := a.Store.AdminCount(r.Context())
	if err != nil {
		auth.WriteError(w, 500, "STORE_ERROR", "could not check setup state", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"required": count == 0, "insecure": auth.Meta(r).Insecure})
}

func (a *AuthAPI) Setup(w http.ResponseWriter, r *http.Request) {
	count, err := a.Store.AdminCount(r.Context())
	if err != nil {
		auth.WriteError(w, 500, "STORE_ERROR", "could not check setup state", nil)
		return
	}
	if count > 0 {
		auth.WriteError(w, 403, "SETUP_LOCKED", "setup is already complete", nil)
		return
	}
	var input credentials
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		auth.WriteError(w, 400, "INVALID_JSON", "invalid request", nil)
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil || input.Username == "" {
		if err == nil {
			err = errors.New("username is required")
		}
		auth.WriteError(w, 400, "INVALID_SETUP", err.Error(), nil)
		return
	}
	root := filepath.Join(a.StorageDir, input.Username)
	if err = os.MkdirAll(root, 0750); err != nil {
		auth.WriteError(w, 500, "ROOT_CREATE_FAILED", "could not create user root", nil)
		return
	}
	id, err := a.Store.CreateUser(r.Context(), store.User{Username: input.Username, PasswordHash: hash, Role: "admin", RootDir: input.Username, Permission: "readwrite"})
	if err != nil {
		auth.WriteError(w, 409, "USER_EXISTS", "user already exists", nil)
		return
	}
	u, _ := a.Store.UserByID(r.Context(), id)
	cookie, _ := a.Sessions.Create(r.Context(), id, auth.Meta(r).Protocol == "https")
	http.SetCookie(w, cookie)
	writeJSON(w, 201, map[string]any{"user": publicUser(u)})
}

func (a *AuthAPI) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.currentUser(r)
		if !ok {
			auth.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", nil)
			return
		}
		next.ServeHTTP(w, WithUser(u, r))
	})
}

func (a *AuthAPI) currentUser(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(auth.SessionCookie)
	if err != nil {
		return store.User{}, false
	}
	u, err := a.Sessions.Lookup(r.Context(), c.Value)
	return u, err == nil
}
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := userFromRequest(r); !ok || u.Role != "admin" {
			auth.WriteError(w, 403, "ADMIN_REQUIRED", "administrator role required", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type userContextKey string

const currentUserKey userContextKey = "current-user"

func WithUser(u store.User, r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), currentUserKey, u))
}
func userFromRequest(r *http.Request) (store.User, bool) {
	u, ok := r.Context().Value(currentUserKey).(store.User)
	return u, ok
}
func publicUser(u store.User) map[string]any {
	return map[string]any{"id": u.ID, "username": u.Username, "role": u.Role, "root_dir": u.RootDir, "permission": u.Permission, "quota": u.Quota, "disabled": u.Disabled}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func userID(value int64) string { return strconv.FormatInt(value, 10) }
