package reconcile

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	capuid "github.com/lauritsk/hatchctl/internal/capability/uidremap"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func TestImageHelperFunctions(t *testing.T) {
	t.Parallel()

	features := []devcontainer.ResolvedFeature{{Metadata: spec.MetadataEntry{ID: "a"}}, {Metadata: spec.MetadataEntry{ID: "b", ContainerEnv: map[string]string{"A": "1"}}}}
	if got := featureMetadata(features); len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected feature metadata %#v", got)
	}
	if mergeManagedImageMetadata(nil, nil) != nil {
		t.Fatal("expected empty managed metadata merge to return nil")
	}
	merged := mergeManagedImageMetadata([]spec.MetadataEntry{{ID: "base"}}, []spec.MetadataEntry{{ID: "overlay"}})
	if !reflect.DeepEqual(merged, []spec.MetadataEntry{{ID: "base"}, {ID: "overlay"}}) {
		t.Fatalf("unexpected merged metadata %#v", merged)
	}
	resolved := devcontainer.ResolvedConfig{ImageName: "hatchctl-demo"}
	if !isManagedImage(&resolved, "hatchctl-demo") || !isManagedImage(&resolved, "hatchctl-demo-base") || isManagedImage(&resolved, "ghcr.io/example/demo") {
		t.Fatal("unexpected managed image detection")
	}
	if got := shellEnvFile(map[string]string{"B": "two words", "A": "1"}); got != "A='1'\nB='two words'\n" {
		t.Fatalf("unexpected shell env file %q", got)
	}
	if got := dockerfileQuotedValue("a\n$HOME\"\\b\r"); got != "\"a\\n\\$HOME\\\"\\\\b\"" {
		t.Fatalf("unexpected dockerfile quoted value %q", got)
	}
}

func TestInspectImageArchitectureAndLocalImageHandling(t *testing.T) {
	t.Parallel()

	pulls := 0
	executor := &Executor{engine: &fakeExecutorEngine{inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
		switch req.Reference {
		case "missing":
			return docker.ImageInspect{}, &docker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
		case "empty-arch":
			return docker.ImageInspect{}, nil
		default:
			return docker.ImageInspect{Architecture: "arm64"}, nil
		}
	}, pullImageFunc: func(_ context.Context, req dockercli.PullImageRequest) error {
		pulls++
		if req.Reference != "missing" {
			t.Fatalf("unexpected pull request %#v", req)
		}
		return nil
	}}}

	if got, err := executor.InspectImageArchitecture(context.Background(), "known"); err != nil || got != "arm64" {
		t.Fatalf("unexpected explicit architecture %q err=%v", got, err)
	}
	if got, err := executor.InspectImageArchitecture(context.Background(), "empty-arch"); err != nil || got != runtime.GOARCH {
		t.Fatalf("unexpected empty-arch fallback %q err=%v", got, err)
	}
	if got, err := executor.InspectImageArchitecture(context.Background(), "missing"); err != nil || got != runtime.GOARCH {
		t.Fatalf("unexpected missing-image fallback %q err=%v", got, err)
	}
	if err := executor.ensureLocalImage(context.Background(), "known", nil); err != nil {
		t.Fatalf("ensure local image existing: %v", err)
	}
	if err := executor.ensureLocalImage(context.Background(), "missing", nil); err != nil {
		t.Fatalf("ensure local image missing: %v", err)
	}
	if pulls != 1 {
		t.Fatalf("expected one pull for missing image, got %d", pulls)
	}
}

func TestBuildDockerfileImageUsesResolvedBuildInputs(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	var req dockercli.BuildImageRequest
	executor := &Executor{engine: &fakeExecutorEngine{buildImageFunc: func(_ context.Context, got dockercli.BuildImageRequest) error {
		req = got
		return nil
	}}}
	resolved := devcontainer.ResolvedConfig{
		ConfigDir: configDir,
		Config:    devcontainer.Config{Build: &devcontainer.BuildConfig{Dockerfile: "Dockerfile.dev", Context: "ctx", Target: "dev", Args: map[string]string{"A": "1"}, Options: []string{"--pull"}}},
		Merged:    spec.MergedConfig{Metadata: []spec.MetadataEntry{{RemoteUser: "vscode"}}},
	}

	if err := executor.buildDockerfileImage(context.Background(), resolved, "hatchctl-demo", "image-key", nil); err != nil {
		t.Fatalf("build dockerfile image: %v", err)
	}
	if req.ContextDir != filepath.Join(configDir, "ctx") || req.Dockerfile != filepath.Join(configDir, "Dockerfile.dev") || req.Tag != "hatchctl-demo" || req.Target != "dev" {
		t.Fatalf("unexpected build image request %#v", req)
	}
	if !reflect.DeepEqual(req.BuildArgs, map[string]string{"A": "1"}) || !reflect.DeepEqual(req.ExtraOptions, []string{"--pull"}) {
		t.Fatalf("unexpected build options %#v %#v", req.BuildArgs, req.ExtraOptions)
	}
	if req.Labels[ImageKeyLabel] != "image-key" || req.Labels[devcontainer.ImageMetadataLabel] == "" {
		t.Fatalf("expected image key and metadata labels, got %#v", req.Labels)
	}
}

