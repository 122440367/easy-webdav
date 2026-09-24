package quota

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/122440367/easy-webdav/internal/store"
)

func Recalculate(s *store.Store, storageRoot string) error {
	ctx := context.Background()
	users, err := s.ListUsers(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, u := range users {
		root, err := filepath.Abs(filepath.Join(storageRoot, u.RootDir))
		if err != nil {
			return err
		}
		if seen[root] {
			continue
		}
		seen[root] = true
		var total int64
		err = filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = s.SetUsage(ctx, root, total); err != nil {
			return err
		}
	}
	return CleanupTemp(filepath.Join(storageRoot, ".ew-tmp"), 24*time.Hour)
}
