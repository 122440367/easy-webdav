package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/store"
)

func createUser(t *testing.T, a *AuthAPI, u store.User) store.User {
	t.Helper()
	if u.PasswordHash == "" {
		hash, err := auth.HashPassword("password123")
		if err != nil {
			t.Fatal(err)
		}
		u.PasswordHash = hash
	}
	if u.Role == "" {
		u.Role = "user"
	}
	if u.Permission == "" {
		u.Permission = "readwrite"
	}
	if err := os.MkdirAll(filepath.Join(a.StorageDir, u.RootDir), 0750); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	created, err := a.Store.UserByName(context.Background(), u.Username)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func call(t *testing.T, handler func(http.ResponseWriter, *http.Request), user store.User, method, url string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	req = WithUser(user, req)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestUsageEndpointsExposeGlobalAndPersonalUsage(t *testing.T) {
	a := testAPI(t)
	admin := createUser(t, a, store.User{Username: "admin", Role: "admin", RootDir: "admin"})
	alice := createUser(t, a, store.User{Username: "alice", RootDir: "alice", Quota: 100})
	root := filepath.Join(a.StorageDir, "alice")
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SetUsage(context.Background(), root, 3); err != nil {
		t.Fatal(err)
	}

	rec := call(t, a.Usage, admin, http.MethodGet, "/api/v1/usage", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("global usage status %d: %s", rec.Code, rec.Body)
	}
	var overview struct {
		Users []struct {
			User map[string]any `json:"user"`
			Used int64          `json:"used"`
		} `json:"users"`
		Disk map[string]any `json:"disk"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if len(overview.Users) != 2 {
		t.Fatalf("overview lists %d users, want 2", len(overview.Users))
	}
	found := false
	for _, item := range overview.Users {
		if item.User["username"] == "alice" {
			found = true
			if item.Used != 3 {
				t.Fatalf("alice usage in overview = %d, want 3", item.Used)
			}
		}
	}
	if !found {
		t.Fatal("overview is missing alice")
	}
	if overview.Disk["storage_dir"] != a.StorageDir {
		t.Fatalf("disk block does not report the storage directory: %v", overview.Disk)
	}
	if free, ok := overview.Disk["free"]; !ok || free.(float64) <= 0 {
		t.Fatalf("disk free space missing: %v", overview.Disk)
	}

	rec = call(t, a.Usage, alice, http.MethodGet, "/api/v1/usage/me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("personal usage status %d: %s", rec.Code, rec.Body)
	}
	var mine struct {
		RootDir string `json:"root_dir"`
		Used    int64  `json:"used"`
		Quota   int64  `json:"quota"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &mine); err != nil {
		t.Fatal(err)
	}
	if mine.Used != 3 || mine.Quota != 100 || mine.RootDir != "alice" {
		t.Fatalf("personal usage = %+v", mine)
	}

	if rec := call(t, a.Usage, alice, http.MethodGet, "/api/v1/usage", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin global usage status %d, want 403", rec.Code)
	}
}

func TestRawPreviewPolicy(t *testing.T) {
	a := testAPI(t)
	alice := createUser(t, a, store.User{Username: "alice", RootDir: "alice"})
	root := filepath.Join(a.StorageDir, "alice")
	files := map[string]string{
		"page.html":     "<script>alert(1)</script>",
		"notes.txt":     "plain text",
		"photo.png":     "not-a-real-png",
		"archive.zip":   "PK",
		"dir/report.md": "# report",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A text file above the 2 MB preview limit.
	if err := os.WriteFile(filepath.Join(root, "huge.log"), bytes.Repeat([]byte("x"), 3*1024*1024), 0600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path            string
		wantContentType string
		wantDisposition string
		wantBody        string
	}{
		{"page.html", "text/plain; charset=utf-8", "attachment", files["page.html"]},
		{"notes.txt", "text/plain; charset=utf-8", "inline", files["notes.txt"]},
		{"photo.png", "image/png", "inline", files["photo.png"]},
		{"archive.zip", "application/octet-stream", "attachment", files["archive.zip"]},
		{"dir/report.md", "text/plain; charset=utf-8", "inline", files["dir/report.md"]},
	}
	for _, tc := range cases {
		rec := call(t, a.Raw, alice, http.MethodGet, "/api/v1/files/raw?path="+tc.path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d %s", tc.path, rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Content-Type"); got != tc.wantContentType {
			t.Fatalf("%s: content type %q, want %q", tc.path, got, tc.wantContentType)
		}
		if got := rec.Header().Get("Content-Disposition"); got != tc.wantDisposition {
			t.Fatalf("%s: disposition %q, want %q", tc.path, got, tc.wantDisposition)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: missing nosniff header", tc.path)
		}
		if rec.Body.String() != tc.wantBody {
			t.Fatalf("%s: body %q, want %q", tc.path, rec.Body.String(), tc.wantBody)
		}
	}

	rec := call(t, a.Raw, alice, http.MethodGet, "/api/v1/files/raw?path=huge.log", nil)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized text preview status %d, want 413", rec.Code)
	}
}

func TestFilesAPIRejectsTraversalAndUnknownCrossUser(t *testing.T) {
	a := testAPI(t)
	admin := createUser(t, a, store.User{Username: "admin", Role: "admin", RootDir: "admin"})
	alice := createUser(t, a, store.User{Username: "alice", RootDir: "alice"})
	outside := filepath.Join(filepath.Dir(a.StorageDir), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"../outside.txt", "/etc/passwd", "%2e%2e/outside.txt"} {
		rec := call(t, a.Files, alice, http.MethodGet, "/api/v1/files?path="+path, nil)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Fatalf("traversal %q returned %d", path, rec.Code)
		}
	}
	if body, err := os.ReadFile(outside); err != nil || string(body) != "secret" {
		t.Fatal("traversal request modified files outside the root")
	}

	// A normal user cannot point the file API at somebody else's root.
	rec := call(t, a.Files, alice, http.MethodGet, "/api/v1/files?user_id="+userID(admin.ID), nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin cross-user listing status %d, want 403", rec.Code)
	}
	rec = call(t, a.Files, admin, http.MethodGet, "/api/v1/files?user_id="+userID(alice.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin cross-user listing status %d: %s", rec.Code, rec.Body)
	}
}

func TestAdminWriteIntoReadOnlyUserRootCountsAgainstThatUser(t *testing.T) {
	a := testAPI(t)
	admin := createUser(t, a, store.User{Username: "admin", Role: "admin", RootDir: "admin"})
	reader := createUser(t, a, store.User{Username: "reader", RootDir: "reader", Permission: "read", Quota: 4})
	root := filepath.Join(a.StorageDir, "reader")
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SetUsage(context.Background(), root, 5); err != nil {
		t.Fatal(err)
	}
	action := func(destination string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"action": "copy", "path": "big.txt", "destination": destination})
		return call(t, a.FileAction, admin, http.MethodPut, "/api/v1/files/action?user_id="+userID(reader.ID), body)
	}
	rec := action("copy.txt")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("admin copy into read-only user's full root: status %d, want 413 (%s)", rec.Code, rec.Body)
	}
	var errorBody struct {
		Details map[string]int64 `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errorBody); err != nil || errorBody.Details["remaining"] != 0 {
		t.Fatalf("413 response lacks remaining bytes: %s (%v)", rec.Body, err)
	}
	if err := a.Store.UpdateUser(context.Background(), reader.ID, reader.Username, reader.RootDir, reader.Permission, 20, false); err != nil {
		t.Fatal(err)
	}
	if rec := action("copy.txt"); rec.Code != http.StatusOK {
		t.Fatalf("admin copy with room: status %d (%s)", rec.Code, rec.Body)
	}
	if used, err := a.Store.Usage(context.Background(), root); err != nil || used != 10 {
		t.Fatalf("usage on the target user's root = %d, %v; want 10", used, err)
	}
	if strings.TrimSpace(reader.RootDir) != "reader" {
		t.Fatal("unexpected fixture state")
	}
}
