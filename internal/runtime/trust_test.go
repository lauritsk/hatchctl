package runtime

import (
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/policy"
)

func TestEnsureWorkspaceTrustRejectsPrivilegedWorkspaceByDefault(t *testing.T) {
	t.Parallel()

	resolved := devcontainer.ResolvedConfig{
		WorkspaceFolder: t.TempDir(),
		ConfigDir:       t.TempDir(),
		Config:          devcontainer.Config{RunArgs: []string{"--network", "host"}},
		Merged:          devcontainer.MergedConfig{Privileged: true},
	}
	if err := policy.EnsureWorkspaceTrust(resolved, false); err == nil || !strings.Contains(err.Error(), "requires explicit trust") {
		t.Fatalf("expected trust error, got %v", err)
	}
}

func TestEnsureWorkspaceTrustRejectsBuildContextOutsideWorkspace(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	resolved := devcontainer.ResolvedConfig{
		WorkspaceFolder: workspace,
		ConfigDir:       workspace,
		Config: devcontainer.Config{
			Build: &devcontainer.BuildConfig{Context: "../outside"},
		},
	}
	if err := policy.EnsureWorkspaceTrust(resolved, false); err == nil || !strings.Contains(err.Error(), "build context resolves outside the workspace") {
		t.Fatalf("expected build trust error, got %v", err)
	}
}

func TestEnsureWorkspaceTrustAllowsExplicitTrust(t *testing.T) {
	t.Parallel()

	resolved := devcontainer.ResolvedConfig{
		WorkspaceFolder: t.TempDir(),
		ConfigDir:       t.TempDir(),
		Config:          devcontainer.Config{WorkspaceMount: "type=bind,source=/tmp,target=/workspace"},
	}
	if err := policy.EnsureWorkspaceTrust(resolved, true); err != nil {
		t.Fatalf("expected trusted workspace to pass, got %v", err)
	}
}
