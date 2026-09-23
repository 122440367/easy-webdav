package quota

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lecritus/easy-webdav/internal/store"
)

var ErrExceeded = errors.New("quota exceeded")

type Manager struct {
	Store *store.Store
	mu    sync.Mutex
}

func (m *Manager) Used(ctx context.Context, root string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Store.Usage(ctx, root)
}
func (m *Manager) Add(ctx context.Context, root string, delta int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	used, err := m.Store.Usage(ctx, root)
	if err != nil {
		return err
	}
	if used+delta < 0 {
		delta = -used
	}
	return m.Store.AddUsage(ctx, root, delta)
}
func (m *Manager) Reserve(ctx context.Context, root string, used, quota, delta int64) error {
	if quota > 0 && used+delta > quota {
		return ErrExceeded
	}
	return nil
}

type CountingWriter struct {
	Writer io.Writer
	Count  int64
	Limit  int64
}

// AtomicWrite writes into the storage directory and publishes the result only
// after the complete stream passes the quota limit.
func AtomicWrite(dst string, src io.Reader, limit int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".ew-write-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	counter := &CountingWriter{Writer: tmp, Limit: limit}
	_, copyErr := io.Copy(counter, src)
	closeErr := tmp.Close()
	if copyErr != nil {
		return counter.Count, copyErr
	}
	if closeErr != nil {
		return counter.Count, closeErr
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return counter.Count, err
	}
	return counter.Count, nil
}

func CleanupTemp(root string, olderThan time.Duration) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "." || entry.Name() == ".." {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
	return nil
}

func (w *CountingWriter) Write(p []byte) (int, error) {
	if w.Limit > 0 && w.Count+int64(len(p)) > w.Limit {
		return 0, ErrExceeded
	}
	n, err := w.Writer.Write(p)
	w.Count += int64(n)
	return n, err
}
