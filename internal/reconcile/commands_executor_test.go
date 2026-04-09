package reconcile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/capability"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
)

type fakeExecutorEngine struct {
	inspectImageFunc     func(context.Context, dockercli.InspectImageRequest) (docker.ImageInspect, error)
	inspectContainerFunc func(context.Context, dockercli.InspectContainerRequest) (docker.ContainerInspect, error)
	buildImageFunc       func(context.Context, dockercli.BuildImageRequest) error
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
	sourceMetadataLabel, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{RemoteUser: "vscode", UpdateRemoteUserUID: &falseValue}})
	if err != nil {
		t.Fatalf("metadata label: %v", err)
	}
	managedMetadataLabel, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{ID: "mise"}})
	if err != nil {
		t.Fatalf("managed metadata label: %v", err)
	}

	var execRequests []dockercli.ExecRequest
	var containerLabels map[string]string
	imageBuilt := false
	engine := &fakeExecutorEngine{
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
				Merged:          devcontainer.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo", UpdateRemoteUserUID: &falseValue}, []devcontainer.MetadataEntry{{ID: "mise"}}),
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
	if err := devcontainer.WriteState(stateDir, devcontainer.State{
		ContainerID:    "container-old",
		ContainerKey:   "old-key",
		LifecycleReady: true,
		DotfilesReady:  true,
		DotfilesRepo:   "https://github.com/example/dotfiles.git",
		DotfilesTarget: "$HOME/.dotfiles",
	}); err != nil {
		t.Fatalf("write prior state: %v", err)
	}
	sourceMetadataLabel, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{RemoteUser: "vscode", UpdateRemoteUserUID: &falseValue}})
	if err != nil {
		t.Fatalf("source metadata label: %v", err)
	}
	managedMetadataLabel, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{ID: "mise"}})
	if err != nil {
		t.Fatalf("managed metadata label: %v", err)
	}

	var execRequests []dockercli.ExecRequest
	imageBuilt := false
	engine := &fakeExecutorEngine{
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
				Merged:          devcontainer.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo", UpdateRemoteUserUID: &falseValue}, []devcontainer.MetadataEntry{{ID: "mise"}}),
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
	if err := devcontainer.WriteState(stateDir, devcontainer.State{ContainerID: "container-123"}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	containerMetadataLabel, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{ID: "mise"}})
	if err != nil {
		t.Fatalf("container metadata label: %v", err)
	}
	sourceMetadataLabel, err := devcontainer.MetadataLabelValue([]devcontainer.MetadataEntry{{RemoteUser: "vscode"}})
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
				Merged:          devcontainer.MergeMetadata(devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"}, nil),
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
