package webdav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/store"
)

// newTestService builds a DAV service backed by a temporary storage root and
// two users: admin (readwrite) and reader (read).
func newTestService(t *testing.T) (*Service, string, string) {
	t.Helper()
	db, err := store.Open("file:dav-integration-" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	storage := t.TempDir()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admin", "reader"} {
		permission := "readwrite"
		if name == "reader" {
			permission = "read"
		}
		if err := os.MkdirAll(filepath.Join(storage, name), 0750); err != nil {
			t.Fatal(err)
		}
		if _, err := db.CreateUser(context.Background(), store.User{Username: name, PasswordHash: hash, Role: "user", RootDir: name, Permission: permission}); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(storage, db), storage, hash
}

func davClient(t *testing.T, username string) *http.Client {
	t.Helper()
	return &http.Client{Transport: basicTransport{username: username, password: "password123", inner: http.DefaultTransport}}
}

type basicTransport struct {
	username, password string
	inner              http.RoundTripper
}

func (b basicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(b.username, b.password)
	return b.inner.RoundTrip(req)
}

func do(t *testing.T, client *http.Client, method, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestWebDAVProtocolBehaviour(t *testing.T) {
	service, storage, _ := newTestService(t)
	srv := httptest.NewServer(service.Handler())
	t.Cleanup(srv.Close)
	admin := davClient(t, "admin")
	base := srv.URL + "/dav/"

	t.Run("OPTIONS advertises classes", func(t *testing.T) {
		resp := do(t, admin, "OPTIONS", base, "", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if resp.Header.Get("DAV") != "1, 2" {
			t.Fatalf("DAV header %q", resp.Header.Get("DAV"))
		}
		if resp.Header.Get("MS-Author-Via") != "DAV" {
			t.Fatalf("MS-Author-Via missing")
		}
	})

	t.Run("PUT GET roundtrip and Range", func(t *testing.T) {
		if resp := do(t, admin, "MKCOL", base+"a", "", nil); resp.StatusCode != 201 {
			t.Fatalf("MKCOL status %d", resp.StatusCode)
		}
		content := strings.Repeat("0123456789", 100) // 1000 bytes
		resp := do(t, admin, "PUT", base+"a/b.txt", content, nil)
		if resp.StatusCode != 201 {
			t.Fatalf("PUT status %d", resp.StatusCode)
		}
		resp = do(t, admin, "GET", base+"a/b.txt", "", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("GET status %d", resp.StatusCode)
		}
		got, _ := io.ReadAll(resp.Body)
		if string(got) != content {
			t.Fatal("content mismatch")
		}
		if resp.Header.Get("ETag") == "" {
			t.Fatal("ETag missing")
		}
		resp = do(t, admin, "GET", base+"a/b.txt", "", map[string]string{"Range": "bytes=0-99"})
		if resp.StatusCode != 206 {
			t.Fatalf("Range status %d", resp.StatusCode)
		}
		got, _ = io.ReadAll(resp.Body)
		if len(got) != 100 {
			t.Fatalf("Range body %d bytes", len(got))
		}
		if cr := resp.Header.Get("Content-Range"); !strings.HasPrefix(cr, "bytes 0-99/1000") {
			t.Fatalf("Content-Range %q", cr)
		}
	})

	t.Run("PROPFIND trailing slash on collections", func(t *testing.T) {
		resp := do(t, admin, "PROPFIND", base+"a/", "", map[string]string{"Depth": "1"})
		if resp.StatusCode != 207 {
			t.Fatalf("PROPFIND status %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), `/dav/a/`) {
			t.Fatalf("collection href missing trailing slash: %s", body)
		}
	})

	t.Run("MOVE without Overwrite header succeeds", func(t *testing.T) {
		if resp := do(t, admin, "PUT", base+"rename-me.txt", "data", nil); resp.StatusCode != 201 {
			t.Fatalf("put status %d", resp.StatusCode)
		}
		req, _ := http.NewRequest("MOVE", base+"rename-me.txt", nil)
		req.Header.Set("Destination", base+"renamed.txt")
		resp, err := admin.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Fatalf("MOVE without Overwrite status %d", resp.StatusCode)
		}
		if _, err := os.Stat(filepath.Join(storage, "admin", "renamed.txt")); err != nil {
			t.Fatal("renamed file missing on disk")
		}
	})

	t.Run("collection PROPPATCH returns 207", func(t *testing.T) {
		body := `<?xml version="1.0"?><D:propertyupdate xmlns:D="DAV:"><D:set><D:prop><Win32LastModifiedTime xmlns="urn:schemas-microsoft-com:">Mon, 01 Jan 2024 00:00:00 GMT</Win32LastModifiedTime></D:prop></D:set></D:propertyupdate>`
		resp := do(t, admin, "PROPPATCH", base+"a/", body, nil)
		if resp.StatusCode != 207 {
			t.Fatalf("PROPPATCH status %d", resp.StatusCode)
		}
		respBody, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(respBody), "403") {
			t.Fatalf("expected 403 propstat inside 207: %s", respBody)
		}
	})

	t.Run("infinite depth PROPFIND rejected", func(t *testing.T) {
		resp := do(t, admin, "PROPFIND", base, "", map[string]string{"Depth": "infinity"})
		if resp.StatusCode != 403 {
			t.Fatalf("infinite depth status %d", resp.StatusCode)
		}
	})

	t.Run("LOCK conflict returns 423", func(t *testing.T) {
		if resp := do(t, admin, "PUT", base+"locked.txt", "v1", nil); resp.StatusCode != 201 {
			t.Fatalf("put status %d", resp.StatusCode)
		}
		lockBody := `<?xml version="1.0"?><D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>tester</D:owner></D:lockinfo>`
		resp := do(t, admin, "LOCK", base+"locked.txt", lockBody, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("LOCK status %d", resp.StatusCode)
		}
		lockBodyXML, _ := io.ReadAll(resp.Body)
		token := resp.Header.Get("Lock-Token")
		if token == "" || !strings.Contains(string(lockBodyXML), "locktoken") {
			t.Fatalf("no lock token: %q %s", token, lockBodyXML)
		}
		// Another client writing without the token is rejected...
		resp = do(t, admin, "PUT", base+"locked.txt", "v2", nil)
		if resp.StatusCode != 423 {
			t.Fatalf("expected 423, got %d", resp.StatusCode)
		}
		// ...while the owner, presenting the token, succeeds.
		req, _ := http.NewRequest("PUT", base+"locked.txt", strings.NewReader("v3"))
		req.Header.Set("If", fmt.Sprintf("(%s)", strings.Trim(token, "<>")))
		resp, err := admin.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			t.Fatalf("owner PUT with token status %d", resp.StatusCode)
		}
	})

	t.Run("encoded traversal stays inside root", func(t *testing.T) {
		resp := do(t, admin, "GET", base+"%2e%2e/reader/secret", "", nil)
		if resp.StatusCode != 404 {
			t.Fatalf("encoded escape status %d", resp.StatusCode)
		}
		if _, err := os.Stat(filepath.Join(storage, "reader", "secret")); !os.IsNotExist(err) {
			t.Fatal("escape created a file")
		}
	})
}

func TestWebDAVReadOnlyUser(t *testing.T) {
	service, _, _ := newTestService(t)
	srv := httptest.NewServer(service.Handler())
	t.Cleanup(srv.Close)
	admin := davClient(t, "admin")
	reader := davClient(t, "reader")
	base := srv.URL + "/dav/"

	// Seed a file as the admin.
	if resp := do(t, admin, "PUT", base+"note.txt", "hello", nil); resp.StatusCode != 201 {
		t.Fatalf("seed put status %d", resp.StatusCode)
	}
	for _, tc := range []struct{ method, url string }{
		{"PUT", base + "new.txt"},
		{"MKCOL", base + "newdir"},
		{"DELETE", base + "note.txt"},
		{"PROPPATCH", base + "note.txt"},
		{"LOCK", base + "note.txt"},
	} {
		resp := do(t, reader, tc.method, tc.url, "", nil)
		if resp.StatusCode != 403 {
			t.Fatalf("%s as read user: expected 403, got %d", tc.method, resp.StatusCode)
		}
	}
	// Reads still work in the reader's own (empty) root.
	if resp := do(t, reader, "PROPFIND", base, "", map[string]string{"Depth": "1"}); resp.StatusCode != 207 {
		t.Fatalf("read user PROPFIND status %d", resp.StatusCode)
	}
	// The admin's files are not visible to the reader (root isolation).
	if resp := do(t, reader, "GET", base+"note.txt", "", nil); resp.StatusCode != 404 {
		t.Fatalf("cross-root GET should be 404, got %d", resp.StatusCode)
	}
	// COPY/MOVE are rejected too.
	req, _ := http.NewRequest("COPY", base+"note.txt", nil)
	req.Header.Set("Destination", base+"copy.txt")
	req.SetBasicAuth("reader", "password123")
	resp, err := (&http.Client{Transport: http.DefaultTransport}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("read user COPY status %d", resp.StatusCode)
	}
}

func TestWebDAVDestinationBoundary(t *testing.T) {
	service, _, _ := newTestService(t)
	srv := httptest.NewServer(service.Handler())
	t.Cleanup(srv.Close)
	admin := davClient(t, "admin")
	base := srv.URL + "/dav/"

	if resp := do(t, admin, "PUT", base+"to-move.txt", "x", nil); resp.StatusCode != 201 {
		t.Fatalf("put status %d", resp.StatusCode)
	}
	for _, method := range []string{"COPY", "MOVE"} {
		req, _ := http.NewRequest(method, base+"to-move.txt", nil)
		// Destination escapes the /dav/ namespace entirely.
		req.Header.Set("Destination", srv.URL+"/elsewhere.txt")
		resp, err := admin.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("%s outside destination: expected 403, got %d", method, resp.StatusCode)
		}
	}
}

func TestWebDAVSymlinks(t *testing.T) {
	service, storage, _ := newTestService(t)
	srv := httptest.NewServer(service.Handler())
	t.Cleanup(srv.Close)
	admin := davClient(t, "admin")
	base := srv.URL + "/dav/"
	root := filepath.Join(storage, "admin")

	if err := os.MkdirAll(filepath.Join(root, "archive", "docs"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "archive", "docs", "file.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "archive", "docs"), filepath.Join(root, "docs")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// A chain: chain -> docs (in-root), docs -> archive/docs; plus an escape.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "chain")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// In-root symlink is followed.
	resp := do(t, admin, "GET", base+"docs/file.txt", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("in-root symlink GET status %d", resp.StatusCode)
	}
	// Chained in-root symlink resolves.
	resp = do(t, admin, "PROPFIND", base+"chain/", "", map[string]string{"Depth": "1"})
	if resp.StatusCode != 207 {
		t.Fatalf("chained symlink PROPFIND status %d", resp.StatusCode)
	}
	// Out-of-root symlink reads as missing.
	resp = do(t, admin, "GET", base+"escape/secret.txt", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("escaping symlink GET status %d", resp.StatusCode)
	}
}
