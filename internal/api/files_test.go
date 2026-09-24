package api

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/122440367/easy-webdav/internal/store"
)

func TestDownloadMultiplePathsAsZip(t *testing.T) {
	a := testAPI(t)
	root := filepath.Join(a.StorageDir, "alice")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0750); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"one.txt": "one", "docs/two.txt": "two"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	id, err := a.Store.CreateUser(context.Background(), store.User{Username: "alice", Role: "user", RootDir: "alice", Permission: "readwrite"})
	if err != nil {
		t.Fatal(err)
	}
	user, err := a.Store.UserByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/files/download?path=one.txt&path=docs", nil)
	req = WithUser(user, req)
	response := httptest.NewRecorder()
	a.Download(response, req)
	if response.Code != 200 || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("unexpected response: %d %q", response.Code, response.Header().Get("Content-Type"))
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 2 {
		t.Fatalf("zip contains %d entries, want 2", len(archive.File))
	}
	contents := map[string]string{}
	for _, file := range archive.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(stream)
		_ = stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		contents[file.Name] = string(data)
	}
	if contents["one.txt"] != "one" || contents[filepath.ToSlash(filepath.Join("docs", "two.txt"))] != "two" {
		t.Fatalf("unexpected ZIP entries: %#v", contents)
	}
}
