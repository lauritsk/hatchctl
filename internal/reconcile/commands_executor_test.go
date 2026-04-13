package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	backenddocker "github.com/lauritsk/hatchctl/internal/backend/docker"
	docker "github.com/lauritsk/hatchctl/internal/backend/testdocker"
	dockercli "github.com/lauritsk/hatchctl/internal/backend/testdockercli"
	"github.com/lauritsk/hatchctl/internal/bridge"
	"github.com/lauritsk/hatchctl/internal/capability"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/policy"
	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

type fakeExecutorEngine struct {
	capabilities         backend.Capabilities
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

func (f *fakeExecutorEngine) ID() string { return "docker" }

func (f *fakeExecutorEngine) Capabilities() backend.Capabilities {
	if f.capabilities == (backend.Capabilities{}) {
		return backend.Capabilities{Bridge: true, ProjectServices: true}
	}
	return f.capabilities
}

func (f *fakeExecutorEngine) BridgeHost() string { return "host.docker.internal" }

func (f *fakeExecutorEngine) BuildDefinitionFileName() string { return "Dockerfile" }

func (f *fakeExecutorEngine) ConnectContainer(context.Context, string, int, io.Reader, io.Writer) error {
	return errors.New("unexpected connect container")
}

func (f *fakeExecutorEngine) InspectImage(ctx context.Context, ref string) (backend.ImageInspect, error) {
	if f.inspectImageFunc != nil {
		inspect, err := f.inspectImageFunc(ctx, dockercli.InspectImageRequest{Reference: ref})
		return toBackendImageInspect(inspect), err
	}
	return backend.ImageInspect{}, errors.New("unexpected inspect image")
}

func (f *fakeExecutorEngine) InspectContainer(ctx context.Context, containerID string) (backend.ContainerInspect, error) {
	if f.inspectContainerFunc != nil {
		inspect, err := f.inspectContainerFunc(ctx, dockercli.InspectContainerRequest{ContainerID: containerID})
		return toBackendContainerInspect(inspect), err
	}
	return backend.ContainerInspect{}, errors.New("unexpected inspect container")
}

func (f *fakeExecutorEngine) BuildImage(ctx context.Context, req backend.BuildImageRequest) error {
	if f.buildImageFunc != nil {
		return f.buildImageFunc(ctx, dockercli.BuildImageRequest{ContextDir: req.ContextDir, Dockerfile: req.DefinitionFile, Tag: req.Tag, Labels: req.Labels, BuildArgs: req.BuildArgs, Target: req.Target, ExtraOptions: req.ExtraOptions, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return errors.New("unexpected build image")
}

func (f *fakeExecutorEngine) PullImage(ctx context.Context, req backend.PullImageRequest) error {
	if f.pullImageFunc != nil {
		return f.pullImageFunc(ctx, dockercli.PullImageRequest{Reference: req.Reference, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return errors.New("unexpected pull image")
}

func (f *fakeExecutorEngine) RunDetachedContainer(ctx context.Context, req backend.RunDetachedContainerRequest) (string, error) {
	if f.runDetachedFunc != nil {
		return f.runDetachedFunc(ctx, dockercli.RunDetachedContainerRequest{Name: req.Name, Labels: req.Labels, Mounts: req.Mounts, Init: req.Init, Privileged: req.Privileged, CapAdd: req.CapAdd, SecurityOpt: req.SecurityOpt, Env: req.Env, ExtraArgs: req.ExtraArgs, Image: req.Image, Command: req.Command, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return "", errors.New("unexpected run detached container")
}

func (f *fakeExecutorEngine) StartContainer(ctx context.Context, req backend.StartContainerRequest) error {
	if f.startContainerFunc != nil {
		return f.startContainerFunc(ctx, dockercli.StartContainerRequest{ContainerID: req.ContainerID, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return errors.New("unexpected start container")
}

func (f *fakeExecutorEngine) RemoveContainer(ctx context.Context, req backend.RemoveContainerRequest) error {
	if f.removeContainerFunc != nil {
		return f.removeContainerFunc(ctx, dockercli.RemoveContainerRequest{ContainerID: req.ContainerID, Force: req.Force, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return errors.New("unexpected remove container")
}

func (f *fakeExecutorEngine) ListContainers(ctx context.Context, req backend.ListContainersRequest) (string, error) {
	if f.listContainersFunc != nil {
		filters := make([]string, 0, len(req.Labels))
		for _, key := range sortedTestKeys(req.Labels) {
			filters = append(filters, "label="+key+"="+req.Labels[key])
		}
		return f.listContainersFunc(ctx, dockercli.ListContainersRequest{All: req.All, Quiet: req.Quiet, Filters: filters, Dir: req.Dir})
	}
	return "", nil
}

func (f *fakeExecutorEngine) ProjectConfig(ctx context.Context, req backend.ProjectConfigRequest) (backend.ProjectConfig, error) {
	if f.composeConfigFunc != nil {
		output, err := f.composeConfigFunc(ctx, dockercli.ComposeConfigRequest{Target: dockercli.ComposeTarget{Files: req.Target.Files, Project: req.Target.Project, Dir: req.Target.Dir}, Format: "json"})
		if err != nil {
			return backend.ProjectConfig{}, err
		}
		if start := strings.Index(output, "{"); start >= 0 {
			output = output[start:]
		}
		var config backend.ProjectConfig
		if err := json.Unmarshal([]byte(output), &config); err != nil {
			return backend.ProjectConfig{}, err
		}
		return config, nil
	}
	return backend.ProjectConfig{}, errors.New("unexpected project config")
}

func (f *fakeExecutorEngine) BuildProject(ctx context.Context, req backend.ProjectBuildRequest) error {
	if f.composeBuildFunc != nil {
		return f.composeBuildFunc(ctx, dockercli.ComposeBuildRequest{Target: dockercli.ComposeTarget{Files: req.Target.Files, Project: req.Target.Project, Dir: req.Target.Dir}, Services: req.Services, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return errors.New("unexpected project build")
}

func (f *fakeExecutorEngine) UpProject(ctx context.Context, req backend.ProjectUpRequest) error {
	if f.composeUpFunc != nil {
		files := append([]string(nil), req.Target.Files...)
		var overridePath string
		if req.Override != nil {
			overridePath = filepath.Join(req.StateDir, "project-service.override.yml")
			if err := os.MkdirAll(req.StateDir, 0o700); err != nil {
				return err
			}
			var data strings.Builder
			if req.Override.Image != "" {
				data.WriteString("image: ")
				data.WriteString(req.Override.Image)
				data.WriteString("\n")
			}
			if err := os.WriteFile(overridePath, []byte(data.String()), 0o600); err != nil {
				return err
			}
			files = append(files, overridePath)
		}
		err := f.composeUpFunc(ctx, dockercli.ComposeUpRequest{Target: dockercli.ComposeTarget{Files: files, Project: req.Target.Project, Dir: req.Target.Dir}, Services: req.Services, NoBuild: req.NoBuild, Detach: req.Detach, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
		if overridePath != "" {
			_ = os.Remove(overridePath)
		}
		return err
	}
	return errors.New("unexpected project up")
}

func (f *fakeExecutorEngine) ProjectContainers(ctx context.Context, req backend.ProjectContainersRequest) ([]backend.ContainerInspect, *backend.ContainerInspect, error) {
	if f.listContainersFunc == nil || f.inspectContainerFunc == nil {
		return nil, nil, nil
	}
	output, err := f.listContainersFunc(ctx, dockercli.ListContainersRequest{All: true, Quiet: true, Filters: []string{"label=com.docker.compose.project=" + req.Target.Project}})
	if err != nil {
		return nil, nil, err
	}
	inspects, err := inspectContainerList(ctx, output, func(ctx context.Context, id string) (backend.ContainerInspect, error) {
		inspect, err := f.inspectContainerFunc(ctx, dockercli.InspectContainerRequest{ContainerID: id})
		return toBackendContainerInspect(inspect), err
	})
	if err != nil {
		return nil, nil, err
	}
	if req.Target.Service == "" {
		return inspects, nil, nil
	}
	var primaryCandidates []backend.ContainerInspect
	for _, inspect := range inspects {
		if inspect.Config.Labels["com.docker.compose.service"] == req.Target.Service {
			primaryCandidates = append(primaryCandidates, inspect)
		}
	}
	if len(primaryCandidates) == 0 {
		return inspects, nil, nil
	}
	best := bestContainer(primaryCandidates)
	return inspects, &best, nil
}

func (f *fakeExecutorEngine) Exec(ctx context.Context, req backend.ExecRequest) error {
	if f.execFunc != nil {
		return f.execFunc(ctx, dockercli.ExecRequest{ContainerID: req.ContainerID, User: req.User, Workdir: req.Workdir, Interactive: req.Interactive, TTY: req.TTY, Env: req.Env, Command: req.Command, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return errors.New("unexpected exec")
}

func (f *fakeExecutorEngine) ExecOutput(ctx context.Context, req backend.ExecRequest) (string, error) {
	if f.execOutputFunc != nil {
		return f.execOutputFunc(ctx, dockercli.ExecRequest{ContainerID: req.ContainerID, User: req.User, Workdir: req.Workdir, Interactive: req.Interactive, TTY: req.TTY, Env: req.Env, Command: req.Command, Streams: dockercli.Streams{Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr}})
	}
	return "", errors.New("unexpected exec output")
}

func toBackendImageInspect(inspect docker.ImageInspect) backend.ImageInspect {
	return inspect
}

func toBackendContainerInspect(inspect docker.ContainerInspect) backend.ContainerInspect {
	return inspect
}

func sortedTestKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type testResolvedConfigOptions struct {
	configDir  string
	stateDir   string
	cacheDir   string
	config     spec.Config
	features   []devcontainer.ResolvedFeature
	sourceKind string
	labels     map[string]string
}

func testResolvedConfig(opts testResolvedConfigOptions) devcontainer.ResolvedConfig {
	configPath := filepath.Join(opts.configDir, "devcontainer.json")
	workspaceFolder := "/workspace"
	remoteWorkspace := opts.config.WorkspaceFolder
	if remoteWorkspace == "" {
		remoteWorkspace = "/workspaces/demo"
	}
	merged := spec.MergeMetadata(opts.config, featureMetadata(opts.features))
	labels := opts.labels
	if labels == nil {
		labels = map[string]string{
			devcontainer.HostFolderLabel: workspaceFolder,
			devcontainer.ConfigFileLabel: configPath,
			devcontainer.ManagedByLabel:  devcontainer.ManagedByValue,
		}
	}
	sourceKind := opts.sourceKind
	if sourceKind == "" {
		sourceKind = "image"
	}
	return devcontainer.ResolvedConfig{
		WorkspaceFolder: workspaceFolder,
		ConfigPath:      configPath,
		ConfigDir:       opts.configDir,
		Config:          opts.config,
		Features:        opts.features,
		Merged:          merged,
		StateDir:        opts.stateDir,
		CacheDir:        opts.cacheDir,
		WorkspaceMount:  "type=bind,source=/workspace,target=/workspaces/demo",
		RemoteWorkspace: remoteWorkspace,
		ImageName:       "hatchctl-demo",
		SourceKind:      sourceKind,
		ContainerName:   "hatchctl-demo-container",
		Labels:          labels,
	}
}

func writableResolvedResolver(resolved devcontainer.ResolvedConfig) *workspaceplan.Resolver {
	return &workspaceplan.Resolver{
		Resolve: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return resolved, nil
		},
	}
}

func readOnlyResolvedResolver(resolved devcontainer.ResolvedConfig) *workspaceplan.Resolver {
	return &workspaceplan.Resolver{
		ResolveReadOnly: func(context.Context, string, string, devcontainer.ResolveOptions) (devcontainer.ResolvedConfig, error) {
			return resolved, nil
		},
	}
}

func fakePasswdExecOutput(t *testing.T) func(context.Context, dockercli.ExecRequest) (string, error) {
	t.Helper()
	return func(_ context.Context, req dockercli.ExecRequest) (string, error) {
		if len(req.Command) != 2 || req.Command[0] != "cat" || req.Command[1] != passwdFilePath {
			t.Fatalf("unexpected exec output command %#v", req.Command)
		}
		return "root:x:0:0:root:/root:/bin/sh\nvscode:x:1000:1000::/home/vscode:/bin/bash\n", nil
	}
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
	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir: configDir,
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		config: spec.Config{
			Image:               "mcr.microsoft.com/devcontainers/base:ubuntu",
			WorkspaceFolder:     "/workspaces/demo",
			UpdateRemoteUserUID: &falseValue,
		},
		features: []devcontainer.ResolvedFeature{{Path: featureDir, Metadata: spec.MetadataEntry{ID: "mise"}}},
	})
	sourceMetadataLabel, err := spec.MetadataLabelValue([]spec.MetadataEntry{{RemoteUser: "vscode", UpdateRemoteUserUID: &falseValue}})
	if err != nil {
		t.Fatalf("metadata label: %v", err)
	}
	managedMetadataLabel, err := spec.MetadataLabelValue([]spec.MetadataEntry{{ID: "mise"}})
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
					return docker.ImageInspect{}, &backenddocker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
				}
				return docker.ImageInspect{
					Architecture: "arm64",
					Config:       docker.InspectConfig{Labels: map[string]string{devcontainer.ImageMetadataLabel: sourceMetadataLabel}},
				}, nil
			case "hatchctl-demo":
				if !imageBuilt {
					return docker.ImageInspect{}, &backenddocker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
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
		execOutputFunc: fakePasswdExecOutput(t),
		execFunc: func(_ context.Context, req dockercli.ExecRequest) error {
			execRequests = append(execRequests, req)
			return nil
		},
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = engine
	executor.planner = writableResolvedResolver(resolvedConfig)

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
	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir: configDir,
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		config: spec.Config{
			Image:               "mcr.microsoft.com/devcontainers/base:ubuntu",
			WorkspaceFolder:     "/workspaces/demo",
			UpdateRemoteUserUID: &falseValue,
		},
		features: []devcontainer.ResolvedFeature{{Path: featureDir, Metadata: spec.MetadataEntry{ID: "mise"}}},
	})
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
	sourceMetadataLabel, err := spec.MetadataLabelValue([]spec.MetadataEntry{{RemoteUser: "vscode", UpdateRemoteUserUID: &falseValue}})
	if err != nil {
		t.Fatalf("source metadata label: %v", err)
	}
	managedMetadataLabel, err := spec.MetadataLabelValue([]spec.MetadataEntry{{ID: "mise"}})
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
					return docker.ImageInspect{}, &backenddocker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
				}
				return docker.ImageInspect{Architecture: "arm64", Config: docker.InspectConfig{Labels: map[string]string{devcontainer.ImageMetadataLabel: sourceMetadataLabel}}}, nil
			case "hatchctl-demo":
				if !imageBuilt {
					return docker.ImageInspect{}, &backenddocker.Error{Args: []string{"image", "inspect", req.Reference}, Stderr: "No such image", Err: errors.New("not found")}
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
		execOutputFunc: fakePasswdExecOutput(t),
		execFunc: func(_ context.Context, req dockercli.ExecRequest) error {
			execRequests = append(execRequests, req)
			return nil
		},
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = engine
	executor.planner = writableResolvedResolver(resolvedConfig)

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
	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir: configDir,
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		config: spec.Config{
			Image:           "mcr.microsoft.com/devcontainers/base:ubuntu",
			WorkspaceFolder: "/workspaces/demo",
		},
	})
	if err := storefs.WriteWorkspaceState(stateDir, storefs.WorkspaceState{ContainerID: "container-123"}); err != nil {
		t.Fatalf("write state: %v", err)
	}
	containerMetadataLabel, err := spec.MetadataLabelValue([]spec.MetadataEntry{{ID: "mise"}})
	if err != nil {
		t.Fatalf("container metadata label: %v", err)
	}
	sourceMetadataLabel, err := spec.MetadataLabelValue([]spec.MetadataEntry{{RemoteUser: "vscode"}})
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
		execOutputFunc: fakePasswdExecOutput(t),
		execFunc: func(_ context.Context, req dockercli.ExecRequest) error {
			execReq = req
			return nil
		},
	}

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = engine
	executor.planner = readOnlyResolvedResolver(resolvedConfig)

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
	configDir := t.TempDir()
	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir: configDir,
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		config:    spec.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu"},
	})
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
	executor.planner = writableResolvedResolver(resolvedConfig)

	if _, err := executor.Build(context.Background(), workspaceplan.WorkspacePlan{LockProtected: workspaceplan.LockProtectedArtifacts{StateDir: stateDir}, Trust: workspaceplan.TrustPlan{WorkspaceAllowed: true}}, BuildOptions{}); err != nil {
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

func TestBuildRejectsUnsupportedComposeBackend(t *testing.T) {
	t.Parallel()

	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir:  t.TempDir(),
		stateDir:   t.TempDir(),
		cacheDir:   t.TempDir(),
		config:     spec.Config{Service: "app", WorkspaceFolder: "/workspaces/demo"},
		sourceKind: "compose",
	})
	resolvedConfig.ComposeFiles = []string{"compose.yml"}
	resolvedConfig.ComposeProject = "demo"
	resolvedConfig.ComposeService = "app"

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = &fakeExecutorEngine{capabilities: backend.Capabilities{Bridge: true, ProjectServices: false}}
	executor.planner = writableResolvedResolver(resolvedConfig)

	_, err := executor.Build(context.Background(), workspaceplan.WorkspacePlan{Trust: workspaceplan.TrustPlan{WorkspaceAllowed: true}}, BuildOptions{})
	var unsupported backend.UnsupportedCapabilityError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected unsupported capability error, got %v", err)
	}
	if unsupported.Capability != "compose-based devcontainers" {
		t.Fatalf("unexpected capability %#v", unsupported)
	}
}

func TestEnsureBackendSupportRejectsUnsupportedBridge(t *testing.T) {
	t.Parallel()

	executor := NewExecutorWithIO(nil, nil, io.Discard, io.Discard)
	executor.engine = &fakeExecutorEngine{capabilities: backend.Capabilities{Bridge: false, ProjectServices: true}}

	err := executor.ensureBackendSupport(testResolvedConfig(testResolvedConfigOptions{configDir: t.TempDir(), stateDir: t.TempDir(), cacheDir: t.TempDir(), config: spec.Config{Image: "alpine:3.23"}}), true)
	var unsupported backend.UnsupportedCapabilityError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected unsupported capability error, got %v", err)
	}
	if unsupported.Capability != "bridge integration" {
		t.Fatalf("unexpected capability %#v", unsupported)
	}
}

func TestBridgeHostsForBackendPrefersPodmanAlias(t *testing.T) {
	t.Parallel()

	if got := bridgeHostsForBackend("podman", ""); !reflect.DeepEqual(got, []string{"host.containers.internal", "host.docker.internal"}) {
		t.Fatalf("unexpected podman bridge hosts %#v", got)
	}
	if got := bridgeHostsForBackend("docker", ""); !reflect.DeepEqual(got, []string{"host.docker.internal", "host.containers.internal"}) {
		t.Fatalf("unexpected docker bridge hosts %#v", got)
	}
}

func TestReadConfigReportsBridgeAndDotfilesState(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	configDir := t.TempDir()
	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir: configDir,
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		config:    spec.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"},
	})
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
	executor.planner = readOnlyResolvedResolver(resolvedConfig)

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
	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir: configDir,
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		config:    spec.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"},
	})
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
	executor.planner = writableResolvedResolver(resolvedConfig)

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
	resolvedConfig := testResolvedConfig(testResolvedConfigOptions{
		configDir: configDir,
		stateDir:  stateDir,
		cacheDir:  cacheDir,
		config:    spec.Config{Image: "mcr.microsoft.com/devcontainers/base:ubuntu", WorkspaceFolder: "/workspaces/demo"},
	})
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
	executor.planner = readOnlyResolvedResolver(resolvedConfig)

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
