package spec

import "github.com/lauritsk/hatchctl/internal/devcontainer"

const ImageMetadataLabel = devcontainer.ImageMetadataLabel

type (
	Config           = devcontainer.Config
	BuildConfig      = devcontainer.BuildConfig
	LifecycleCommand = devcontainer.LifecycleCommand
	ForwardPorts     = devcontainer.ForwardPorts
	LifecycleStep    = devcontainer.LifecycleStep
	MetadataEntry    = devcontainer.MetadataEntry
	MergedConfig     = devcontainer.MergedConfig
	MountSpec        = devcontainer.MountSpec
	WorkspaceSpec    = devcontainer.WorkspaceSpec
)

func StandardizeJSONC(path string, data []byte) ([]byte, error) {
	return devcontainer.StandardizeJSONC(path, data)
}

func ResolveComposeFiles(configDir string, raw any) ([]string, error) {
	return devcontainer.ResolveComposeFiles(configDir, raw)
}

func ComposeProjectName(workspace string, configPath string) string {
	return devcontainer.ComposeProjectName(workspace, configPath)
}

func ParseMountSpec(raw string) (MountSpec, bool) {
	return devcontainer.ParseMountSpec(raw)
}

func ResolveWorkspaceSpec(workspaceArg string, configArg string) (WorkspaceSpec, error) {
	return devcontainer.ResolveWorkspaceSpec(workspaceArg, configArg)
}

func Load(configPath string) (Config, error) {
	return devcontainer.Load(configPath)
}

func ResolveWorkspacePath(workspaceArg string) (string, error) {
	return devcontainer.ResolveWorkspacePath(workspaceArg)
}

func ResolveConfigPath(workspace string, configArg string) (string, error) {
	return devcontainer.ResolveConfigPath(workspace, configArg)
}

func EffectiveDockerfile(config Config) string {
	return devcontainer.EffectiveDockerfile(config)
}

func ResolvedDockerfile(configDir string, config Config) string {
	return devcontainer.ResolvedDockerfile(configDir, config)
}

func EffectiveContext(config Config) string {
	return devcontainer.EffectiveContext(config)
}

func ContainerCommand(config Config) []string {
	return devcontainer.ContainerCommand(config)
}

func KeepAliveCommand() string {
	return devcontainer.KeepAliveCommand()
}

func RemoteExecUser(config Config) string {
	return devcontainer.RemoteExecUser(config)
}

func ShellQuote(value string) string {
	return devcontainer.ShellQuote(value)
}

func NormalizeForwardPorts(raw []any) (ForwardPorts, error) {
	return devcontainer.NormalizeForwardPorts(raw)
}

func MergeForwardPorts(entries ...ForwardPorts) ForwardPorts {
	return devcontainer.MergeForwardPorts(entries...)
}

func MetadataFromLabel(value string) ([]MetadataEntry, error) {
	return devcontainer.MetadataFromLabel(value)
}

func MetadataLabelValue(entries []MetadataEntry) (string, error) {
	return devcontainer.MetadataLabelValue(entries)
}

func ConfigMetadata(config Config) MetadataEntry {
	return devcontainer.ConfigMetadata(config)
}

func MergeMetadata(config Config, metadata []MetadataEntry) MergedConfig {
	return devcontainer.MergeMetadata(config, metadata)
}

func SortedMapKeys(values map[string]string) []string {
	return devcontainer.SortedMapKeys(values)
}
