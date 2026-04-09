package fs

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/fileutil"
	"github.com/tailscale/hujson"
)

type StateTransition struct {
	Kind string `json:"kind,omitempty"`
	Key  string `json:"key,omitempty"`
}

type WorkspaceState struct {
	Version             int              `json:"version,omitempty"`
	ContainerID         string           `json:"containerId,omitempty"`
	ContainerKey        string           `json:"containerKey,omitempty"`
	LifecycleReady      bool             `json:"lifecycleReady,omitempty"`
	LifecycleKey        string           `json:"lifecycleKey,omitempty"`
	LifecycleTransition *StateTransition `json:"lifecycleTransition,omitempty"`
	BridgeEnabled       bool             `json:"bridgeEnabled,omitempty"`
	BridgeSessionID     string           `json:"bridgeSessionId,omitempty"`
	BridgeTransition    *StateTransition `json:"bridgeTransition,omitempty"`
	DotfilesReady       bool             `json:"dotfilesReady,omitempty"`
	DotfilesRepo        string           `json:"dotfilesRepo,omitempty"`
	DotfilesInstall     string           `json:"dotfilesInstall,omitempty"`
	DotfilesTarget      string           `json:"dotfilesTarget,omitempty"`
	DotfilesTransition  *StateTransition `json:"dotfilesTransition,omitempty"`
}

const workspaceStateSchemaVersion = 2

func ReadWorkspaceState(stateDir string) (WorkspaceState, error) {
	path := filepath.Join(stateDir, "state.json")
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkspaceState{}, nil
		}
		return WorkspaceState{}, err
	}
	data, err = hujson.Standardize(data)
	if err != nil {
		return WorkspaceState{}, err
	}
	var state WorkspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		return WorkspaceState{}, err
	}
	if state.Version == 0 {
		state.Version = workspaceStateSchemaVersion
	}
	return state, nil
}

func WriteWorkspaceState(stateDir string, state WorkspaceState) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(stateDir, "state.json")
	if state.Version == 0 {
		state.Version = workspaceStateSchemaVersion
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, 0o600)
}
