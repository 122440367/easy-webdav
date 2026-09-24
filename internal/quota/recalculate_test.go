package quota

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/122440367/easy-webdav/internal/store"
)

func TestRecalculateFixesDriftAndIgnoresTempData(t *testing.T) {
	storage := t.TempDir()
	dsn := "file:recalculate-" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.CreateUser(ctx, store.User{Username: "alice", Role: "user", RootDir: "alice", Permission: "readwrite"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(storage, "alice")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("1234"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "b.txt"), []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	// The cached number has drifted away from the real directory contents.
	if err := db.SetUsage(ctx, root, 999); err != nil {
		t.Fatal(err)
	}

	// Upload scratch space lives outside every user root, so it must never be
	// part of a user's usage; abandoned directories are cleaned up.
	tempRoot := filepath.Join(storage, ".ew-tmp")
	fresh := filepath.Join(tempRoot, "fresh-upload")
	stale := filepath.Join(tempRoot, "stale-upload")
	for _, dir := range []string{fresh, stale} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "payload"), bytes.Repeat([]byte("x"), 1024), 0600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if err := Recalculate(db, storage); err != nil {
		t.Fatal(err)
	}
	used, err := db.Usage(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if used != 9 {
		t.Fatalf("usage after recalculation = %d, want 9", used)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale upload directory was not cleaned up")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("in-flight upload directory was removed")
	}
}

func TestRecalculateCoversSharedRootOnce(t *testing.T) {
	storage := t.TempDir()
	dsn := "file:recalculate-shared-" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, name := range []string{"alice", "bob"} {
		if _, err := db.CreateUser(ctx, store.User{Username: name, Role: "user", RootDir: "shared", Permission: "readwrite"}); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(storage, "shared")
	if err := os.MkdirAll(root, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("1234567"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Recalculate(db, storage); err != nil {
		t.Fatal(err)
	}
	used, err := db.Usage(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if used != 7 {
		t.Fatalf("shared root usage = %d, want 7 (counted once)", used)
	}
}
