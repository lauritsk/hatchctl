package plan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func TestBuildWorkspacePlanMapsExplicitIntentAndArtifacts(t *testing.T) {
	t.Parallel()

	workspace, configPath := newPlanTestWorkspace(t, `{
		"dockerFile": "Dockerfile",
		"workspaceFolder": "/workspaces/demo",
		"initializeCommand": "echo init"
	}`)
	workspacePlan, err := BuildWorkspacePlan(BuildWorkspacePlanRequest{
		Workspace:          workspace,
		ConfigPath:         configPath,
		StateBaseDir:       "/state-root",
		CacheBaseDir:       "/cache-root",
		FeatureTimeout:     45 * time.Second,
		LockfilePolicy:     devcontainer.FeatureLockfilePolicyUpdate,
		BridgeEnabled:      true,
		SSHAgent:           true,
		Dotfiles:           DotfilesPreference{Repository: "https://github.com/example/dotfiles.git", TargetPath: "$HOME/.dotfiles"},
		TrustWorkspace:     true,
		AllowHostLifecycle: true,
	})
	if err != nil {
		t.Fatalf("build workspace plan: %v", err)
	}
	if workspacePlan.ReadOnly {
		t.Fatal("expected mutating plan")
	}
	if workspacePlan.FeatureMaterialization != FeatureMaterializationRefresh {
		t.Fatalf("unexpected materialization mode %q", workspacePlan.FeatureMaterialization)
	}
	if workspacePlan.Immutable.Workspace != workspace || workspacePlan.Immutable.ConfigPath != configPath {
		t.Fatalf("unexpected immutable inputs %#v", workspacePlan.Immutable)
	}
	if workspacePlan.Immutable.Spec.SourceKind != "dockerfile" {
		t.Fatalf("unexpected spec source kind %#v", workspacePlan.Immutable.Spec)
	}
	if workspacePlan.LockProtected.StateBaseDir != "/state-root" || workspacePlan.LockProtected.CacheBaseDir != "/cache-root" {
		t.Fatalf("unexpected lock-protected roots %#v", workspacePlan.LockProtected)
	}
	if got := filepath.Dir(workspacePlan.LockProtected.StateDir); got != filepath.Join("/state-root", "workspaces") {
		t.Fatalf("unexpected state dir %q", workspacePlan.LockProtected.StateDir)
	}
	if !workspacePlan.LockProtected.UsesPlanCache || !workspacePlan.LockProtected.UsesFeatureLock || !workspacePlan.LockProtected.UsesFeatureState || !workspacePlan.LockProtected.RequiresRevalidation {
		t.Fatalf("unexpected lock-protected artifacts %#v", workspacePlan.LockProtected)
	}
	if !workspacePlan.Preferences.BridgeEnabled || !workspacePlan.Preferences.SSHAgent || workspacePlan.Preferences.Dotfiles.Repository == "" {
		t.Fatalf("unexpected preferences %#v", workspacePlan.Preferences)
	}
	if !workspacePlan.Trust.WorkspaceAllowed || !workspacePlan.Trust.HostLifecycleAllowed || !workspacePlan.Trust.HostLifecycleRequired {
		t.Fatalf("unexpected trust plan %#v", workspacePlan.Trust)
	}
}

func TestBuildWorkspacePlanMarksReadOnlyIntentSeparatelyFromMaterialization(t *testing.T) {
	t.Parallel()

	workspace, configPath := newPlanTestWorkspace(t, `{"image":"alpine:3.20","workspaceFolder":"/workspaces/demo"}`)
	workspacePlan, err := BuildWorkspacePlan(BuildWorkspacePlanRequest{
		Workspace:      workspace,
		ConfigPath:     configPath,
		LockfilePolicy: devcontainer.FeatureLockfilePolicyAuto,
		ReadOnly:       true,
	})
	if err != nil {
		t.Fatalf("build workspace plan: %v", err)
	}
	if !workspacePlan.ReadOnly {
		t.Fatal("expected read-only plan")
	}
	if workspacePlan.FeatureMaterialization != FeatureMaterializationReuse {
		t.Fatalf("unexpected materialization mode %q", workspacePlan.FeatureMaterialization)
	}
	if workspacePlan.LockProtected.UsesFeatureState || workspacePlan.LockProtected.RequiresRevalidation {
		t.Fatalf("unexpected read-only lock-protected artifacts %#v", workspacePlan.LockProtected)
	}
	resolveOpts := workspacePlan.ResolveOptions(nil)
	if resolveOpts.AllowNetwork || resolveOpts.WritePlanCache || resolveOpts.WriteFeatureLock || resolveOpts.WriteFeatureState {
		t.Fatalf("unexpected read-only resolve options %#v", resolveOpts)
	}
	if resolveOpts.LockfilePolicy != devcontainer.FeatureLockfilePolicyAuto {
		t.Fatalf("unexpected lockfile policy %q", resolveOpts.LockfilePolicy)
	}
	if workspacePlan.Trust.WorkspaceRequired {
		t.Fatalf("did not expect trust requirement %#v", workspacePlan.Trust)
	}
	if workspacePlan.Immutable.Spec.SourceKind == "" {
		t.Fatalf("expected resolved workspace spec %#v", workspacePlan.Immutable.Spec)
	}
}

func newPlanTestWorkspace(t *testing.T, config string) (string, string) {
	t.Helper()

	workspace := t.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace, configPath
}
