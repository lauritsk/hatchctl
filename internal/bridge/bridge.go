package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type Session struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	StatePath  string `json:"statePath"`
	HelperPath string `json:"helperPath"`
	Status     string `json:"status"`
}

type Report = Session

func Prepare(stateDir string, enabled bool) (*Session, error) {
	if err := os.MkdirAll(filepath.Join(stateDir, "bridge"), 0o755); err != nil {
		return nil, err
	}
	helperPath := filepath.Join(stateDir, "bridge", "devcontainer-open")
	if err := os.WriteFile(helperPath, []byte(openShim()), 0o755); err != nil {
		return nil, err
	}
	status := "disabled"
	if enabled {
		status = "scaffolded"
	}
	return &Session{
		ID:         devcontainer.ContainerName(stateDir, helperPath),
		Enabled:    enabled,
		StatePath:  filepath.Join(stateDir, "bridge"),
		HelperPath: helperPath,
		Status:     status,
	}, nil
}

func Doctor(stateDir string) (Report, error) {
	helperPath := filepath.Join(stateDir, "bridge", "devcontainer-open")
	_, err := os.Stat(helperPath)
	status := "not configured"
	enabled := false
	if err == nil {
		enabled = true
		status = "scaffolded"
	}
	if err != nil && !os.IsNotExist(err) {
		return Report{}, err
	}
	return Report{
		ID:         devcontainer.ContainerName(stateDir, helperPath),
		Enabled:    enabled,
		StatePath:  filepath.Join(stateDir, "bridge"),
		HelperPath: helperPath,
		Status:     status,
	}, nil
}

func openShim() string {
	launcher := "open"
	if runtime.GOOS != "darwin" {
		launcher = "xdg-open"
	}
	return fmt.Sprintf(`#!/bin/sh
set -eu

if [ $# -lt 1 ]; then
  exit 1
fi

url="$1"
if [ -n "${DEVCONTAINER_BRIDGE_OPEN_COMMAND:-}" ]; then
  DEVCONTAINER_BRIDGE_URL="$url" exec /bin/sh -lc "$DEVCONTAINER_BRIDGE_OPEN_COMMAND"
fi

exec %s "$url"
`, launcher)
}
