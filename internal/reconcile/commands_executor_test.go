package reconcile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/capability"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type fakeExecutorEngine struct {
	inspectImageFunc     func(context.Context, dockercli.InspectImageRequest) (docker.ImageInspect, error)
	inspectContainerFunc func(context.Context, dockercli.InspectContainerRequest) (docker.ContainerInspect, error)
	buildImageFunc       func(context.Context, dockercli.BuildImageRequest) error
	pullImageFunc        func(context.Context, dockercli.PullImageRequest) error
	runDetachedFunc      func(context.Context, dockercli.RunDetachedContainerRequest) (string, error)
	startContainerFunc   func(context.Context, dockercli.StartContainerRequest) error
	removeContainerFunc  func(context.Context, dockercli.RemoveContainerRequest) error
	listContainersFunc   func(context.Context, dockercli.ListContainersRequest) (string, error)
	composeConfigFunc    func(context.Context, dockercli.ComposeConfigRequest) (string, error)
	composeBuildFunc     func(context.Context, dockercli.ComposeBuildRequest) error
	composeUpFunc        func(context.Context, dockercli.ComposeUpRequest) error
	execFunc             func(context.Context, dockercli.ExecRequest) error
	execOutputFunc       func(context.Context, dockercli.ExecRequest) (string, error)
}

func (f *fakeExecutorEngine) InspectImage(ctx context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
	if f.inspectImageFunc != nil {
		return f.inspectImageFunc(ctx, req)
	}
	return docker.ImageInspect{}, errors.New("unexpected inspect image")
}

func (f *fakeExecutorEngine) InspectContainer(ctx context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
	if f.inspectContainerFunc != nil {
		return f.inspectContainerFunc(ctx, req)
	}
	return docker.ContainerInspect{}, errors.New("unexpected inspect container")
}

func (f *fakeExecutorEngine) BuildImage(ctx context.Context, req dockercli.BuildImageRequest) error {
	if f.buildImageFunc != nil {
		return f.buildImageFunc(ctx, req)
	}
	return errors.New("unexpected build image")
}

func (f *fakeExecutorEngine) PullImage(ctx context.Context, req dockercli.PullImageRequest) error {
	if f.pullImageFunc != nil {
		return f.pullImageFunc(ctx, req)
	}
	return errors.New("unexpected pull image")
}

func (f *fakeExecutorEngine) RunDetachedContainer(ctx context.Context, req dockercli.RunDetachedContainerRequest) (string, error) {
	if f.runDetachedFunc != nil {
		return f.runDetachedFunc(ctx, req)
	}
	return "", errors.New("unexpected run detached container")
}

func (f *fakeExecutorEngine) StartContainer(ctx context.Context, req dockercli.StartContainerRequest) error {
	if f.startContainerFunc != nil {
		return f.startContainerFunc(ctx, req)
	}
	return errors.New("unexpected start container")
}

func (f *fakeExecutorEngine) RemoveContainer(ctx context.Context, req dockercli.RemoveContainerRequest) error {
	if f.removeContainerFunc != nil {
		return f.removeContainerFunc(ctx, req)
	}
	return errors.New("unexpected remove container")
}

func (f *fakeExecutorEngine) ListContainers(ctx context.Context, req dockercli.ListContainersRequest) (string, error) {
	if f.listContainersFunc != nil {
		return f.listContainersFunc(ctx, req)
	}
	return "", nil
}

func (f *fakeExecutorEngine) ComposeConfig(ctx context.Context, req dockercli.ComposeConfigRequest) (string, error) {
	if f.composeConfigFunc != nil {
		return f.composeConfigFunc(ctx, req)
	}
	return "", errors.New("unexpected compose config")
}

func (f *fakeExecutorEngine) ComposeBuild(ctx context.Context, req dockercli.ComposeBuildRequest) error {
	if f.composeBuildFunc != nil {
		return f.composeBuildFunc(ctx, req)
	}
	return errors.New("unexpected compose build")
}

func (f *fakeExecutorEngine) ComposeUp(ctx context.Context, req dockercli.ComposeUpRequest) error {
	if f.composeUpFunc != nil {
		return f.composeUpFunc(ctx, req)
	}
	return errors.New("unexpected compose up")
}

func (f *fakeExecutorEngine) Exec(ctx context.Context, req dockercli.ExecRequest) error {
	if f.execFunc != nil {
		return f.execFunc(ctx, req)
	}
	return errors.New("unexpected exec")
}

func (f *fakeExecutorEngine) ExecOutput(ctx context.Context, req dockercli.ExecRequest) (string, error) {
	if f.execOutputFunc != nil {
		return f.execOutputFunc(ctx, req)
	}
	return "", errors.New("unexpected exec output")
}

