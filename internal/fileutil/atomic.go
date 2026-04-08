package fileutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

type WriteOptions struct {
	Mode    os.FileMode
	DirMode os.FileMode
}

func WriteFileAtomic(path string, data []byte, opts WriteOptions) (err error) {
	dir := filepath.Dir(path)
	dirMode := opts.DirMode
	if dirMode == 0 {
		dirMode = 0o755
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	if opts.Mode != 0 {
		if err := tmp.Chmod(opts.Mode); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !ignorableSyncError(err) {
		return err
	}
	return nil
}

func ignorableSyncError(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP)
}
