package fileutil

import (
	"errors"
	"os"
	"path/filepath"
)

func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	tmpPath := tempPath(path)
	if renameErr := os.Rename(tmpPath, path); renameErr == nil {
		return os.ReadFile(path)
	} else if !os.IsNotExist(renameErr) {
		data, readErr := os.ReadFile(tmpPath)
		if readErr == nil {
			return data, nil
		}
		if !os.IsNotExist(readErr) {
			return nil, readErr
		}
	}
	return nil, err
}

func WriteFile(path string, data []byte, perm os.FileMode) error {
	tmpPath := tempPath(path)
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return syncDir(filepath.Dir(path))
}

func RemoveFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	tmpPath := tempPath(path)
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func tempPath(path string) string {
	return path + ".tmp"
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return err
	}
	return nil
}