func TestUpUsesEnrichedResolvedMetadataForDotfilesTargetPath(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	featureDir := t.TempDir()
	falseValue := false
	if err := os.WriteFile(filepath.Join(featureDir, "install.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write feature install script: %v", err)
	}
	sourceMetadataLabel, err := spec.MetadataLabelValue([]devcontainer.MetadataEntry{{RemoteUser: "vscode", UpdateRemoteUserUID: &falseValue}})
	if err != nil {
		t.Fatalf("metadata label: %v", err)
	}
	managedMetadataLabel, err := spec.MetadataLabelValue([]devcontainer.MetadataEntry{{ID: "mise"}})
	if err != nil {
		t.Fatalf("managed metadata label: %v", err)
	}

	var execRequests []dockercli.ExecRequest
	var containerLabels map[string]string
	baseImagePulled := false
	imageBuilt := false
	engine := &fakeExecutorEngine{
		pullImageFunc: func(_ context.Context, req dockercli.PullImageRequest) error {
			if req.Reference != "mcr.microsoft.com/devcontainers/base:ubuntu" {
				t.Fatalf("unexpected pull image ref %q", req.Reference)
			}
			baseImagePulled = true
			return nil
		},
		listContainersFunc: func(context.Context, dockercli.ListContainersRequest) (string, error) {
			return "", nil
		},
		buildImageFunc: func(context.Context, dockercli.BuildImageRequest) error {
			imageBuilt = true
			return nil
		},
		inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
			switch req.Reference {
			case "mcr.microsoft.com/devcontainers/base:ubuntu":
				if !baseImagePulled {
					return docker.ImageInspect{}, &docker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
				}
				return docker.ImageInspect{
					Architecture: "arm64",
					Config:       docker.InspectConfig{Labels: map[string]string{devcontainer.ImageMetadataLabel: sourceMetadataLabel}},
				}, nil
			case "hatchctl-demo":
				if !imageBuilt {
					return docker.ImageInspect{}, &docker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
				}
				return docker.ImageInspect{
					Architecture: "arm64",
					Config:       docker.InspectConfig{Labels: map[string]string{devcontainer.ImageMetadataLabel: managedMetadataLabel}},
				}, nil
			default:
				t.Fatalf("unexpected inspect image ref %q", req.Reference)
			}
			return docker.ImageInspect{}, nil
		},
		runDetachedFunc: func(_ context.Context, req dockercli.RunDetachedContainerRequest) (string, error) {
			containerLabels = req.Labels
			return "container-123", nil
		},
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			if req.ContainerID != "container-123" {
				t.Fatalf("unexpected inspect container id %q", req.ContainerID)
			}
			return docker.ContainerInspect{
				ID:    "container-123",
				Image: "hatchctl-demo",
				Config: docker.InspectConfig{
					User:   "root",
					Labels: containerLabels,
				},
				State: docker.ContainerState{Status: "running", Running: true},
			}, nil
		},
		execOutputFunc: func(_ context.Context, req dockercli.ExecRequest) (string, error) {
			if len(req.Command) != 2 || req.Command[0] != "cat" || req.Command[1] != passwdFilePath {
				t.Fatalf("unexpected exec output command %#v", req.Command)
			}
			return "root:x:0:0:root:/root:/bin/sh\nvscode:x:1000:1000::/home/vscode:/bin/bash\n", nil
		},
		execFunc: func(_ context.Context, req dockercli.ExecRequest) error {
			execRequests = append(execRequests, req)
			return nil
		},
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = engine
	executor.planner = &workspaceplan.Resolver{
		Resolve: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				WorkspaceFolder: "/workspace",
				ConfigPath:      filepath.Join(configDir, "devcontainer.json"),
				ConfigDir:       configDir,
				Config: devcontainer.Config{
					Image:               "mcr.microsoft.com/devcontainers/base:ubuntu",
					WorkspaceFolder:     "/workspaces/demo",
					UpdateRemoteUserUID: &falseValue,
				},
				Features:        []devcontainer.ResolvedFeature{{Path: featureDir, Metadata: devcontainer.MetadataEntry{ID: "mise"}}},
				Merged:          spec.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo", UpdateRemoteUserUID: &falseValue}, []devcontainer.MetadataEntry{{ID: "mise"}}),
				StateDir:        stateDir,
				CacheDir:        cacheDir,
				WorkspaceMount:  "type=bind,source=/workspace,target=/workspaces/demo",
				RemoteWorkspace: "/workspaces/demo",
				ImageName:       "hatchctl-demo",
				SourceKind:      "image",
				ContainerName:   "hatchctl-demo-container",
				Labels: map[string]string{
					devcontainer.HostFolderLabel: "/workspace",
					devcontainer.ConfigFileLabel: filepath.Join(configDir, "devcontainer.json"),
					devcontainer.ManagedByLabel:  devcontainer.ManagedByValue,
				},
			}, nil
		},
	}

	workspacePlan := workspaceplan.WorkspacePlan{
		Preferences: workspaceplan.Preferences{
			Dotfiles: workspaceplan.DotfilesPreference{Repository: "https://github.com/example/dotfiles.git"},
		},
		Capabilities: capability.Set{
			Dotfiles: capability.Dotfiles{Repository: "https://github.com/example/dotfiles.git"},
		},
		Trust: workspaceplan.TrustPlan{WorkspaceAllowed: true},
	}

	if _, err := executor.Up(context.Background(), workspacePlan, UpOptions{}); err != nil {
		t.Fatalf("up: %v", err)
	}

	if len(execRequests) != 1 {
		t.Fatalf("expected one exec request, got %#v", execRequests)
	}
	if execRequests[0].User != "vscode" {
		t.Fatalf("expected dotfiles install to run as vscode, got %#v", execRequests[0])
	}
	if got := execRequests[0].Command[4]; got != "/home/vscode/.dotfiles" {
		t.Fatalf("expected dotfiles target path /home/vscode/.dotfiles, got %q", got)
	}
}

