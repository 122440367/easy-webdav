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

func TestOverwriteQuotaUsesNetSizeChange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	var committed int64
	m := NewManager(t.TempDir())
	m.Precheck = func(_, _ string, total int64, strategy string) error {
		if total != 3 || strategy != "overwrite" {
			t.Fatalf("unexpected precheck: %d %q", total, strategy)
		}
		return nil
	}
	m.Commit = func(_ string, delta int64) error { committed = delta; return nil }
	s, err := m.Create(root, "file.txt", 3, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Chunk(s.ID, 0, bytes.NewBufferString("new")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Complete(s.ID); err != nil {
		t.Fatal(err)
	}
	if committed != -5 {
		t.Fatalf("usage delta = %d, want -5", committed)
	}
}

func TestChunkCannotExceedDeclaredSize(t *testing.T) {
	manager := NewManager(t.TempDir())
	session, err := manager.Create(t.TempDir(), "file", 2, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Chunk(session.ID, 0, bytes.NewBufferString("too long")); err == nil {
		t.Fatal("accepted bytes beyond declared upload size")
	}
	if _, err = manager.Complete(session.ID); err == nil {
		t.Fatal("completed an upload after an oversized chunk")
	}
}

func TestConflictStrategies(t *testing.T) {
	for _, tc := range []struct {
		strategy string
		wantName string
		content  string
	}{
		{"overwrite", "file.txt", "new"},
		{"skip", "file.txt", "old"},
		{"rename", "file (1).txt", "new"},
	} {
		t.Run(tc.strategy, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			var delta int64
			manager := NewManager(t.TempDir())
			manager.Commit = func(_ string, value int64) error { delta = value; return nil }
			session, err := manager.Create(root, "file.txt", 3, tc.strategy)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.Chunk(session.ID, 0, bytes.NewBufferString("new")); err != nil {
				t.Fatal(err)
			}
			destination, err := manager.Complete(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(destination) != tc.wantName {
				t.Fatalf("destination = %s, want %s", destination, tc.wantName)
			}
			body, err := os.ReadFile(destination)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != tc.content {
				t.Fatalf("content = %q, want %q", body, tc.content)
			}
			switch tc.strategy {
			case "overwrite":
				if delta != 0 {
					t.Fatalf("overwrite delta = %d, want 0 (3 new bytes replacing 3)", delta)
				}
			case "skip":
				if delta != 0 {
					t.Fatalf("skip delta = %d, want 0", delta)
				}
			case "rename":
				if delta != 3 {
					t.Fatalf("rename delta = %d, want 3 (the original file stays)", delta)
				}
			}
			if _, err := os.Stat(filepath.Join(root, ".ew-tmp")); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		})
	}
}

func TestChunkOrderingErrors(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(t.TempDir())
	session, err := manager.Create(root, "file.bin", 6, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Chunk(session.ID, 0, bytes.NewBufferString("abc")); err != nil {
		t.Fatal(err)
	}
	// Replaying an already accepted chunk is a duplicate, not a resume.
	if _, err = manager.Chunk(session.ID, 0, bytes.NewBufferString("abc")); err == nil {
		t.Fatal("duplicate chunk accepted")
	}
	// A missing middle chunk shows up when the upload is completed.
	if _, err = manager.Complete(session.ID); err == nil {
		t.Fatal("incomplete upload completed")
	}
	if _, err := os.Stat(filepath.Join(root, "file.bin")); !os.IsNotExist(err) {
		t.Fatal("incomplete upload published a destination file")
	}
}

func TestUnknownSessionRejected(t *testing.T) {
	manager := NewManager(t.TempDir())
	if _, err := manager.Chunk("does-not-exist", 0, bytes.NewBufferString("x")); err == nil {
		t.Fatal("unknown session accepted a chunk")
	}
	if _, err := manager.Complete("does-not-exist"); err == nil {
		t.Fatal("unknown session completed")
	}
}
