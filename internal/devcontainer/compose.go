package devcontainer

import (
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type MountSpec = spec.MountSpec

func ComposeOverrideFile(stateDir string) string {
	return storefs.ComposeOverridePath(stateDir)
}

func ResolveComposeFiles(configDir string, raw any) ([]string, error) {
	return spec.ResolveComposeFiles(configDir, raw)
}

func ComposeProjectName(workspace string, configPath string) string {
	return spec.ComposeProjectName(workspace, configPath)
}

func ParseMountSpec(raw string) (MountSpec, bool) {
	return spec.ParseMountSpec(raw)
}
