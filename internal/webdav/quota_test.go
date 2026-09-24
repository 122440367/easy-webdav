package webdav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lecritus/easy-webdav/internal/auth"
	"github.com/lecritus/easy-webdav/internal/store"
)

func TestWebDAVQuotaEnforcementAndOverwriteAccounting(t *testing.T) {
	service, storage, _ := newTestService(t)
	user, err := service.Store.UserByName(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.UpdateUser(context.Background(), user.ID, user.Username, user.RootDir, user.Permission, 5, false); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	client := davClient(t, "admin")
	base := server.URL + "/dav/"

	request, _ := http.NewRequest("PUT", base+"over-limit.txt", strings.NewReader("123456"))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 507 {
		t.Fatalf("over-quota PUT status %d, want 507", response.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(storage, "admin", "over-limit.txt")); !os.IsNotExist(err) {
		t.Fatal("over-quota PUT left a destination file")
	}
	if entries, err := os.ReadDir(filepath.Join(storage, ".ew-tmp")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("over-quota PUT left temporary files: %v", entries)
	}
	if used, err := service.Store.Usage(context.Background(), filepath.Join(storage, "admin")); err != nil || used != 0 {
		t.Fatalf("usage after rejected PUT = %d, %v", used, err)
	}

	if response := do(t, client, "PUT", base+"file.txt", "123", nil); response.StatusCode != 201 {
		t.Fatalf("initial PUT status %d", response.StatusCode)
	}
	if response := do(t, client, "PUT", base+"file.txt", "12345", nil); response.StatusCode != 201 {
		t.Fatalf("within-quota overwrite status %d", response.StatusCode)
	}
	if used, err := service.Store.Usage(context.Background(), filepath.Join(storage, "admin")); err != nil || used != 5 {
		t.Fatalf("usage after overwrite = %d, %v; want 5", used, err)
	}
	request, _ = http.NewRequest("COPY", base+"file.txt", nil)
	request.Header.Set("Destination", base+"copy.txt")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 507 {
		t.Fatalf("over-quota COPY status %d, want 507", response.StatusCode)
	}
}

// A PUT without Content-Length is streamed, so the quota can only be enforced
// while the body is being written. The write must be abandoned and the partial
// data removed.
func TestWebDAVChunkedUploadOverQuotaIsAborted(t *testing.T) {
	service, storage, _ := newTestService(t)
	user, err := service.Store.UserByName(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.UpdateUser(context.Background(), user.ID, user.Username, user.RootDir, user.Permission, 5, false); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	client := davClient(t, "admin")

	// io.LimitedReader is not one of the types net/http measures, so the
	// request goes out without a Content-Length header (chunked encoding).
	body := io.LimitReader(strings.NewReader(strings.Repeat("x", 64)), 64)
	request, _ := http.NewRequest("PUT", server.URL+"/dav/streamed.bin", body)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 507 {
		t.Fatalf("streamed over-quota PUT status %d, want 507", response.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(storage, "admin", "streamed.bin")); !os.IsNotExist(err) {
		t.Fatal("aborted streamed upload published a destination file")
	}
	tempRoot := filepath.Join(storage, ".ew-tmp")
	entries, err := os.ReadDir(tempRoot)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("aborted streamed upload left temporary data: %v", entries)
	}
	if used, err := service.Store.Usage(context.Background(), filepath.Join(storage, "admin")); err != nil || used != 0 {
		t.Fatalf("usage after aborted upload = %d, %v", used, err)
	}
}

// Two accounts pointing at the same root share both the files and the usage
// counter, so each of them is limited by the combined total.
func TestWebDAVSharedRootSharesUsageAndQuota(t *testing.T) {
	db, err := store.Open("file:dav-shared-" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	storage := t.TempDir()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(storage, "shared"), 0750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "bob"} {
		if _, err := db.CreateUser(context.Background(), store.User{Username: name, PasswordHash: hash, Role: "user", RootDir: "shared", Permission: "readwrite", Quota: 5}); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(NewService(storage, db).Handler())
	t.Cleanup(server.Close)

	if response := do(t, davClient(t, "alice"), "PUT", server.URL+"/dav/first.txt", "12345", nil); response.StatusCode != 201 {
		t.Fatalf("alice PUT status %d", response.StatusCode)
	}
	// The shared root is already full, so bob cannot add to it.
	if response := do(t, davClient(t, "bob"), "PUT", server.URL+"/dav/second.txt", "6", nil); response.StatusCode != 507 {
		t.Fatalf("bob PUT into full shared root status %d, want 507", response.StatusCode)
	}
	used, err := db.Usage(context.Background(), filepath.Join(storage, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	if used != 5 {
		t.Fatalf("shared root usage = %d, want 5", used)
	}
}
