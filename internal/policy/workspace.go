package policy

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

const TrustWorkspaceEnvVar = "HATCHCTL_TRUST_WORKSPACE"

var ErrWorkspaceTrustRequired = errors.New("workspace requires explicit trust for repo-controlled container backend settings")

func EnsureWorkspaceTrust(resolved devcontainer.ResolvedConfig, trusted bool) error {
	if trusted || envTruthy(TrustWorkspaceEnvVar) {
		return nil
	}
	issues := workspaceTrustIssues(spec.WorkspaceSpec{
		WorkspaceFolder: resolved.WorkspaceFolder,
		ConfigPath:      resolved.ConfigPath,
		ConfigDir:       resolved.ConfigDir,
		Config:          resolved.Config,
		Merged:          resolved.Merged,
		WorkspaceMount:  resolved.WorkspaceMount,
		RemoteWorkspace: resolved.RemoteWorkspace,
		SourceKind:      resolved.SourceKind,
		ComposeFiles:    resolved.ComposeFiles,
		ComposeService:  resolved.ComposeService,
	})
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("%w\nDetected settings:\n- %s\nReview the workspace config and rerun with --trust-workspace or set %s=1 if you want to allow these settings.", ErrWorkspaceTrustRequired, strings.Join(issues, "\n- "), TrustWorkspaceEnvVar)
}

func WorkspaceTrustRequiredForSpec(workspaceSpec spec.WorkspaceSpec) bool {
	return len(workspaceTrustIssues(workspaceSpec)) > 0
}

func workspaceTrustIssues(workspaceSpec spec.WorkspaceSpec) []string {
	issues := make([]string, 0)
	if workspaceSpec.Config.WorkspaceMount != "" {
		issues = append(issues, "workspaceMount overrides the default workspace bind mount")
	}
	if workspaceSpec.Merged.Privileged {
		issues = append(issues, "privileged container mode is enabled")
	}
	if len(workspaceSpec.Merged.CapAdd) > 0 {
		issues = append(issues, fmt.Sprintf("additional Linux capabilities requested (%s)", strings.Join(workspaceSpec.Merged.CapAdd, ", ")))
	}
	if len(workspaceSpec.Merged.SecurityOpt) > 0 {
		issues = append(issues, fmt.Sprintf("custom security options requested (%s)", strings.Join(workspaceSpec.Merged.SecurityOpt, ", ")))
	}
	if bindMounts := riskyBindMounts(workspaceSpec.Merged.Mounts); len(bindMounts) > 0 {
		issues = append(issues, fmt.Sprintf("bind mounts requested (%s)", strings.Join(bindMounts, ", ")))
	}
	if risky := riskyRunArgs(workspaceSpec.Config.RunArgs); len(risky) > 0 {
		issues = append(issues, fmt.Sprintf("container runtime arguments request host-affecting settings (%s)", strings.Join(risky, ", ")))
	}
	if buildIssue := riskyBuildSettings(workspaceSpec); buildIssue != "" {
		issues = append(issues, buildIssue)
	}
	return issues
}

func riskyBindMounts(mounts []string) []string {
	result := make([]string, 0)
	for _, mount := range mounts {
		mountSpec, ok := spec.ParseMountSpec(mount)
		if ok && mountSpec.Type == "bind" {
			result = append(result, mount)
		}
	}
	return result
}

func riskyRunArgs(args []string) []string {
	result := make([]string, 0)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		switch {
		case arg == "--privileged":
			result = append(result, arg)
		case arg == "--mount" || arg == "-v" || arg == "--volume" || arg == "--cap-add" || arg == "--security-opt" || arg == "--device":
			result = append(result, strings.TrimSpace(arg+" "+next))
			i++
		case (arg == "--pid" || arg == "--network" || arg == "--userns") && strings.EqualFold(next, "host"):
			result = append(result, arg+" "+next)
			i++
		case strings.HasPrefix(arg, "--mount=") || strings.HasPrefix(arg, "--volume=") || strings.HasPrefix(arg, "-v=") || strings.HasPrefix(arg, "--cap-add=") || strings.HasPrefix(arg, "--security-opt=") || strings.HasPrefix(arg, "--device=") || strings.HasPrefix(arg, "--pid=") || strings.HasPrefix(arg, "--network=") || strings.HasPrefix(arg, "--userns="):
			if strings.HasPrefix(arg, "--pid=") || strings.HasPrefix(arg, "--network=") || strings.HasPrefix(arg, "--userns=") {
				if !strings.HasSuffix(strings.ToLower(arg), "=host") {
					continue
				}
			}
			result = append(result, arg)
		}
	}
	return result
}

func riskyBuildSettings(workspaceSpec spec.WorkspaceSpec) string {
	if workspaceSpec.Config.Build != nil && len(workspaceSpec.Config.Build.Options) > 0 {
		return fmt.Sprintf("container build options requested (%s)", strings.Join(workspaceSpec.Config.Build.Options, ", "))
	}
	dockerfilePath := resolveConfigRelativePath(workspaceSpec.ConfigDir, spec.EffectiveDockerfile(workspaceSpec.Config))
	if outsideWorkspace(workspaceSpec.WorkspaceFolder, dockerfilePath) {
		return fmt.Sprintf("build definition path resolves outside the workspace (%s)", dockerfilePath)
	}
	contextPath := resolveConfigRelativePath(workspaceSpec.ConfigDir, spec.EffectiveContext(workspaceSpec.Config))
	if outsideWorkspace(workspaceSpec.WorkspaceFolder, contextPath) {
		return fmt.Sprintf("build context resolves outside the workspace (%s)", contextPath)
	}
	return ""
}

func resolveConfigRelativePath(base string, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(base, value)
}

func outsideWorkspace(workspace string, target string) bool {
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return true
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return true
	}
	rel, err := filepath.Rel(workspaceAbs, targetAbs)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
