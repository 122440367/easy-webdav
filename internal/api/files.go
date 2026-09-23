package api

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lecritus/easy-webdav/internal/auth"
	"github.com/lecritus/easy-webdav/internal/store"
)

type Target struct {
	User       store.User
	Root       string
	Permission string
}

func (a *AuthAPI) UploadRoot(r *http.Request) (string, error) {
	t, err := a.ResolveTarget(r)
	if err != nil {
		return "", err
	}
	return t.Root, nil
}
func (a *AuthAPI) UploadQuota(r *http.Request) (string, int64, int64, error) {
	t, err := a.ResolveTarget(r)
	if err != nil {
		return "", 0, 0, err
	}
	used, _ := a.Store.Usage(r.Context(), filepath.Join(a.StorageDir, t.User.RootDir))
	return t.Root, t.User.Quota, used, nil
}
func UploadReadOnly(r *http.Request) bool {
	t, ok := userFromRequest(r)
	return !ok || t.Permission == "read"
}

func (a *AuthAPI) ResolveTarget(r *http.Request) (Target, error) {
	session, ok := userFromRequest(r)
	if !ok {
		return Target{}, os.ErrPermission
	}
	target := session
	if id := r.URL.Query().Get("user_id"); id != "" {
		if session.Role != "admin" {
			return Target{}, os.ErrPermission
		}
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return Target{}, os.ErrNotExist
		}
		target, err = a.Store.UserByID(r.Context(), parsed)
		if err != nil {
			return Target{}, err
		}
		target.Permission = "readwrite"
	}
	root := filepath.Join(a.StorageDir, target.RootDir)
	return Target{User: target, Root: root, Permission: target.Permission}, nil
}
func safeTargetPath(root, value string) (string, error) {
	value = strings.TrimPrefix(value, "/")
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." {
		clean = ""
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", os.ErrNotExist
	}
	p := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", os.ErrNotExist
	}
	return p, nil
}

func (a *AuthAPI) Files(w http.ResponseWriter, r *http.Request) {
	target, err := a.ResolveTarget(r)
	if err != nil {
		auth.WriteError(w, 403, "FORBIDDEN", "access denied", nil)
		return
	}
	p, err := safeTargetPath(target.Root, r.URL.Query().Get("path"))
	if err != nil {
		auth.WriteError(w, 400, "INVALID_PATH", "invalid path", nil)
		return
	}
	info, err := os.Stat(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !info.IsDir() {
		auth.WriteError(w, 400, "NOT_DIRECTORY", "path is not a directory", nil)
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		auth.WriteError(w, 500, "STORAGE_ERROR", err.Error(), nil)
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		item, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, map[string]any{"name": entry.Name(), "directory": entry.IsDir(), "size": item.Size(), "modified": item.ModTime()})
	}
	writeJSON(w, 200, map[string]any{"path": r.URL.Query().Get("path"), "entries": out})
}

func (a *AuthAPI) FileAction(w http.ResponseWriter, r *http.Request) {
	target, err := a.ResolveTarget(r)
	if err != nil {
		auth.WriteError(w, 403, "FORBIDDEN", "access denied", nil)
		return
	}
	if target.Permission == "read" {
		auth.WriteError(w, 403, "READ_ONLY", "write access denied", nil)
		return
	}
	var input struct {
		Path        string `json:"path"`
		Destination string `json:"destination"`
		Action      string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	source, err := safeTargetPath(target.Root, input.Path)
	if err != nil {
		auth.WriteError(w, 400, "INVALID_PATH", "invalid path", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if err = os.MkdirAll(source, 0750); err != nil {
			auth.WriteError(w, 500, "STORAGE_ERROR", err.Error(), nil)
			return
		}
	case http.MethodDelete:
		if err = os.RemoveAll(source); err != nil {
			auth.WriteError(w, 500, "STORAGE_ERROR", err.Error(), nil)
			return
		}
	case http.MethodPut:
		destination, err := safeTargetPath(target.Root, input.Destination)
		if err != nil {
			auth.WriteError(w, 400, "INVALID_PATH", "invalid destination", nil)
			return
		}
		if input.Action == "copy" {
			err = copyPath(source, destination)
		} else {
			err = os.Rename(source, destination)
		}
		if err != nil {
			auth.WriteError(w, 500, "STORAGE_ERROR", err.Error(), nil)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func copyPath(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.Walk(source, func(name string, item os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, _ := filepath.Rel(source, name)
			dst := filepath.Join(destination, rel)
			if item.IsDir() {
				return os.MkdirAll(dst, item.Mode())
			}
			in, e := os.Open(name)
			if e != nil {
				return e
			}
			defer in.Close()
			out, e := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, item.Mode())
			if e != nil {
				return e
			}
			_, e = io.Copy(out, in)
			ce := out.Close()
			if e != nil {
				return e
			}
			return ce
		})
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	ce := out.Close()
	if err != nil {
		return err
	}
	return ce
}

func (a *AuthAPI) Download(w http.ResponseWriter, r *http.Request) {
	target, err := a.ResolveTarget(r)
	if err != nil {
		auth.WriteError(w, 403, "FORBIDDEN", "access denied", nil)
		return
	}
	p, err := safeTargetPath(target.Root, r.URL.Query().Get("path"))
	if err != nil {
		auth.WriteError(w, 400, "INVALID_PATH", "invalid path", nil)
		return
	}
	info, err := os.Stat(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if info.IsDir() {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="download.zip"`)
		zw := zip.NewWriter(w)
		_ = filepath.Walk(p, func(name string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(p, name)
			entry, err := zw.Create(filepath.ToSlash(rel))
			if err != nil {
				return err
			}
			f, err := os.Open(name)
			if err != nil {
				return err
			}
			_, err = io.Copy(entry, f)
			_ = f.Close()
			return err
		})
		_ = zw.Close()
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+info.Name()+`"`)
	http.ServeFile(w, r, p)
}

func (a *AuthAPI) Raw(w http.ResponseWriter, r *http.Request) {
	target, err := a.ResolveTarget(r)
	if err != nil {
		auth.WriteError(w, 403, "FORBIDDEN", "access denied", nil)
		return
	}
	p, err := safeTargetPath(target.Root, r.URL.Query().Get("path"))
	if err != nil {
		auth.WriteError(w, 400, "INVALID_PATH", "invalid path", nil)
		return
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if info.Size() > 2*1024*1024 && strings.HasPrefix(mimeType(p), "text/") {
		auth.WriteError(w, 413, "FILE_TOO_LARGE", "text preview is limited to 2 MB", nil)
		return
	}
	kind := mimeType(p)
	if strings.HasSuffix(strings.ToLower(p), ".html") || strings.HasSuffix(strings.ToLower(p), ".htm") {
		kind = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, p)
}
func mimeType(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".txt", ".md", ".go", ".json", ".js", ".ts", ".css", ".xml":
		return "text/plain; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".pdf":
		return "application/pdf"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	}
	return "application/octet-stream"
}
