package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lauritsk/hatchctl/internal/fileutil"
)

type BridgePaths struct {
	Dir         string
	BinDir      string
	SessionPath string
	ConfigPath  string
	PIDPath     string
	StatusPath  string
}

func WorkspaceBridgePaths(stateDir string) BridgePaths {
	bridgeDir := filepath.Join(stateDir, "bridge")
	return BridgePaths{
		Dir:         bridgeDir,
		BinDir:      filepath.Join(bridgeDir, "bin"),
		SessionPath: filepath.Join(bridgeDir, "session.json"),
		ConfigPath:  filepath.Join(bridgeDir, "bridge-config.json"),
		PIDPath:     filepath.Join(bridgeDir, "bridge.pid"),
		StatusPath:  filepath.Join(bridgeDir, "bridge-status.json"),
	}
}

func EnsureWorkspaceBridgePaths(stateDir string) (BridgePaths, error) {
	paths := WorkspaceBridgePaths(stateDir)
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		return BridgePaths{}, err
	}
	if err := os.MkdirAll(paths.BinDir, 0o755); err != nil {
		return BridgePaths{}, err
	}
	return paths, nil
}

func WriteBridgeExecutable(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, 0o755)
}

func ReadBridgeSession[T any](bridgeDir string) (*T, error) {
	data, err := fileutil.ReadFile(filepath.Join(bridgeDir, "session.json"))
	if err != nil {
		return nil, err
	}
	var session T
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func WriteBridgeSession(bridgeDir string, session any) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(filepath.Join(bridgeDir, "session.json"), data, 0o600)
}

func ReadBridgePID(pidPath string) (int, error) {
	data, err := fileutil.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(trimASCIIWhitespace(data)))
	if err != nil {
		return 0, nil
	}
	return pid, nil
}

func WriteBridgePID(pidPath string, pid int) error {
	return fileutil.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o600)
}

func WriteBridgeConfig(configPath string, config any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(configPath, data, 0o600)
}

func WriteBridgeStatus(statusPath string, status any) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o700); err != nil {
		return err
	}
	return fileutil.WriteFile(statusPath, data, 0o600)
}

func ReadBridgeStatus(statusPath string) ([]byte, error) {
	return fileutil.ReadFile(statusPath)
}

func trimASCIIWhitespace(data []byte) string {
	start := 0
	for start < len(data) && isASCIIWhitespace(data[start]) {
		start++
	}
	end := len(data)
	for end > start && isASCIIWhitespace(data[end-1]) {
		end--
	}
	return string(data[start:end])
}

func isASCIIWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}
