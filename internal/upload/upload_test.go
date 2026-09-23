package upload

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSequentialUploadAndConflict(t *testing.T) {
	root := t.TempDir()
	m := NewManager(root)
	s, err := m.Create(root, "folder/file.txt", 6, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Chunk(s.ID, 1, bytes.NewBufferString("x")); err == nil {
		t.Fatal("out-of-order chunk accepted")
	}
	if _, err = m.Chunk(s.ID, 0, bytes.NewBufferString("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Chunk(s.ID, 1, bytes.NewBufferString("def")); err != nil {
		t.Fatal(err)
	}
	dst, err := m.Complete(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "abcdef" {
		t.Fatalf("content=%q", b)
	}
	if _, err = os.Stat(filepath.Join(root, ".ew-tmp", s.ID)); !os.IsNotExist(err) {
		t.Fatal("temporary directory remains")
	}
}
func TestRejectTraversal(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Create(t.TempDir(), "../outside", 1, "overwrite"); err == nil {
		t.Fatal("traversal accepted")
	}
}