func TestUpRecreateReinstallsDotfilesForNewContainer(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	featureDir := t.TempDir()
	falseValue := false
	if err := os.WriteFile(filepath.Join(featureDir, "install.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write feature install script: %v", err)
	}
	if err := storefs.WriteWorkspaceState(stateDir, storefs.WorkspaceState{
		ContainerID:    "container-old",
		ContainerKey:   "old-key",
		LifecycleReady: true,
		DotfilesReady:  true,
		DotfilesRepo:   "https://github.com/example/dotfiles.git",
		DotfilesTarget: "$HOME/.dotfiles",
	}); err != nil {
		t.Fatalf("write prior state: %v", err)
	}
	sourceMetadataLabel, err := spec.MetadataLabelValue([]devcontainer.MetadataEntry{{RemoteUser: "vscode", UpdateRemoteUserUID: &falseValue}})
	if err != nil {
		t.Fatalf("source metadata label: %v", err)
	}
	managedMetadataLabel, err := spec.MetadataLabelValue([]devcontainer.MetadataEntry{{ID: "mise"}})
	if err != nil {
		t.Fatalf("managed metadata label: %v", err)
	}

	var execRequests []dockercli.ExecRequest
	baseImagePulled := false
	imageBuilt := false
	engine := &fakeExecutorEngine{
		pullImageFunc: func(_ context.Context, req dockercli.PullImageRequest) error {
			if req.Reference != "mcr.microsoft.com/devcontainers/base:ubuntu" {
				t.Fatalf("unexpected pull image ref %q", req.Reference)
			}
			baseImagePulled = true
			return nil
		},
		listContainersFunc: func(context.Context, dockercli.ListContainersRequest) (string, error) {
			return "container-old\n", nil
		},
		buildImageFunc: func(context.Context, dockercli.BuildImageRequest) error {
			imageBuilt = true
			return nil
		},
		inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
			switch req.Reference {
			case "mcr.microsoft.com/devcontainers/base:ubuntu":
				if !baseImagePulled {
					return docker.ImageInspect{}, &docker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
				}
				return docker.ImageInspect{Architecture: "arm64", Config: docker.InspectConfig{Labels: map[string]string{devcontainer.ImageMetadataLabel: sourceMetadataLabel}}}, nil
			case "hatchctl-demo":
				if !imageBuilt {
					return docker.ImageInspect{}, &docker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
				}
				return docker.ImageInspect{Architecture: "arm64", Config: docker.InspectConfig{Labels: map[string]string{devcontainer.ImageMetadataLabel: managedMetadataLabel}}}, nil
			default:
				t.Fatalf("unexpected inspect image ref %q", req.Reference)
			}
			return docker.ImageInspect{}, nil
		},
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			switch req.ContainerID {
			case "container-old":
				return docker.ContainerInspect{ID: "container-old", Image: "hatchctl-demo", Config: docker.InspectConfig{User: "root", Labels: map[string]string{ContainerKeyLabel: "old-key", devcontainer.ImageMetadataLabel: managedMetadataLabel}}, State: docker.ContainerState{Status: "running", Running: true}}, nil
			case "container-new":
				return docker.ContainerInspect{ID: "container-new", Image: "hatchctl-demo", Config: docker.InspectConfig{User: "root", Labels: map[string]string{devcontainer.ImageMetadataLabel: managedMetadataLabel}}, State: docker.ContainerState{Status: "running", Running: true}}, nil
			default:
				t.Fatalf("unexpected inspect container id %q", req.ContainerID)
			}
			return docker.ContainerInspect{}, nil
		},
		removeContainerFunc: func(_ context.Context, req dockercli.RemoveContainerRequest) error {
			if req.ContainerID != "container-old" {
				t.Fatalf("unexpected removed container %q", req.ContainerID)
			}
			return nil
		},
		runDetachedFunc: func(_ context.Context, req dockercli.RunDetachedContainerRequest) (string, error) {
			return "container-new", nil
		},
		execOutputFunc: func(_ context.Context, req dockercli.ExecRequest) (string, error) {
			if len(req.Command) != 2 || req.Command[0] != "cat" || req.Command[1] != passwdFilePath {
				t.Fatalf("unexpected exec output command %#v", req.Command)
			}
			return "root:x:0:0:root:/root:/bin/sh\nvscode:x:1000:1000::/home/vscode:/bin/bash\n", nil
		},
		execFunc: func(_ context.Context, req dockercli.ExecRequest) error {
			execRequests = append(execRequests, req)
			return nil
		},
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = engine
	executor.planner = &workspaceplan.Resolver{
		Resolve: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				WorkspaceFolder: "/workspace",
				ConfigPath:      filepath.Join(configDir, "devcontainer.json"),
				ConfigDir:       configDir,
				Config: devcontainer.Config{
					Image:               "mcr.microsoft.com/devcontainers/base:ubuntu",
					WorkspaceFolder:     "/workspaces/demo",
					UpdateRemoteUserUID: &falseValue,
				},
				Features:        []devcontainer.ResolvedFeature{{Path: featureDir, Metadata: devcontainer.MetadataEntry{ID: "mise"}}},
				Merged:          spec.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo", UpdateRemoteUserUID: &falseValue}, []devcontainer.MetadataEntry{{ID: "mise"}}),
				StateDir:        stateDir,
				CacheDir:        cacheDir,
				WorkspaceMount:  "type=bind,source=/workspace,target=/workspaces/demo",
				RemoteWorkspace: "/workspaces/demo",
				ImageName:       "hatchctl-demo",
				SourceKind:      "image",
				ContainerName:   "hatchctl-demo-container",
				Labels: map[string]string{
					devcontainer.HostFolderLabel: "/workspace",
					devcontainer.ConfigFileLabel: filepath.Join(configDir, "devcontainer.json"),
					devcontainer.ManagedByLabel:  devcontainer.ManagedByValue,
				},
			}, nil
		},
	}

	workspacePlan := workspaceplan.WorkspacePlan{
		Preferences:  workspaceplan.Preferences{Dotfiles: workspaceplan.DotfilesPreference{Repository: "https://github.com/example/dotfiles.git"}},
		Capabilities: capability.Set{Dotfiles: capability.Dotfiles{Repository: "https://github.com/example/dotfiles.git"}},
		Trust:        workspaceplan.TrustPlan{WorkspaceAllowed: true},
	}

	if _, err := executor.Up(context.Background(), workspacePlan, UpOptions{Recreate: true}); err != nil {
		t.Fatalf("up recreate: %v", err)
	}
	if len(execRequests) != 1 {
		t.Fatalf("expected dotfiles install for recreated container, got %#v", execRequests)
	}
	if execRequests[0].ContainerID != "container-new" || execRequests[0].User != "vscode" {
		t.Fatalf("unexpected exec request %#v", execRequests[0])
	}
}

