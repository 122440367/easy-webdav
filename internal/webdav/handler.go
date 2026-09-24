package webdav

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/store"
	upstream "github.com/122440367/easy-webdav/internal/webdav/xnet"
)

type Service struct {
	StorageRoot string
	Store       *store.Store
	Basic       *auth.BasicAuthenticator
	Locks       upstream.LockSystem
}

func NewService(storageRoot string, db *store.Store) *Service {
	return &Service{StorageRoot: storageRoot, Store: db, Basic: auth.NewBasicAuthenticator(db), Locks: upstream.NewMemLS()}
}
func (s *Service) Handler() http.Handler {
	return s.Basic.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("MS-Author-Via", "DAV")
		}
		if r.Method == "PROPFIND" && strings.EqualFold(r.Header.Get("Depth"), "infinity") {
			http.Error(w, "infinite depth is not supported", http.StatusForbidden)
			return
		}
		u, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		// Read-only users are rejected before the request reaches the
		// protocol layer so every write method reports a clean 403.
		if u.Permission == "read" {
			switch r.Method {
			case "PUT", "DELETE", "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK":
				http.Error(w, "write access denied", http.StatusForbidden)
				return
			}
		}
		if r.Method == "COPY" || r.Method == "MOVE" {
			if !destinationInside(r, filepath.Join(s.StorageRoot, u.RootDir)) {
				http.Error(w, "destination outside user root", http.StatusForbidden)
				return
			}
		}
		root := filepath.Join(s.StorageRoot, u.RootDir)
		fs, err := NewRestrictedFS(root, u.Permission == "read")
		if err != nil {
			http.Error(w, "storage unavailable", 500)
			return
		}
		used, err := s.Store.Usage(r.Context(), root)
		if err != nil {
			http.Error(w, "usage unavailable", 500)
			return
		}
		fs.SetQuota(u.Quota, used, filepath.Join(s.StorageRoot, ".ew-tmp"), func(delta int64) error {
			return s.Store.AddUsage(r.Context(), root, delta)
		})
		h := &upstream.Handler{Prefix: "/dav/", FileSystem: fs, LockSystem: scopedLocks{base: filepath.Clean(filepath.Join(s.StorageRoot, u.RootDir)), inner: s.Locks}}
		h.ServeHTTP(w, r)
	}))
}

func destinationInside(r *http.Request, storage string) bool {
	raw := strings.TrimSpace(r.Header.Get("Destination"))
	if raw == "" {
		return false
	}
	destination, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if destination.Host != "" && !strings.EqualFold(destination.Host, r.Host) {
		return false
	}
	if !strings.HasPrefix(destination.Path, "/dav/") {
		return false
	}
	root := filepath.Clean(storage)
	relative := filepath.FromSlash(strings.TrimPrefix(destination.Path, "/dav/"))
	target := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
