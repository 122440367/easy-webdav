package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/diskusage"
	"github.com/122440367/easy-webdav/internal/quota"
)

func (a *AuthAPI) Usage(w http.ResponseWriter, r *http.Request) {
	u, ok := userFromRequest(r)
	if !ok {
		auth.WriteError(w, 401, "UNAUTHENTICATED", "authentication required", nil)
		return
	}
	if r.URL.Path == "/api/v1/usage/recalculate" {
		if u.Role != "admin" {
			auth.WriteError(w, 403, "ADMIN_REQUIRED", "administrator role required", nil)
			return
		}
		if err := quota.Recalculate(a.Store, a.StorageDir); err != nil {
			auth.WriteError(w, 500, "RECALCULATE_FAILED", err.Error(), nil)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.URL.Path == "/api/v1/usage/me" {
		used, err := a.Store.Usage(r.Context(), filepath.Join(a.StorageDir, u.RootDir))
		if err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
		writeJSON(w, 200, map[string]any{"root_dir": u.RootDir, "used": used, "quota": u.Quota})
		return
	}
	if u.Role != "admin" {
		auth.WriteError(w, 403, "ADMIN_REQUIRED", "administrator role required", nil)
		return
	}
	users, err := a.Store.ListUsers(r.Context())
	if err != nil {
		auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(users))
	for _, item := range users {
		used, _ := a.Store.Usage(r.Context(), filepath.Join(a.StorageDir, item.RootDir))
		items = append(items, map[string]any{"user": publicUser(item), "used": used})
	}
	disk := map[string]any{"storage_dir": a.StorageDir}
	if _, err := os.Stat(a.StorageDir); err == nil {
		if free, total, err := diskusage.Free(a.StorageDir); err == nil {
			disk["free"] = free
			disk["total"] = total
		}
	}
	writeJSON(w, 200, map[string]any{"users": items, "disk": disk})
}
