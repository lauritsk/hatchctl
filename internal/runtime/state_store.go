package runtime

import "github.com/lauritsk/hatchctl/internal/devcontainer"

type workspaceStateStore struct{}

func (workspaceStateStore) EnsureDir(stateDir string) error {
	return ensureDir(stateDir)
}

func (workspaceStateStore) Read(stateDir string) (devcontainer.State, error) {
	return devcontainer.ReadState(stateDir)
}

func (workspaceStateStore) Write(stateDir string, state devcontainer.State) error {
	return devcontainer.WriteState(stateDir, state)
}