func TestExecMergesConfiguredImageMetadataWhenContainerLabelIsIncomplete(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	if err := storefs.WriteWorkspaceState(stateDir, storefs.WorkspaceState{ContainerID: "container-123"}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	containerMetadataLabel, err := spec.MetadataLabelValue([]devcontainer.MetadataEntry{{ID: "mise"}})
	if err != nil {
		t.Fatalf("container metadata label: %v", err)
	}
	sourceMetadataLabel, err := spec.MetadataLabelValue([]devcontainer.MetadataEntry{{RemoteUser: "vscode"}})
	if err != nil {
		t.Fatalf("metadata label: %v", err)
	}

	var inspectImageRefs []string
	var execReq dockercli.ExecRequest
	engine := &fakeExecutorEngine{
		inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
			inspectImageRefs = append(inspectImageRefs, req.Reference)
			if req.Reference == "mcr.microsoft.com/devcontainers/base:ubuntu" {
				return docker.ImageInspect{Config: docker.InspectConfig{Labels: map[string]string{devcontainer.ImageMetadataLabel: sourceMetadataLabel}}}, nil
			}
			return docker.ImageInspect{}, errors.New("unexpected inspect image")
		},
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			if req.ContainerID != "container-123" {
				t.Fatalf("unexpected inspect container id %q", req.ContainerID)
			}
			return docker.ContainerInspect{
				ID:    "container-123",
				Image: "managed-image-id",
				Config: docker.InspectConfig{
					User:   "root",
					Labels: map[string]string{devcontainer.ImageMetadataLabel: containerMetadataLabel},
				},
				State: docker.ContainerState{Status: "running", Running: true},
			}, nil
		},
		execOutputFunc: func(_ context.Context, req dockercli.ExecRequest) (string, error) {
			if len(req.Command) != 2 || req.Command[0] != "cat" || req.Command[1] != passwdFilePath {
				t.Fatalf("unexpected exec output command %#v", req.Command)
			}
			return "root:x:0:0:root:/root:/bin/sh\nvscode:x:1000:1000::/home/vscode:/bin/bash\n", nil
		},
		execFunc: func(_ context.Context, req dockercli.ExecRequest) error {
			execReq = req
			return nil
		},
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = engine
	executor.planner = &workspaceplan.Resolver{
		ResolveReadOnly: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				WorkspaceFolder: "/workspace",
				ConfigPath:      filepath.Join(configDir, "devcontainer.json"),
				ConfigDir:       configDir,
				Config: devcontainer.Config{
					Image:           "mcr.microsoft.com/devcontainers/base:ubuntu",
					WorkspaceFolder: "/workspaces/demo",
				},
				Merged:          spec.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"}, nil),
				StateDir:        stateDir,
				CacheDir:        cacheDir,
				RemoteWorkspace: "/workspaces/demo",
				ImageName:       "hatchctl-demo",
				SourceKind:      "image",
				ContainerName:   "hatchctl-demo-container",
				Labels: map[string]string{
					devcontainer.HostFolderLabel: "/workspace",
					devcontainer.ConfigFileLabel: filepath.Join(configDir, "devcontainer.json"),
					devcontainer.ManagedByLabel:  devcontainer.ManagedByValue,
				},
			}, nil
		},
	}

	workspacePlan := workspaceplan.WorkspacePlan{
		ReadOnly: true,
	}

	code, err := executor.Exec(context.Background(), workspacePlan, ExecOptions{Args: []string{"pwd"}})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}
	if execReq.ContainerID != "container-123" || execReq.User != "vscode" {
		t.Fatalf("unexpected exec request %#v", execReq)
	}
	if len(inspectImageRefs) != 1 || inspectImageRefs[0] != "mcr.microsoft.com/devcontainers/base:ubuntu" {
		t.Fatalf("expected exec to inspect configured image metadata, got %#v", inspectImageRefs)
	}
}

