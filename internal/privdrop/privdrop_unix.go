//go:build !windows

// Package privdrop implements the container-friendly PUID/PGID handover: when
// the process starts as root with EW_PUID/EW_PGID set, the data directory is
// handed to that account and the process drops its privileges before serving.
package privdrop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const defaultUID, defaultGID = 1000, 1000

func number(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s: %q is not a valid id", name, raw)
	}
	return value, nil
}

// MaybeDrop returns nil without doing anything unless the process runs as root
// and PUID/PGID were provided.
func MaybeDrop(dataDir string) error {
	if os.Getenv("EW_PUID") == "" && os.Getenv("EW_PGID") == "" {
		return nil
	}
	if os.Geteuid() != 0 {
		return nil
	}
	uid, err := number("EW_PUID", defaultUID)
	if err != nil {
		return err
	}
	gid, err := number("EW_PGID", defaultGID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dataDir, 0o750); err != nil {
		return err
	}
	// Only the data directory and its direct children are handed over: a full
	// recursive walk of a large file tree on every start is not acceptable.
	if err = os.Chown(dataDir, uid, gid); err != nil {
		return err
	}
	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, entry := range entries {
			_ = os.Lchown(filepath.Join(dataDir, entry.Name()), uid, gid)
		}
	}
	if err = syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err = syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid: %w", err)
	}
	if err = syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid: %w", err)
	}
	return nil
}
