package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"go.yaml.in/yaml/v3"
)

func TestRenderComposeOverridePreservesMountSemantics(t *testing.T) {
	resolved := devcontainer.ResolvedConfig{
		ComposeService: "app",
		WorkspaceMount: "type=bind,source=/workspace,target=/workspaces/demo,readonly=true,bind-propagation=rshared,create-host-path=false",
		Labels:         map[string]string{"a": "b"},
		Merged: devcontainer.MergedConfig{
			Mounts: []string{"type=volume,source=cache,target=/cache,readonly=true,nocopy=true,subpath=tooling"},
		},
	}

	contents, err := renderComposeOverride(resolved, "image:latest")
	if err != nil {
		t.Fatalf("render compose override: %v", err)
	}

	var override struct {
		Services map[string]struct {
			Volumes []composeServiceMount `yaml:"volumes"`
			Image   string                `yaml:"image"`
		} `yaml:"services"`
		Volumes map[string]map[string]any `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(contents), &override); err != nil {
		t.Fatalf("unmarshal compose override: %v", err)
	}
	service := override.Services["app"]
	if service.Image != "image:latest" {
		t.Fatalf("unexpected service image %q", service.Image)
	}
	if len(service.Volumes) != 2 {
		t.Fatalf("unexpected service volumes %#v", service.Volumes)
	}
	workspace := service.Volumes[0]
	if !workspace.ReadOnly || workspace.Bind == nil || workspace.Bind.Propagation != "rshared" || workspace.Bind.CreateHostPath == nil || *workspace.Bind.CreateHostPath {
		t.Fatalf("unexpected bind mount %#v", workspace)
	}
	cache := service.Volumes[1]
	if cache.Volume == nil || !cache.ReadOnly || !cache.Volume.NoCopy || cache.Volume.Subpath != "tooling" {
		t.Fatalf("unexpected volume mount %#v", cache)
	}
	if _, ok := override.Volumes["cache"]; !ok {
		t.Fatalf("expected named volume declaration, got %#v", override.Volumes)
	}
}

func TestComposeMountValueSkipsUnsupportedMounts(t *testing.T) {
	if _, ok := composeMountValue("type=tmpfs,target=/tmp"); ok {
		t.Fatal("expected tmpfs mount to be skipped")
	}
	if _, ok := composeMountValue("type=bind,source=/tmp"); ok {
		t.Fatal("expected missing target mount to be skipped")
	}
}

func TestWriteComposeOverrideUsesOwnerOnlyPermissions(t *testing.T) {
	resolved := devcontainer.ResolvedConfig{
		StateDir:       t.TempDir(),
		ComposeService: "app",
		WorkspaceMount: "type=bind,source=/workspace,target=/workspaces/demo",
		Merged:         devcontainer.MergedConfig{},
		Labels:         map[string]string{},
	}

	path, err := writeComposeOverride(resolved, "image:latest")
	if err != nil {
		t.Fatalf("write compose override: %v", err)
	}
	if filepath.Dir(path) != resolved.StateDir {
		t.Fatalf("unexpected override path %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat compose override: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected owner-only override file, got %#o", got)
	}
}