func TestMaterializeReadOnlyRejectsUntrustedFeatureWithoutPrompt(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	ref := "ghcr.io/example/feature@sha256:abc123"
	prompted := false
	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.imageVerifier = policy.NewImageVerificationPolicyWithPrompt(false, func(string) (bool, bool, error) {
		prompted = true
		return true, true, nil
	})
	executor.planner = &workspaceplan.Resolver{
		ResolveReadOnly: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				StateDir: stateDir,
				Features: []devcontainer.ResolvedFeature{{
					Source:       ref,
					SourceKind:   "oci",
					Resolved:     ref,
					Verification: security.VerificationResult{Ref: ref, Reason: "no signatures found"},
				}},
			}, nil
		},
	}

	_, err := executor.Materialize(context.Background(), workspaceplan.WorkspacePlan{ReadOnly: true, LockProtected: workspaceplan.LockProtectedArtifacts{StateDir: stateDir}}, false, nil, phaseResolve, "Resolving development container")
	if err == nil {
		t.Fatal("expected materialize to reject untrusted feature")
	}
	if !strings.Contains(err.Error(), ref) {
		t.Fatalf("expected error to mention feature ref, got %v", err)
	}
	if prompted {
		t.Fatal("expected read-only materialize to fail without prompting")
	}
}

