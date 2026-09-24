package webdav

import (
	"path/filepath"
	"strings"
	"time"

	upstream "github.com/lecritus/easy-webdav/internal/webdav/xnet"
)

type scopedLocks struct {
	base  string
	inner upstream.LockSystem
}

func (l scopedLocks) name(value string) string {
	value = strings.TrimPrefix(value, "/")
	return filepath.ToSlash(filepath.Join(l.base, value))
}
func (l scopedLocks) Confirm(now time.Time, a, b string, c ...upstream.Condition) (func(), error) {
	// Empty names must stay empty: routing them through name() would turn
	// them into the user root itself and fail lock confirmation (412).
	if a != "" {
		a = l.name(a)
	}
	if b != "" {
		b = l.name(b)
	}
	return l.inner.Confirm(now, a, b, c...)
}
func (l scopedLocks) Create(now time.Time, d upstream.LockDetails) (string, error) {
	d.Root = l.name(d.Root)
	return l.inner.Create(now, d)
}
func (l scopedLocks) Refresh(now time.Time, t string, d time.Duration) (upstream.LockDetails, error) {
	return l.inner.Refresh(now, t, d)
}
func (l scopedLocks) Unlock(now time.Time, t string) error { return l.inner.Unlock(now, t) }
