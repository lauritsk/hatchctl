package devcontainer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lauritsk/hatchctl/internal/fileutil"
	"github.com/tailscale/hujson"
)

type State struct {
	ContainerID     string `json:"containerId,omitempty"`
	LifecycleReady  bool   `json:"lifecycleReady,omitempty"`
	BridgeEnabled   bool   `json:"bridgeEnabled,omitempty"`
	BridgeSessionID string `json:"bridgeSessionId,omitempty"`
	DotfilesReady   bool   `json:"dotfilesReady,omitempty"`
	DotfilesRepo    string `json:"dotfilesRepo,omitempty"`
	DotfilesInstall string `json:"dotfilesInstall,omitempty"`
	DotfilesTarget  string `json:"dotfilesTarget,omitempty"`
}

type OutputRoots struct {
	StateRoot string
	CacheRoot string
}

func WorkspaceStateDir(workspace string, configPath string) (string, error) {
	roots, err := DefaultOutputRoots()
	if err != nil {
		return "", err
	}
	return workspaceScopedDir(roots.StateRoot, workspace, configPath), nil
}

func WorkspaceCacheDir(workspace string, configPath string) (string, error) {
	roots, err := DefaultOutputRoots()
	if err != nil {
		return "", err
	}
	return workspaceScopedDir(roots.CacheRoot, workspace, configPath), nil
}

func DefaultOutputRoots() (OutputRoots, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return OutputRoots{}, err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return OutputRoots{}, err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return OutputRoots{}, err
	}
	return outputRoots(runtime.GOOS, homeDir, configDir, cacheDir, os.Getenv("XDG_STATE_HOME")), nil
}

func outputRoots(goos string, homeDir string, configDir string, cacheDir string, xdgStateHome string) OutputRoots {
	configRoot := filepath.Join(configDir, "hatchctl")
	cacheRoot := filepath.Join(cacheDir, "hatchctl")
	stateRoot := configRoot
	if goos != "darwin" {
		if xdgStateHome != "" {
			stateRoot = filepath.Join(xdgStateHome, "hatchctl")
		} else if homeDir != "" {
			stateRoot = filepath.Join(homeDir, ".local", "state", "hatchctl")
		}
	}
	return OutputRoots{StateRoot: stateRoot, CacheRoot: cacheRoot}
}

func workspaceScopedDir(root string, workspace string, configPath string) string {
	key := hashKey(workspace + "\n" + configPath)
	return filepath.Join(root, "workspaces", key)
}

func ContainerName(workspace string, configPath string) string {
	return fmt.Sprintf("hatchctl-%s", hashKey(workspace+"\n"+configPath))
}

func ImageName(workspace string, configPath string) string {
	return fmt.Sprintf("hatchctl-%s", hashKey(workspace+"\n"+configPath))
}

func ReadState(stateDir string) (State, error) {
	path := filepath.Join(stateDir, "state.json")
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	data, err = hujson.Standardize(data)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func WriteState(stateDir string, state State) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(stateDir, "state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, 0o600)
}

func hashKey(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:8])
}
