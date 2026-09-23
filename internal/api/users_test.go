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
