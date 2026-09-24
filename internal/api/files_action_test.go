package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/122440367/easy-webdav/internal/store"
)

func TestFileActionsEnforceQuotaAndMaintainUsage(t *testing.T) {
	a := testAPI(t)
	root := filepath.Join(a.StorageDir, "alice")
	if err := os.MkdirAll(root, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("four"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := a.Store.CreateUser(context.Background(), store.User{Username: "alice", Role: "user", RootDir: "alice", Permission: "readwrite", Quota: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SetUsage(context.Background(), root, 4); err != nil {
		t.Fatal(err)
	}
	u, err := a.Store.UserByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, action, source, destination string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"action": action, "path": source, "destination": destination})
		req := httptest.NewRequest(method, "/api/v1/files/action", bytes.NewReader(body))
		req = WithUser(u, req)
		recorder := httptest.NewRecorder()
		a.FileAction(recorder, req)
		return recorder
	}
	response := call(http.MethodPut, "copy", "source.txt", "copy.txt")
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-quota copy status %d, want 413", response.Code)
	}
	var errorBody struct {
		Details map[string]int64 `json:"details"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &errorBody); err != nil || errorBody.Details["remaining"] != 1 {
		t.Fatalf("quota response lacks remaining bytes: %s (%v)", response.Body, err)
	}
	if _, err := os.Stat(filepath.Join(root, "copy.txt")); !os.IsNotExist(err) {
		t.Fatal("rejected copy created a destination")
	}

	if err := a.Store.UpdateUser(context.Background(), id, u.Username, u.RootDir, u.Permission, 10, false); err != nil {
		t.Fatal(err)
	}
	u.Quota = 10
	if response := call(http.MethodPut, "copy", "source.txt", "copy.txt"); response.Code != http.StatusOK {
		t.Fatalf("copy status %d: %s", response.Code, response.Body)
	}
	if used, _ := a.Store.Usage(context.Background(), root); used != 8 {
		t.Fatalf("usage after copy %d, want 8", used)
	}
	if response := call(http.MethodDelete, "", "copy.txt", ""); response.Code != http.StatusOK {
		t.Fatalf("delete status %d: %s", response.Code, response.Body)
	}
	if used, _ := a.Store.Usage(context.Background(), root); used != 4 {
		t.Fatalf("usage after delete %d, want 4", used)
	}
	if response := call(http.MethodPut, "move", "source.txt", "moved.txt"); response.Code != http.StatusOK {
		t.Fatalf("move status %d: %s", response.Code, response.Body)
	}
	if used, _ := a.Store.Usage(context.Background(), root); used != 4 {
		t.Fatalf("usage after move %d, want 4", used)
	}
}

func TestSafeTargetPathRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := safeTargetPath(root, "escape/secret.txt"); err == nil {
		t.Fatal("symlink escape was allowed")
	}
}
