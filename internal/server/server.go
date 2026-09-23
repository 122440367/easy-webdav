package server

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/lecritus/easy-webdav/internal/api"
	"github.com/lecritus/easy-webdav/internal/auth"
	"github.com/lecritus/easy-webdav/internal/config"
	"github.com/lecritus/easy-webdav/internal/logging"
	"github.com/lecritus/easy-webdav/internal/quota"
	"github.com/lecritus/easy-webdav/internal/store"
	"github.com/lecritus/easy-webdav/internal/upload"
	webassets "github.com/lecritus/easy-webdav/internal/web"
	dav "github.com/lecritus/easy-webdav/internal/webdav"
)

type Server struct {
	Config config.Config
	Store  *store.Store
	Auth   *api.AuthAPI
	HTTP   *http.Server
}

func New(c config.Config, db *store.Store) *Server {
	basic := auth.NewBasicAuthenticator(db)
	authAPI := &api.AuthAPI{Store: db, Sessions: auth.Sessions{Store: db}, Limiter: auth.NewLoginLimiter(), StorageDir: c.StorageDir, Basic: basic}
	davService := dav.NewService(c.StorageDir, db)
	uploads := upload.NewManager(c.StorageDir)
	uploads.Precheck = func(root string, total int64) error {
		users, err := db.ListUsers(context.Background())
		if err != nil {
			return err
		}
		for _, u := range users {
			if filepath.Clean(filepath.Join(c.StorageDir, u.RootDir)) == filepath.Clean(root) && u.Quota > 0 {
				used, _ := db.Usage(context.Background(), root)
				if used+total > u.Quota {
					return quota.ErrExceeded
				}
			}
		}
		return nil
	}
	uploads.Commit = func(root string, delta int64) error { return db.AddUsage(context.Background(), root, delta) }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /api/v1/auth/login", authAPI.Login)
	mux.HandleFunc("POST /api/v1/auth/logout", authAPI.Logout)
	mux.HandleFunc("GET /api/v1/auth/me", authAPI.Me)
	mux.HandleFunc("POST /api/v1/auth/password", authAPI.Password)
	mux.HandleFunc("GET /api/v1/setup/status", authAPI.SetupStatus)
	mux.HandleFunc("POST /api/v1/setup", authAPI.Setup)
	mux.Handle("/dav/", davService.Handler())
	mux.Handle("/api/v1/users", authAPI.RequireSession(http.HandlerFunc(authAPI.Users)))
	mux.Handle("/api/v1/users/", authAPI.RequireSession(http.HandlerFunc(authAPI.UserByPath)))
	mux.Handle("/api/v1/settings", authAPI.RequireSession(http.HandlerFunc(authAPI.Settings)))
	mux.Handle("/api/v1/files", authAPI.RequireSession(http.HandlerFunc(authAPI.Files)))
	mux.Handle("/api/v1/files/download", authAPI.RequireSession(http.HandlerFunc(authAPI.Download)))
	mux.Handle("/api/v1/files/action", authAPI.RequireSession(http.HandlerFunc(authAPI.FileAction)))
	mux.Handle("/api/v1/files/raw", authAPI.RequireSession(http.HandlerFunc(authAPI.Raw)))
	mux.Handle("/api/v1/usage", authAPI.RequireSession(http.HandlerFunc(authAPI.Usage)))
	mux.Handle("/api/v1/usage/", authAPI.RequireSession(http.HandlerFunc(authAPI.Usage)))
	mux.Handle("/api/v1/uploads", authAPI.RequireSession(uploads.Handler(authAPI.UploadRoot, api.UploadReadOnly)))
	mux.Handle("/api/v1/uploads/", authAPI.RequireSession(uploads.Handler(authAPI.UploadRoot, api.UploadReadOnly)))
	ui := webassets.Handler(c.BaseURL)
	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			http.NotFound(w, r)
			return
		}
		ui.ServeHTTP(w, r)
	})
	mux.Handle("/", root)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted, _ := auth.ParseTrustedProxies(c.TrustedProxies)
		r = auth.WithRequestMeta(r, trusted)
		auth.CSRF(mux).ServeHTTP(w, r)
	})
	logger := logging.New(c.LogFormat, c.LogLevel)
	return &Server{Config: c, Store: db, Auth: authAPI, HTTP: &http.Server{Addr: c.Listen, Handler: logging.Access(logger, c.AccessLog, wrapped)}}
}

func (s *Server) ListenAndServe() error { return s.HTTP.ListenAndServe() }
func (s *Server) ListenAndServeTLS() error {
	return s.HTTP.ListenAndServeTLS(s.Config.TLSCert, s.Config.TLSKey)
}
func (s *Server) Shutdown(ctx context.Context) error { return s.HTTP.Shutdown(ctx) }
