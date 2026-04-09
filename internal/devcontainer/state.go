package devcontainer

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
	"github.com/tailscale/hujson"
)

type State struct {
	Version         int              `json:"version,omitempty"`
	ContainerID     string           `json:"containerId,omitempty"`
	ContainerKey    string           `json:"containerKey,omitempty"`
	LifecycleReady  bool             `json:"lifecycleReady,omitempty"`
	LifecycleKey    string           `json:"lifecycleKey,omitempty"`
	Transition      *StateTransition `json:"transition,omitempty"`
	BridgeEnabled   bool             `json:"bridgeEnabled,omitempty"`
	BridgeSessionID string           `json:"bridgeSessionId,omitempty"`
	DotfilesReady   bool             `json:"dotfilesReady,omitempty"`
	DotfilesRepo    string           `json:"dotfilesRepo,omitempty"`
	DotfilesInstall string           `json:"dotfilesInstall,omitempty"`
	DotfilesTarget  string           `json:"dotfilesTarget,omitempty"`
}

type StateTransition struct {
	Kind string `json:"kind,omitempty"`
	Key  string `json:"key,omitempty"`
}

const stateSchemaVersion = 1

type OutputRoots = storefs.OutputRoots

func WorkspaceStateDir(workspace string, configPath string) (string, error) {
	return storefs.WorkspaceStateDir(workspace, configPath)
}

func WorkspaceCacheDir(workspace string, configPath string) (string, error) {
	return storefs.WorkspaceCacheDir(workspace, configPath)
}

func DefaultOutputRoots() (OutputRoots, error) {
	return storefs.DefaultOutputRoots()
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
	return storefs.WorkspaceScopedDir(root, workspace, configPath)
}

func ContainerName(workspace string, configPath string) string {
	return storefs.ContainerName(workspace, configPath)
}

func ImageName(workspace string, configPath string) string {
	return storefs.ImageName(workspace, configPath)
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
	if state.Version == 0 {
		state.Version = stateSchemaVersion
	}
	return state, nil
}

func WriteState(stateDir string, state State) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(stateDir, "state.json")
	if state.Version == 0 {
		state.Version = stateSchemaVersion
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, 0o600)
}