func TestMaterializeReadOnlyRejectsUntrustedLoopbackFeatureWithoutPrompt(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	ref := "localhost:5000/example/feature@sha256:abc123"
	prompted := false
	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.imageVerifier = policy.NewImageVerificationPolicyWithPrompt(false, func(string) (bool, bool, error) {
		prompted = true
		return true, true, nil
	})
	executor.planner = &workspaceplan.Resolver{
		ResolveReadOnly: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				StateDir: stateDir,
				Features: []devcontainer.ResolvedFeature{{
					Source:       ref,
					SourceKind:   "oci",
					Resolved:     ref,
					Verification: security.VerificationResult{Ref: ref, Reason: "no signatures found"},
				}},
			}, nil
		},
	}

	_, err := executor.Materialize(context.Background(), workspaceplan.WorkspacePlan{ReadOnly: true, LockProtected: workspaceplan.LockProtectedArtifacts{StateDir: stateDir}}, false, nil, phaseResolve, "Resolving development container")
	if err == nil {
		t.Fatal("expected materialize to reject untrusted loopback feature")
	}
	if !strings.Contains(err.Error(), ref) {
		t.Fatalf("expected error to mention feature ref, got %v", err)
	}
	if prompted {
		t.Fatal("expected read-only materialize to fail without prompting")
	}
}

func TestMaterializeReadOnlyUsesPersistedTrustedRefs(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	ref := "ghcr.io/example/feature@sha256:def456"
	if err := storefs.WriteWorkspaceState(stateDir, storefs.WorkspaceState{TrustedRefs: []string{ref}}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	prompted := false
	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.imageVerifier = policy.NewImageVerificationPolicyWithPrompt(false, func(string) (bool, bool, error) {
		prompted = true
		return true, true, nil
	})
	executor.planner = &workspaceplan.Resolver{
		ResolveReadOnly: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				StateDir: stateDir,
				Features: []devcontainer.ResolvedFeature{{
					Source:       ref,
					SourceKind:   "oci",
					Resolved:     ref,
					Verification: security.VerificationResult{Ref: ref, Reason: "no signatures found"},
				}},
			}, nil
		},
	}

	resolved, err := executor.Materialize(context.Background(), workspaceplan.WorkspacePlan{ReadOnly: true, LockProtected: workspaceplan.LockProtectedArtifacts{StateDir: stateDir}}, false, nil, phaseResolve, "Resolving development container")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if len(resolved.Features) != 1 || resolved.Features[0].Resolved != ref {
		t.Fatalf("unexpected resolved features %#v", resolved.Features)
	}
	if prompted {
		t.Fatal("expected persisted trust to avoid prompting")
	}
}

