package webdav

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRestrictedFSRejectsEscapeAndOutsideSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	fs, err := NewRestrictedFS(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fs.Stat(context.Background(), "../secret"); !os.IsNotExist(err) {
		t.Fatal("path escape accepted")
	}
	if _, err = fs.Stat(context.Background(), "link/secret"); !os.IsNotExist(err) {
		t.Fatal("outside symlink accepted")
	}
}
func TestRestrictedFSReadOnly(t *testing.T) {
	root := t.TempDir()
	fs, err := NewRestrictedFS(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = fs.Mkdir(context.Background(), "new", 0750); !os.IsPermission(err) {
		t.Fatal("read-only mkdir accepted")
	}
}
