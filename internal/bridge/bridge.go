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
	MountPath  string `json:"mountPath"`
	BinPath    string `json:"binPath"`
	Status     string `json:"status"`
}

type Report = Session

const containerBridgeMountPath = "/var/run/hatchctl/bridge"

func Prepare(stateDir string, enabled bool) (*Session, error) {
	if err := os.MkdirAll(filepath.Join(stateDir, "bridge"), 0o755); err != nil {
		return nil, err
	}
	binPath := filepath.Join(stateDir, "bridge", "bin")
	if err := os.MkdirAll(binPath, 0o755); err != nil {
		return nil, err
	}
	helperPath := filepath.Join(binPath, "devcontainer-open")
	if err := os.WriteFile(helperPath, []byte(openShim()), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(binPath, "xdg-open"), []byte(xdgOpenShim()), 0o755); err != nil {
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
		MountPath:  containerBridgeMountPath,
		BinPath:    filepath.ToSlash(filepath.Join(containerBridgeMountPath, "bin")),
		Status:     status,
	}, nil
}

func Apply(stateDir string, enabled bool, merged devcontainer.MergedConfig) (*Session, devcontainer.MergedConfig, error) {
	if !enabled {
		return nil, merged, nil
	}
	session, err := Prepare(stateDir, enabled)
	if err != nil {
		return nil, devcontainer.MergedConfig{}, err
	}
	if session == nil {
		return session, merged, nil
	}
	containerEnv := cloneEnv(merged.ContainerEnv)
	containerEnv["BROWSER"] = filepath.ToSlash(filepath.Join(session.BinPath, "devcontainer-open"))
	containerEnv["DEVCONTAINER_BRIDGE_ENABLED"] = "true"
	containerEnv["PATH"] = prependPath(session.BinPath, containerEnv["PATH"])

	mount := fmt.Sprintf("type=bind,source=%s,target=%s", session.StatePath, session.MountPath)
	merged.ContainerEnv = containerEnv
	merged.Mounts = appendMount(merged.Mounts, mount)
	return session, merged, nil
}

func Doctor(stateDir string) (Report, error) {
	helperPath := filepath.Join(stateDir, "bridge", "bin", "devcontainer-open")
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
		MountPath:  containerBridgeMountPath,
		BinPath:    filepath.ToSlash(filepath.Join(containerBridgeMountPath, "bin")),
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

func xdgOpenShim() string {
	return "#!/bin/sh\nexec /var/run/hatchctl/bridge/bin/devcontainer-open \"$@\"\n"
}

func cloneEnv(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func prependPath(prefix string, existing string) string {
	if existing == "" {
		return prefix + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	for _, entry := range filepath.SplitList(existing) {
		if entry == prefix {
			return existing
		}
	}
	return prefix + ":" + existing
}

func appendMount(mounts []string, mount string) []string {
	for _, existing := range mounts {
		if existing == mount {
			return mounts
		}
	}
	return append(mounts, mount)
}