func TestBuildPersistsTrustedRefsToWorkspaceState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	trustedRef := "ghcr.io/example/feature@sha256:trusted"
	engine := &fakeExecutorEngine{
		inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
			if req.Reference != "mcr.microsoft.com/devcontainers/base:ubuntu" {
				t.Fatalf("unexpected inspect image ref %q", req.Reference)
			}
			return docker.ImageInspect{Config: docker.InspectConfig{}}, nil
		},
	}
	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = engine
	executor.imageVerifier = policy.NewImageVerificationPolicyWithPrompt(false, nil, trustedRef)
	executor.planner = &workspaceplan.Resolver{
		Resolve: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				Config:   devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu"},
				StateDir: stateDir,
				CacheDir: cacheDir,
			}, nil
		},
	}

	if _, err := executor.Build(context.Background(), workspaceplan.WorkspacePlan{LockProtected: workspaceplan.LockProtectedArtifacts{StateDir: stateDir}}, BuildOptions{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	state, err := storefs.ReadWorkspaceState(stateDir)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(state.TrustedRefs) != 1 || state.TrustedRefs[0] != trustedRef {
		t.Fatalf("expected trusted refs to persist, got %#v", state.TrustedRefs)
	}
}

func TestReadConfigReportsBridgeAndDotfilesState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	if err := storefs.WriteWorkspaceState(stateDir, storefs.WorkspaceState{
		ContainerID:     "container-123",
		ContainerKey:    "container-key",
		BridgeEnabled:   true,
		BridgeSessionID: "bridge-session",
		DotfilesReady:   true,
		DotfilesRepo:    "https://github.com/example/dotfiles.git",
		DotfilesTarget:  "/home/vscode/.dotfiles",
	}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	paths, err := storefs.EnsureWorkspaceBridgePaths(stateDir)
	if err != nil {
		t.Fatalf("ensure bridge paths: %v", err)
	}
	helperPath := filepath.Join(paths.BinDir, "devcontainer-open")
	if err := storefs.WriteBridgeExecutable(helperPath, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatalf("write bridge helper: %v", err)
	}
	session := bridge.Session{
		ID:         "bridge-session",
		Enabled:    true,
		Host:       "host.docker.internal",
		Port:       41234,
		StatePath:  paths.Dir,
		ConfigPath: paths.ConfigPath,
		PIDPath:    paths.PIDPath,
		StatusPath: paths.StatusPath,
		HelperPath: helperPath,
		MountPath:  "/var/run/hatchctl/bridge",
		BinPath:    "/var/run/hatchctl/bridge/bin",
		Status:     "scaffolded",
	}
	if err := storefs.WriteBridgeSession(paths.Dir, session); err != nil {
		t.Fatalf("write bridge session: %v", err)
	}
	if err := storefs.WriteBridgeStatus(paths.StatusPath, map[string]any{"lastEvent": "bridge ready"}); err != nil {
		t.Fatalf("write bridge status: %v", err)
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = &fakeExecutorEngine{
		inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
			if req.Reference != "mcr.microsoft.com/devcontainers/base:ubuntu" {
				t.Fatalf("unexpected inspect image ref %q", req.Reference)
			}
			return docker.ImageInspect{Config: docker.InspectConfig{User: "vscode"}}, nil
		},
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			if req.ContainerID != "container-123" {
				t.Fatalf("unexpected inspect container id %q", req.ContainerID)
			}
			return docker.ContainerInspect{
				ID:    "container-123",
				Image: "mcr.microsoft.com/devcontainers/base:ubuntu",
				Config: docker.InspectConfig{
					User:   "vscode",
					Labels: map[string]string{},
				},
				State: docker.ContainerState{Status: "running", Running: true},
			}, nil
		},
		listContainersFunc: func(context.Context, dockercli.ListContainersRequest) (string, error) {
			return "", nil
		},
	}
	executor.planner = &workspaceplan.Resolver{
		ResolveReadOnly: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				WorkspaceFolder: "/workspace",
				ConfigPath:      filepath.Join(configDir, "devcontainer.json"),
				ConfigDir:       configDir,
				Config:          devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"},
				Merged:          spec.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"}, nil),
				StateDir:        stateDir,
				CacheDir:        cacheDir,
				WorkspaceMount:  "type=bind,source=/workspace,target=/workspaces/demo",
				RemoteWorkspace: "/workspaces/demo",
				ImageName:       "hatchctl-demo",
				SourceKind:      "image",
				ContainerName:   "hatchctl-demo-container",
				Labels: map[string]string{
					devcontainer.HostFolderLabel: "/workspace",
					devcontainer.ConfigFileLabel: filepath.Join(configDir, "devcontainer.json"),
					devcontainer.ManagedByLabel:  devcontainer.ManagedByValue,
				},
			}, nil
		},
	}

	result, err := executor.ReadConfig(context.Background(), workspaceplan.WorkspacePlan{
		ReadOnly: true,
		Preferences: workspaceplan.Preferences{
			Dotfiles: workspaceplan.DotfilesPreference{Repository: "https://github.com/example/dotfiles.git", TargetPath: "/home/vscode/.dotfiles"},
		},
	}, ReadConfigOptions{})
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if result.RemoteUser != "vscode" || result.ImageUser != "vscode" {
		t.Fatalf("expected image-backed users to resolve to vscode, got %#v", result)
	}
	if result.Bridge == nil || result.Bridge.Status != "bridge ready" || !result.Bridge.Enabled {
		t.Fatalf("expected bridge report from persisted session, got %#v", result.Bridge)
	}
	if result.Dotfiles == nil || !result.Dotfiles.Configured || !result.Dotfiles.Applied || result.Dotfiles.NeedsInstall {
		t.Fatalf("unexpected dotfiles status %#v", result.Dotfiles)
	}
	if result.ManagedContainer == nil || result.ManagedContainer.ID != "container-123" || !result.ManagedContainer.Running {
		t.Fatalf("expected managed container details from inspected target, got %#v", result.ManagedContainer)
	}
}