func TestEnsureComposeImageBuildsServiceAndFallsBackToComposeName(t *testing.T) {
	t.Parallel()

	composeBuilds := 0
	executor := &Executor{engine: &fakeExecutorEngine{composeConfigFunc: func(_ context.Context, req dockercli.ComposeConfigRequest) (string, error) {
		if !reflect.DeepEqual(req.Target.Files, []string{"compose.yml"}) || req.Target.Dir != "/workspace/.devcontainer" {
			t.Fatalf("unexpected compose config request %#v", req)
		}
		return `{"name":"demo","services":{"app":{"build":{"context":"."}}}}`, nil
	}, composeBuildFunc: func(_ context.Context, req dockercli.ComposeBuildRequest) error {
		composeBuilds++
		if !reflect.DeepEqual(req.Services, []string{"app"}) {
			t.Fatalf("unexpected compose build request %#v", req)
		}
		return nil
	}}}
	resolved := devcontainer.ResolvedConfig{ConfigDir: "/workspace/.devcontainer", ComposeFiles: []string{"compose.yml"}, ComposeService: "app"}

	image, err := executor.ensureComposeImage(context.Background(), resolved, "image-key", nil)
	if err != nil {
		t.Fatalf("ensure compose image: %v", err)
	}
	if image != "demo-app" {
		t.Fatalf("unexpected compose image %q", image)
	}
	if composeBuilds != 1 {
		t.Fatalf("expected one compose build, got %d", composeBuilds)
	}
}

func TestEnsureUpdatedUIDContainerSkipsAndExecutesEligibleUser(t *testing.T) {
	t.Parallel()

	falseValue := false
	called := false
	executor := &Executor{engine: &fakeExecutorEngine{inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
		if req.Reference != "image-ref" {
			t.Fatalf("unexpected inspect image request %#v", req)
		}
		return docker.ImageInspect{Config: docker.InspectConfig{User: "root"}}, nil
	}, execFunc: func(_ context.Context, req dockercli.ExecRequest) error {
		called = true
		if req.ContainerID != "container-123" || req.User != "root" || !req.Interactive {
			t.Fatalf("unexpected uid remap exec request %#v", req)
		}
		if len(req.Command) < 6 || req.Command[0] != "sh" || req.Command[1] != "-s" || req.Command[3] != "vscode" {
			t.Fatalf("unexpected uid remap command %#v", req.Command)
		}
		if stdin, err := io.ReadAll(req.Streams.Stdin); err != nil || !strings.Contains(string(stdin), "REMOTE_USER=$1") {
			t.Fatalf("unexpected uid remap stdin %q err=%v", string(stdin), err)
		}
		return nil
	}}}
	resolved := devcontainer.ResolvedConfig{Merged: spec.MergedConfig{RemoteUser: "vscode"}}

	if err := executor.EnsureUpdatedUIDContainer(context.Background(), devcontainer.ResolvedConfig{Merged: spec.MergedConfig{UpdateRemoteUserUID: &falseValue}}, "image-ref", "container-123", nil); err != nil {
		t.Fatalf("expected disabled uid remap to skip, got %v", err)
	}
	if called {
		t.Fatal("expected disabled uid remap to avoid exec")
	}
	if err := executor.EnsureUpdatedUIDContainer(context.Background(), resolved, "image-ref", "container-123", nil); err != nil {
		t.Fatalf("ensure updated uid container: %v", err)
	}
	if !called {
		t.Fatal("expected eligible uid remap to exec update script")
	}
	if remoteUser, ok := capuid.Eligible(resolved, docker.ImageInspect{Config: docker.InspectConfig{User: "root"}}); !ok || remoteUser != "vscode" {
		t.Fatalf("unexpected capuid eligibility result %q ok=%v", remoteUser, ok)
	}
}
