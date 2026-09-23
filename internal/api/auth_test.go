package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lecritus/easy-webdav/internal/auth"
	"github.com/lecritus/easy-webdav/internal/store"
)

func testAPI(t *testing.T) *AuthAPI {
	t.Helper()
	db, err := store.Open("file:api-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &AuthAPI{Store: db, Sessions: auth.Sessions{Store: db}, Limiter: auth.NewLoginLimiter(), StorageDir: filepath.Join(t.TempDir(), "files")}
}
func TestSetupLocksAndLogin(t *testing.T) {
	a := testAPI(t)
	body, _ := json.Marshal(credentials{Username: "admin", Password: "password123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	req.Header.Set("X-Requested-With", "fetch")
	rec := httptest.NewRecorder()
	a.Setup(rec, req)
	if rec.Code != 201 {
		t.Fatalf("setup status=%d body=%s", rec.Code, rec.Body)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	a.Setup(rec, req)
	if rec.Code != 403 {
		t.Fatalf("expected setup lock, got %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	a.Login(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body)
	}
}
func TestSetupBootstrap(t *testing.T) {
	a := testAPI(t)
	if err := a.Bootstrap(context.Background(), "admin", "password123"); err != nil {
		t.Fatal(err)
	}
	if count, _ := a.Store.AdminCount(context.Background()); count != 1 {
		t.Fatal("bootstrap did not create admin")
	}
}
