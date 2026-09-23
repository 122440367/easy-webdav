package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lecritus/easy-webdav/internal/auth"
	"github.com/lecritus/easy-webdav/internal/store"
)

func adminRequest(t *testing.T, a *AuthAPI, method, url string, body any) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, url, bytes.NewReader(raw))
	admin, err := a.Store.UserByName(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	req = WithUser(admin, req)
	return httptest.NewRecorder(), req
}
func TestUserManagementRules(t *testing.T) {
	a := testAPI(t)
	hash, _ := auth.HashPassword("password123")
	id, err := a.Store.CreateUser(context.Background(), store.User{Username: "admin", PasswordHash: hash, Role: "admin", RootDir: "admin", Permission: "readwrite"})
	if err != nil {
		t.Fatal(err)
	}
	_ = id
	rec, req := adminRequest(t, a, http.MethodPost, "/api/v1/users", userInput{Username: "alice", Password: "password123"})
	a.Users(rec, req)
	if rec.Code != 201 {
		t.Fatalf("create user status %d: %s", rec.Code, rec.Body)
	}
	rec, req = adminRequest(t, a, http.MethodPost, "/api/v1/users", userInput{Username: "alice", Password: "password123"})
	a.Users(rec, req)
	if rec.Code != 409 {
		t.Fatalf("duplicate status %d", rec.Code)
	}
	rec, req = adminRequest(t, a, http.MethodDelete, "/api/v1/users/1", nil)
	a.UserByPath(rec, req)
	if rec.Code != 409 {
		t.Fatalf("last admin delete status %d", rec.Code)
	}
}

func TestInvalidRootDirRejected(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	for _, root := range []string{"../outside", "a/../../b", "", ".", "/abs"} {
		rec, req := adminRequest(t, a, http.MethodPost, "/api/v1/users", userInput{Username: "bob", Password: "password123", RootDir: &root})
		a.Users(rec, req)
		if rec.Code != 400 {
			t.Fatalf("root %q: expected 400, got %d %s", root, rec.Code, rec.Body)
		}
	}
}

func TestLastAdminCannotBeDisabled(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	admin, _ := a.Store.UserByName(context.Background(), "admin")
	rec, req := adminRequest(t, a, http.MethodPut, "/api/v1/users/"+userID(admin.ID), userInput{Username: "admin", RootDir: ptr("admin"), Permission: "readwrite", Disabled: true})
	a.UserByPath(rec, req)
	if rec.Code != 409 {
		t.Fatalf("last admin disable status %d: %s", rec.Code, rec.Body)
	}
}

func TestPermissionChangeTakesEffectImmediately(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	rec, req := adminRequest(t, a, http.MethodPost, "/api/v1/users", userInput{Username: "alice", Password: "password123"})
	a.Users(rec, req)
	if rec.Code != 201 {
		t.Fatalf("create alice: %d %s", rec.Code, rec.Body)
	}
	alice, _ := a.Store.UserByName(context.Background(), "alice")
	// Alice logs in, then the admin flips her to read-only. The session
	// middleware re-reads the user per request, so no re-login is required.
	cookie := sessionCookie(t, postJSON(a.Login, "/api/v1/auth/login", loginBody("alice", "password123"), nil))
	rec, req = adminRequest(t, a, http.MethodPut, "/api/v1/users/"+userID(alice.ID), userInput{Username: "alice", RootDir: ptr("alice"), Permission: "read"})
	a.UserByPath(rec, req)
	if rec.Code != 200 {
		t.Fatalf("permission update status %d: %s", rec.Code, rec.Body)
	}
	mkdir, _ := json.Marshal(map[string]string{"path": "newdir"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/files/action", bytes.NewReader(mkdir))
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	a.RequireSession(http.HandlerFunc(a.FileAction)).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("read user mkdir status %d: %s", rec.Code, rec.Body)
	}
}

func TestDefaultSettingsAppliedToNewUsers(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	rec, req := adminRequest(t, a, http.MethodPut, "/api/v1/settings", map[string]string{"default_quota": "5368709120", "default_permission": "read"})
	a.Settings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("update settings: %d %s", rec.Code, rec.Body)
	}
	rec, req = adminRequest(t, a, http.MethodPost, "/api/v1/users", userInput{Username: "carol", Password: "password123"})
	a.Users(rec, req)
	if rec.Code != 201 {
		t.Fatalf("create carol: %d %s", rec.Code, rec.Body)
	}
	carol, _ := a.Store.UserByName(context.Background(), "carol")
	if carol.Quota != 5368709120 || carol.Permission != "read" {
		t.Fatalf("defaults not applied: quota=%d permission=%s", carol.Quota, carol.Permission)
	}
}

func TestNonAdminCannotChangeSettings(t *testing.T) {
	a := testAPI(t)
	seedAdmin(t, a)
	rec, req := adminRequest(t, a, http.MethodPost, "/api/v1/users", userInput{Username: "alice", Password: "password123"})
	a.Users(rec, req)
	if rec.Code != 201 {
		t.Fatalf("create alice: %d %s", rec.Code, rec.Body)
	}
	alice, _ := a.Store.UserByName(context.Background(), "alice")
	req = httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader([]byte(`{"site_name":"hax"}`)))
	req = WithUser(alice, req)
	rec = httptest.NewRecorder()
	a.Settings(rec, req)
	if rec.Code != 403 {
		t.Fatalf("non-admin settings status %d: %s", rec.Code, rec.Body)
	}
	// Read access is allowed for any logged-in user.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req = WithUser(alice, req)
	rec = httptest.NewRecorder()
	a.Settings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("non-admin settings read status %d", rec.Code)
	}
}

func ptr(value string) *string { return &value }
