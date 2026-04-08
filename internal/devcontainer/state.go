package devcontainer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

func WorkspaceStateDir(workspace string, configPath string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	key := hashKey(workspace + "\n" + configPath)
	return filepath.Join(cacheDir, "hatchctl", "workspaces", key), nil
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
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(stateDir, "state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, 0o644)
}

func hashKey(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:8])
}
