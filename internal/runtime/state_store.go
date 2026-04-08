package runtime

import "github.com/lauritsk/hatchctl/internal/devcontainer"

type workspaceStateStore interface {
	EnsureDir(string) error
	Read(string) (devcontainer.State, error)
	Write(string, devcontainer.State) error
}

type devcontainerStateStore struct{}

func (devcontainerStateStore) EnsureDir(stateDir string) error {
	return ensureDir(stateDir)
}

func (devcontainerStateStore) Read(stateDir string) (devcontainer.State, error) {
	return devcontainer.ReadState(stateDir)
}

func (devcontainerStateStore) Write(stateDir string, state devcontainer.State) error {
	return devcontainer.WriteState(stateDir, state)
}
