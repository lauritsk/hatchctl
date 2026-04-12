package devcontainer

import storefs "github.com/lauritsk/hatchctl/internal/store/fs"

func ComposeOverrideFile(stateDir string) string {
	return storefs.ComposeOverridePath(stateDir)
}
