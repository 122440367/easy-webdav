package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lecritus/easy-webdav/internal/auth"
	"github.com/lecritus/easy-webdav/internal/store"
)

type userInput struct {
	Username   string  `json:"username"`
	Password   string  `json:"password"`
	RootDir    *string `json:"root_dir"`
	Permission string  `json:"permission"`
	Quota      int64   `json:"quota"`
	Disabled   bool    `json:"disabled"`
}

func (a *AuthAPI) Users(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromRequest(r)
	if !ok || u.Role != "admin" {
		auth.WriteError(w, 403, "ADMIN_REQUIRED", "administrator role required", nil)
		return
	}
	if r.Method == http.MethodGet {
		users, err := a.Store.ListUsers(r.Context())
		if err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
		out := make([]map[string]any, 0, len(users))
		for _, item := range users {
			out = append(out, publicUser(item))
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var input userInput
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		auth.WriteError(w, 400, "INVALID_JSON", "invalid request", nil)
		return
	}
	if err := ValidateUsername(input.Username); err != nil {
		auth.WriteError(w, 400, "INVALID_USERNAME", err.Error(), nil)
		return
	}
	// An omitted root_dir defaults to the username; an explicitly invalid
	// value (empty, ".", absolute, escaping) is rejected per spec.
	root := path.Clean(strings.ReplaceAll(input.Username, `\`, "/"))
	if input.RootDir != nil {
		var err error
		root, err = ValidateRootDir(*input.RootDir)
		if err != nil {
			auth.WriteError(w, 400, "INVALID_ROOT_DIR", err.Error(), nil)
			return
		}
	}
	if input.Permission != "read" && input.Permission != "readwrite" {
		input.Permission = ""
	}
	settings, _ := a.Store.Settings(r.Context())
	if input.Permission == "" {
		input.Permission = settings["default_permission"]
		if input.Permission != "read" && input.Permission != "readwrite" {
			input.Permission = "readwrite"
		}
	}
	if input.Quota < 0 {
		auth.WriteError(w, 400, "INVALID_QUOTA", "quota must be zero or greater", nil)
		return
	}
	if input.Quota == 0 && settings["default_quota"] != "" {
		input.Quota, _ = strconv.ParseInt(settings["default_quota"], 10, 64)
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		auth.WriteError(w, 400, "WEAK_PASSWORD", err.Error(), nil)
		return
	}
	if err = os.MkdirAll(filepath.Join(a.StorageDir, root), 0750); err != nil {
		auth.WriteError(w, 500, "ROOT_CREATE_FAILED", err.Error(), nil)
		return
	}
	id, err := a.Store.CreateUser(r.Context(), store.User{Username: input.Username, PasswordHash: hash, Role: "user", RootDir: root, Permission: input.Permission, Quota: input.Quota})
	if err != nil {
		auth.WriteError(w, 409, "USER_EXISTS", "username already exists", nil)
		return
	}
	created, _ := a.Store.UserByID(r.Context(), id)
	writeJSON(w, 201, publicUser(created))
}

func (a *AuthAPI) UserByPath(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromRequest(r)
	if !ok || u.Role != "admin" {
		auth.WriteError(w, 403, "ADMIN_REQUIRED", "administrator role required", nil)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/users/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	target, err := a.Store.UserByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "password" && r.Method == http.MethodPost {
		var input struct {
			Password string `json:"password"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			auth.WriteError(w, 400, "INVALID_JSON", "invalid request", nil)
			return
		}
		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			auth.WriteError(w, 400, "WEAK_PASSWORD", err.Error(), nil)
			return
		}
		if err = a.Store.UpdateUserPassword(r.Context(), id, hash); err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
		_ = a.Sessions.RevokeUser(r.Context(), id)
		if a.Basic != nil {
			a.Basic.ClearUser(id)
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if len(parts) > 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, publicUser(target))
	case http.MethodDelete:
		if target.Role == "admin" {
			count, _ := a.Store.AdminCount(r.Context())
			if count <= 1 {
				auth.WriteError(w, 409, "LAST_ADMIN", "cannot remove the last administrator", nil)
				return
			}
		}
		_ = a.Sessions.RevokeUser(r.Context(), id)
		if err = a.Store.DeleteUser(r.Context(), id); err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case http.MethodPut:
		var input userInput
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			auth.WriteError(w, 400, "INVALID_JSON", "invalid request", nil)
			return
		}
		if input.Username == "" {
			input.Username = target.Username
		}
		if err := ValidateUsername(input.Username); err != nil {
			auth.WriteError(w, 400, "INVALID_USERNAME", err.Error(), nil)
			return
		}
		root := target.RootDir
		if input.RootDir != nil {
			var err error
			root, err = ValidateRootDir(*input.RootDir)
			if err != nil {
				auth.WriteError(w, 400, "INVALID_ROOT_DIR", err.Error(), nil)
				return
			}
		}
		if input.Permission == "" {
			input.Permission = target.Permission
		}
		if input.Permission != "read" && input.Permission != "readwrite" {
			auth.WriteError(w, 400, "INVALID_PERMISSION", "invalid permission", nil)
			return
		}
		if target.Role == "admin" && input.Disabled {
			count, _ := a.Store.AdminCount(r.Context())
			if count <= 1 {
				auth.WriteError(w, 409, "LAST_ADMIN", "cannot disable the last administrator", nil)
				return
			}
		}
		if err = a.Store.UpdateUser(r.Context(), id, input.Username, root, input.Permission, input.Quota, input.Disabled); err != nil {
			auth.WriteError(w, 409, "USER_EXISTS", err.Error(), nil)
			return
		}
		if input.Disabled {
			_ = a.Sessions.RevokeUser(r.Context(), id)
			if a.Basic != nil {
				a.Basic.ClearUser(id)
			}
		}
		updated, _ := a.Store.UserByID(r.Context(), id)
		writeJSON(w, 200, publicUser(updated))
	default:
		http.NotFound(w, r)
	}
}

func (a *AuthAPI) Settings(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromRequest(r)
	if !ok {
		auth.WriteError(w, 401, "UNAUTHENTICATED", "authentication required", nil)
		return
	}
	if r.Method == http.MethodGet {
		values, err := a.Store.Settings(r.Context())
		if err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
		writeJSON(w, 200, values)
		return
	}
	if u.Role != "admin" {
		auth.WriteError(w, 403, "ADMIN_REQUIRED", "administrator role required", nil)
		return
	}
	var values map[string]string
	if json.NewDecoder(r.Body).Decode(&values) != nil {
		auth.WriteError(w, 400, "INVALID_JSON", "invalid request", nil)
		return
	}
	for key, value := range values {
		if key != "default_quota" && key != "default_permission" && key != "site_name" {
			auth.WriteError(w, 400, "INVALID_SETTING", "unknown setting", map[string]string{"key": key})
			return
		}
		if key == "default_permission" && value != "read" && value != "readwrite" {
			auth.WriteError(w, 400, "INVALID_SETTING", "invalid default permission", nil)
			return
		}
		if err := a.Store.SetSetting(r.Context(), key, value); err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
	}
	writeJSON(w, 200, values)
}
