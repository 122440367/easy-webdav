package webdav

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	upstream "github.com/lecritus/easy-webdav/internal/webdav/xnet"
)

type RestrictedFS struct {
	root     string
	readOnly bool
	dir      upstream.Dir
}

func NewRestrictedFS(root string, readOnly bool) (*RestrictedFS, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(absolute, 0750); err != nil {
		return nil, err
	}
	return &RestrictedFS{root: absolute, readOnly: readOnly, dir: upstream.Dir(absolute)}, nil
}
func (f *RestrictedFS) path(name string, allowMissing bool) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." {
		clean = ""
	}
	if filepath.IsAbs(clean) {
		clean = strings.TrimPrefix(clean, string(filepath.Separator))
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", os.ErrNotExist
	}
	target := filepath.Join(f.root, clean)
	if !within(f.root, target) {
		return "", os.ErrNotExist
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err == nil {
		if !within(f.root, resolved) {
			return "", os.ErrNotExist
		}
		return resolved, nil
	}
	if !allowMissing {
		return "", err
	}
	parent := filepath.Dir(target)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || !within(f.root, resolvedParent) {
		return "", os.ErrNotExist
	}
	return target, nil
}
func within(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func (f *RestrictedFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p, e := f.path(name, true)
	if e != nil {
		return e
	}
	return os.Mkdir(p, perm)
}
func (f *RestrictedFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (upstream.File, error) {
	write := flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0
	if f.readOnly && write {
		return nil, os.ErrPermission
	}
	p, e := f.path(name, write)
	if e != nil {
		return nil, e
	}
	return f.dir.OpenFile(ctx, filepath.ToSlash(filepath.Join("/", strings.TrimPrefix(p, f.root))), flag, perm)
}
func (f *RestrictedFS) RemoveAll(ctx context.Context, name string) error {
	if f.readOnly {
		return os.ErrPermission
	}
	p, e := f.path(name, false)
	if e != nil {
		return e
	}
	if filepath.Clean(p) == filepath.Clean(f.root) {
		return errors.New("cannot remove root")
	}
	return os.RemoveAll(p)
}
func (f *RestrictedFS) Rename(ctx context.Context, oldName, newName string) error {
	if f.readOnly {
		return os.ErrPermission
	}
	old, e := f.path(oldName, false)
	if e != nil {
		return e
	}
	newPath, e := f.path(newName, true)
	if e != nil {
		return e
	}
	return os.Rename(old, newPath)
}
func (f *RestrictedFS) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	p, e := f.path(name, false)
	if e != nil {
		return nil, e
	}
	return os.Stat(p)
}