func TestRunLifecycleCreatePersistsLifecycleState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	if err := storefs.WriteWorkspaceState(stateDir, storefs.WorkspaceState{ContainerID: "container-123", ContainerKey: "container-key"}); err != nil {
		t.Fatalf("write state: %v", err)
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = &fakeExecutorEngine{
		inspectImageFunc: func(_ context.Context, req dockercli.InspectImageRequest) (docker.ImageInspect, error) {
			if req.Reference != "mcr.microsoft.com/devcontainers/base:ubuntu" {
				t.Fatalf("unexpected inspect image ref %q", req.Reference)
			}
			return docker.ImageInspect{Config: docker.InspectConfig{User: "vscode"}}, nil
		},
		inspectContainerFunc: func(_ context.Context, req dockercli.InspectContainerRequest) (docker.ContainerInspect, error) {
			if req.ContainerID != "container-123" {
				t.Fatalf("unexpected inspect container id %q", req.ContainerID)
			}
			return docker.ContainerInspect{
				ID:    "container-123",
				Image: "mcr.microsoft.com/devcontainers/base:ubuntu",
				Config: docker.InspectConfig{
					User:   "vscode",
					Labels: map[string]string{},
				},
				State: docker.ContainerState{Status: "running", Running: true},
			}, nil
		},
	}
	executor.planner = &workspaceplan.Resolver{
		Resolve: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				WorkspaceFolder: "/workspace",
				ConfigPath:      filepath.Join(configDir, "devcontainer.json"),
				ConfigDir:       configDir,
				Config:          devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"},
				Merged:          spec.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"}, nil),
				StateDir:        stateDir,
				CacheDir:        cacheDir,
				RemoteWorkspace: "/workspaces/demo",
				ImageName:       "hatchctl-demo",
				SourceKind:      "image",
				ContainerName:   "hatchctl-demo-container",
				Labels: map[string]string{
					devcontainer.HostFolderLabel: "/workspace",
					devcontainer.ConfigFileLabel: filepath.Join(configDir, "devcontainer.json"),
					devcontainer.ManagedByLabel:  devcontainer.ManagedByValue,
				},
			}, nil
		},
	}

	result, err := executor.RunLifecycle(context.Background(), workspaceplan.WorkspacePlan{}, RunLifecycleOptions{Phase: "create"})
	if err != nil {
		t.Fatalf("run lifecycle: %v", err)
	}
	if result.ContainerID != "container-123" || result.Phase != "create" {
		t.Fatalf("unexpected lifecycle result %#v", result)
	}

	state, err := storefs.ReadWorkspaceState(stateDir)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !state.LifecycleReady || state.LifecycleTransition != nil {
		t.Fatalf("expected lifecycle state to be complete, got %#v", state)
	}
	if state.LifecycleKey == "" {
		t.Fatalf("expected lifecycle key to persist, got %#v", state)
	}
	if state.ContainerID != "container-123" || state.ContainerKey != "container-key" {
		t.Fatalf("expected container identity to be preserved, got %#v", state)
	}
}

func TestBridgeDoctorReportsPersistedBridgeState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	paths, err := storefs.EnsureWorkspaceBridgePaths(stateDir)
	if err != nil {
		t.Fatalf("ensure bridge paths: %v", err)
	}
	helperPath := filepath.Join(paths.BinDir, "devcontainer-open")
	if err := storefs.WriteBridgeExecutable(helperPath, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatalf("write bridge helper: %v", err)
	}
	if err := storefs.WriteBridgeSession(paths.Dir, bridge.Session{
		ID:         "bridge-session",
		Enabled:    true,
		Host:       "host.docker.internal",
		Port:       43123,
		StatePath:  paths.Dir,
		ConfigPath: paths.ConfigPath,
		PIDPath:    paths.PIDPath,
		StatusPath: paths.StatusPath,
		HelperPath: helperPath,
		MountPath:  "/var/run/hatchctl/bridge",
		BinPath:    "/var/run/hatchctl/bridge/bin",
		Status:     "scaffolded",
	}); err != nil {
		t.Fatalf("write bridge session: %v", err)
	}
	if err := storefs.WriteBridgeStatus(paths.StatusPath, map[string]any{"lastEvent": "ready"}); err != nil {
		t.Fatalf("write bridge status: %v", err)
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.planner = &workspaceplan.Resolver{
		ResolveReadOnly: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return devcontainer.ResolvedConfig{
				WorkspaceFolder: "/workspace",
				ConfigPath:      filepath.Join(configDir, "devcontainer.json"),
				ConfigDir:       configDir,
				Config:          devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"},
				Merged:          spec.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"}, nil),
				StateDir:        stateDir,
				CacheDir:        cacheDir,
				RemoteWorkspace: "/workspaces/demo",
				ImageName:       "hatchctl-demo",
				SourceKind:      "image",
			}, nil
		},
	}

	report, err := executor.BridgeDoctor(context.Background(), workspaceplan.WorkspacePlan{ReadOnly: true}, BridgeDoctorOptions{})
	if err != nil {
		t.Fatalf("bridge doctor: %v", err)
	}
	if !report.Enabled || report.Status != "ready" {
		t.Fatalf("unexpected bridge report %#v", report)
	}
	if report.StatePath != paths.Dir || report.HelperPath != helperPath || report.Port != 43123 {
		t.Fatalf("unexpected bridge report details %#v", report)
	}
}
