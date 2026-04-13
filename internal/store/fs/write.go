package fs

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
)

func writeFile(path string, data []byte, dirMode os.FileMode, mode os.FileMode) error {
	if dirMode != 0 {
		if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
			return err
		}
	}
	return fileutil.WriteFile(path, data, mode)
}

func writeJSON(path string, value any, dirMode os.FileMode, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, data, dirMode, mode)
}

func writeOptionalJSON(path string, empty bool, value any, dirMode os.FileMode, mode os.FileMode) error {
	if empty {
		return fileutil.RemoveFile(path)
	}
	return writeJSON(path, value, dirMode, mode)
}
