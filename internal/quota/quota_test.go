package quota

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteCleansOnLimit(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "file")
	_, err := AtomicWrite(dst, bytes.NewBufferString("123456"), 5)
	if !errors.Is(err, ErrExceeded) {
		t.Fatalf("expected quota error, got %v", err)
	}
	if _, err = os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("destination published after quota error")
	}
}
func TestAtomicWritePublishes(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "nested", "file")
	n, err := AtomicWrite(dst, bytes.NewBufferString("hello"), 5)
	if err != nil || n != 5 {
		t.Fatalf("write failed: %d %v", n, err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "hello" {
		t.Fatal("wrong content")
	}
}
