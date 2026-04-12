package devcontainer

import storefs "github.com/lauritsk/hatchctl/internal/store/fs"

type (
	State           = storefs.WorkspaceState
	StateTransition = storefs.StateTransition
)
