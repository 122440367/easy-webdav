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

	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/store"
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
	if !ok {
		return true
	}
	if r.URL.Query().Get("user_id") != "" && t.Role == "admin" {
		return false
	}
	return t.Permission == "read"
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
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absoluteTarget := filepath.Join(absoluteRoot, clean)
	if !withinPath(absoluteRoot, absoluteTarget) {
		return "", os.ErrNotExist
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", err
	}
	resolvedTarget, err := filepath.EvalSymlinks(absoluteTarget)
	if err == nil {
		if !withinPath(resolvedRoot, resolvedTarget) {
			return "", os.ErrNotExist
		}
		return resolvedTarget, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absoluteTarget))
	if err != nil || !withinPath(resolvedRoot, resolvedParent) {
		return "", os.ErrNotExist
	}
	return absoluteTarget, nil
}

func withinPath(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
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
	usageRoot := filepath.Join(a.StorageDir, target.User.RootDir)
	switch r.Method {
	case http.MethodPost:
		if err = os.MkdirAll(source, 0750); err != nil {
			auth.WriteError(w, 500, "STORAGE_ERROR", err.Error(), nil)
			return
		}
	case http.MethodDelete:
		bytes, err := pathBytes(source)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err = os.RemoveAll(source); err != nil {
			auth.WriteError(w, 500, "STORAGE_ERROR", err.Error(), nil)
			return
		}
		if err = a.Store.AddUsage(r.Context(), usageRoot, -bytes); err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
	case http.MethodPut:
		destination, err := safeTargetPath(target.Root, input.Destination)
		if err != nil {
			auth.WriteError(w, 400, "INVALID_PATH", "invalid destination", nil)
			return
		}
		used, err := a.Store.Usage(r.Context(), usageRoot)
		if err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
		var sourceBytes, destinationBytes int64
		if input.Action == "copy" {
			sourceBytes, err = pathBytes(source)
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		if input.Action != "copy" && filepath.Clean(source) == filepath.Clean(destination) {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if _, statErr := os.Stat(destination); statErr == nil {
			destinationBytes, err = pathBytes(destination)
			if err != nil {
				auth.WriteError(w, 500, "STORAGE_ERROR", err.Error(), nil)
				return
			}
		} else if !os.IsNotExist(statErr) {
			auth.WriteError(w, 500, "STORAGE_ERROR", statErr.Error(), nil)
			return
		}
		delta := sourceBytes - destinationBytes
		if input.Action != "copy" {
			delta = -destinationBytes
		}
		if target.User.Quota > 0 && used+delta > target.User.Quota {
			auth.WriteError(w, 413, "QUOTA_EXCEEDED", "quota exceeded", map[string]int64{"remaining": max(target.User.Quota-used+destinationBytes, 0)})
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
		if err = a.Store.AddUsage(r.Context(), usageRoot, delta); err != nil {
			auth.WriteError(w, 500, "STORE_ERROR", err.Error(), nil)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func pathBytes(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
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
	requested := r.URL.Query()["path"]
	if len(requested) == 0 || len(requested) == 1 && requested[0] == "" {
		requested = []string{""}
	}
	type downloadItem struct {
		path string
		info os.FileInfo
	}
	items := make([]downloadItem, 0, len(requested))
	for _, name := range requested {
		p, err := safeTargetPath(target.Root, name)
		if err != nil {
			auth.WriteError(w, 400, "INVALID_PATH", "invalid path", nil)
			return
		}
		info, err := os.Stat(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		items = append(items, downloadItem{path: p, info: info})
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if len(items) > 1 || items[0].info.IsDir() {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="download.zip"`)
		zw := zip.NewWriter(w)
		for _, item := range items {
			rootName := item.info.Name()
			_ = filepath.Walk(item.path, func(name string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				rel, err := filepath.Rel(item.path, name)
				if err != nil {
					return err
				}
				zipName := rootName
				if item.info.IsDir() {
					zipName = filepath.Join(rootName, rel)
				}
				entry, err := zw.Create(filepath.ToSlash(zipName))
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
		}
		_ = zw.Close()
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+items[0].info.Name()+`"`)
	http.ServeFile(w, r, items[0].path)
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
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if info.Size() > 2*1024*1024 && strings.HasPrefix(mimeType(p), "text/") {
		auth.WriteError(w, 413, "FILE_TOO_LARGE", "text preview is limited to 2 MB", nil)
		return
	}
	kind := mimeType(p)
	disposition := "attachment"
	if strings.HasSuffix(strings.ToLower(p), ".html") || strings.HasSuffix(strings.ToLower(p), ".htm") {
		// HTML is never rendered from the panel origin; the preview layer
		// shows it as highlighted source instead.
		kind = "text/plain; charset=utf-8"
	} else if inlineTypes[kind] {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", kind)
	if kind == "image/svg+xml" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
	w.Header().Set("Content-Disposition", disposition)
	http.ServeFile(w, r, p)
}

// inlineTypes is the preview whitelist: everything outside it is served as a
// download so the panel can never be used to render arbitrary content.
var inlineTypes = map[string]bool{
	"text/plain; charset=utf-8": true,
	"image/png":                 true,
	"image/jpeg":                true,
	"image/gif":                 true,
	"image/webp":                true,
	"image/svg+xml":             true,
	"image/bmp":                 true,
	"image/avif":                true,
	"application/pdf":           true,
	"video/mp4":                 true,
	"video/webm":                true,
	"video/ogg":                 true,
	"video/quicktime":           true,
	"audio/mpeg":                true,
	"audio/wav":                 true,
	"audio/ogg":                 true,
	"audio/flac":                true,
	"audio/mp4":                 true,
	"audio/webm":                true,
}

func mimeType(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".txt", ".md", ".go", ".json", ".js", ".mjs", ".ts", ".tsx", ".css", ".xml",
		".log", ".yaml", ".yml", ".toml", ".ini", ".conf", ".csv", ".sh", ".bash",
		".py", ".rb", ".java", ".c", ".h", ".cpp", ".hpp", ".rs", ".sql", ".vue",
		".env", ".gitignore", ".dockerfile":
		return "text/plain; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".avif":
		return "image/avif"
	case ".pdf":
		return "application/pdf"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".ogv":
		return "video/ogg"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".m4a":
		return "audio/mp4"
	}
	return "application/octet-stream"
}
