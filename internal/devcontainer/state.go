package devcontainer

import storefs "github.com/lauritsk/hatchctl/internal/store/fs"

type (
	State           = storefs.WorkspaceState
	StateTransition = storefs.StateTransition
)

func ReadState(stateDir string) (State, error) {
	return storefs.ReadWorkspaceState(stateDir)
}

func WriteState(stateDir string, state State) error {
	return storefs.WriteWorkspaceState(stateDir, state)
}
