package devcontainer

import (
	"path/filepath"

	"github.com/lauritsk/hatchctl/internal/spec"
)

type MountSpec = spec.MountSpec

func ComposeOverrideFile(stateDir string) string {
	return filepath.Join(stateDir, "docker-compose.override.yml")
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
