package webdav

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	upstream "github.com/122440367/easy-webdav/internal/webdav/xnet"
)

type RestrictedFS struct {
	root     string
	readOnly bool
	dir      upstream.Dir
	quota    int64
	used     *atomic.Int64
	tempRoot string
	commit   func(int64) error
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

func (f *RestrictedFS) SetQuota(quota, used int64, tempRoot string, commit func(int64) error) {
	f.quota = quota
	f.used = &atomic.Int64{}
	f.used.Store(used)
	f.tempRoot = tempRoot
	f.commit = commit
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
	if write && flag&os.O_TRUNC != 0 && f.commit != nil {
		return f.openQuotaFile(p, perm)
	}
	return f.dir.OpenFile(ctx, filepath.ToSlash(filepath.Join("/", strings.TrimPrefix(p, f.root))), flag, perm)
}

func (f *RestrictedFS) openQuotaFile(destination string, perm os.FileMode) (upstream.File, error) {
	if err := os.MkdirAll(f.tempRoot, 0750); err != nil {
		return nil, err
	}
	temp, err := os.CreateTemp(f.tempRoot, "webdav-*")
	if err != nil {
		return nil, err
	}
	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		return nil, err
	}
	var previous int64
	if info, err := os.Stat(destination); err == nil && !info.IsDir() {
		previous = info.Size()
	} else if err != nil && !os.IsNotExist(err) {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		return nil, err
	}
	return &quotaFile{File: temp, temp: temp.Name(), destination: destination, previous: previous, quota: f.quota, used: f.used, commit: f.commit}, nil
}

type quotaFile struct {
	upstream.File
	temp, destination      string
	previous, count, quota int64
	used                   *atomic.Int64
	commit                 func(int64) error
	failed                 bool
	closed                 bool
}

type quotaExceededError struct{}

func (quotaExceededError) Error() string   { return "user quota exceeded" }
func (quotaExceededError) HTTPStatus() int { return 507 }

func (f *quotaFile) Write(data []byte) (int, error) {
	if f.quota > 0 && f.used.Load()+f.count+int64(len(data))-f.previous > f.quota {
		f.failed = true
		return 0, quotaExceededError{}
	}
	n, err := f.File.Write(data)
	f.count += int64(n)
	return n, err
}

func (f *quotaFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	closeErr := f.File.Close()
	if closeErr != nil || f.failed {
		_ = os.Remove(f.temp)
		return closeErr
	}
	if err := os.Rename(f.temp, f.destination); err != nil {
		_ = os.Remove(f.temp)
		return err
	}
	delta := f.count - f.previous
	if err := f.commit(delta); err != nil {
		return err
	}
	f.used.Add(delta)
	return nil
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
	bytes, err := pathSize(p)
	if err != nil {
		return err
	}
	if err = os.RemoveAll(p); err != nil {
		return err
	}
	if bytes > 0 && f.commit != nil {
		if err = f.commit(-bytes); err != nil {
			return err
		}
		f.used.Add(-bytes)
	}
	return nil
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
	var replaced int64
	if info, err := os.Stat(newPath); err == nil {
		if info.IsDir() {
			return os.ErrExist
		}
		replaced = info.Size()
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(old, newPath); err != nil {
		return err
	}
	if replaced > 0 && f.commit != nil {
		if err := f.commit(-replaced); err != nil {
			return err
		}
		f.used.Add(-replaced)
	}
	return nil
}

func pathSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
func (f *RestrictedFS) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	p, e := f.path(name, false)
	if e != nil {
		return nil, e
	}
	return os.Stat(p)
}
