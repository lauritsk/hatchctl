package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/command"
	"go.yaml.in/yaml/v3"
)

const projectOverrideFileName = "project-service.override.yml"

type Client struct {
	Binary              string
	runtimeID           string
	bridgeHost          string
	buildDefinitionFile string
	composeBinary       string
	composeCommand      []string
	runner              command.Runner
}

type Options struct {
	Binary              string
	RuntimeID           string
	BridgeHost          string
	BuildDefinitionFile string
	ComposeBinary       string
	ComposeCommand      []string
}

type runOptions struct {
	Binary string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Dir    string
	Env    []string
	Args   []string
}

type Error struct {
	Binary string
	Args   []string
	Stderr string
	Err    error
}

func New(binary string) *Client {
	return NewWithOptions(Options{Binary: binary, RuntimeID: "docker", BridgeHost: "host.docker.internal", BuildDefinitionFile: "Dockerfile", ComposeCommand: []string{"compose"}})
}

func NewWithOptions(opts Options) *Client {
	if strings.TrimSpace(opts.Binary) == "" {
		opts.Binary = "docker"
	}
	if strings.TrimSpace(opts.RuntimeID) == "" {
		opts.RuntimeID = "docker"
	}
	if strings.TrimSpace(opts.BridgeHost) == "" {
		opts.BridgeHost = "host.docker.internal"
	}
	if strings.TrimSpace(opts.BuildDefinitionFile) == "" {
		opts.BuildDefinitionFile = "Dockerfile"
	}
	if len(opts.ComposeCommand) == 0 {
		opts.ComposeCommand = []string{"compose"}
	}
	return &Client{
		Binary:              strings.TrimSpace(opts.Binary),
		runtimeID:           opts.RuntimeID,
		bridgeHost:          opts.BridgeHost,
		buildDefinitionFile: opts.BuildDefinitionFile,
		composeBinary:       strings.TrimSpace(opts.ComposeBinary),
		composeCommand:      append([]string(nil), opts.ComposeCommand...),
		runner:              command.Local{},
	}
}

func (c *Client) ID() string {
	if c.runtimeID == "" {
		return "docker"
	}
	return c.runtimeID
}

func (c *Client) Capabilities() backend.Capabilities {
	return backend.Capabilities{Bridge: true, ProjectServices: true}
}

func (c *Client) BuildDefinitionFileName() string {
	if c.buildDefinitionFile == "" {
		return "Dockerfile"
	}
	return c.buildDefinitionFile
}

func (c *Client) BridgeHost() string {
	if c.bridgeHost == "" {
		return "host.docker.internal"
	}
	return c.bridgeHost
}

func (e *Error) Error() string {
	message := strings.TrimSpace(e.Stderr)
	if message == "" {
		message = e.Err.Error()
	}
	binary := e.Binary
	if binary == "" {
		binary = "docker"
	}
	return fmt.Sprintf("%s %s: %s", binary, strings.Join(e.Args, " "), message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) ExitCode() (int, bool) {
	var exitErr *exec.ExitError
	if !errors.As(e.Err, &exitErr) {
		return 0, false
	}
	return exitErr.ExitCode(), true
}

func (e *Error) NotFound() bool {
	message := strings.ToLower(e.Stderr)
	return strings.Contains(message, "no such") || strings.Contains(message, "not found") || strings.Contains(message, "not known")
}

func (c *Client) InspectImage(ctx context.Context, image string) (backend.ImageInspect, error) {
	data, err := c.combinedOutput(ctx, "image", "inspect", image)
	if err != nil {
		return backend.ImageInspect{}, err
	}
	var values []backend.ImageInspect
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		return backend.ImageInspect{}, fmt.Errorf("parse image inspect: %w", err)
	}
	if len(values) == 0 {
		return backend.ImageInspect{}, fmt.Errorf("image %q not found", image)
	}
	return values[0], nil
}

func (c *Client) InspectContainer(ctx context.Context, container string) (backend.ContainerInspect, error) {
	data, err := c.combinedOutput(ctx, "inspect", container)
	if err != nil {
		return backend.ContainerInspect{}, err
	}
	var values []backend.ContainerInspect
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		return backend.ContainerInspect{}, fmt.Errorf("parse container inspect: %w", err)
	}
	if len(values) == 0 {
		return backend.ContainerInspect{}, fmt.Errorf("container %q not found", container)
	}
	return values[0], nil
}

func (c *Client) BuildImage(ctx context.Context, req backend.BuildImageRequest) error {
	args := []string{"build", "-f", req.DefinitionFile, "-t", req.Tag}
	for _, key := range sortedKeys(req.Labels) {
		args = append(args, "--label", key+"="+req.Labels[key])
	}
	if req.Target != "" {
		args = append(args, "--target", req.Target)
	}
	for _, key := range sortedKeys(req.BuildArgs) {
		args = append(args, "--build-arg", key+"="+req.BuildArgs[key])
	}
	args = append(args, req.ExtraOptions...)
	args = append(args, req.ContextDir)
	return c.run(ctx, runOptions{Args: args, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) PullImage(ctx context.Context, req backend.PullImageRequest) error {
	return c.run(ctx, runOptions{Args: []string{"pull", req.Reference}, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) RunDetachedContainer(ctx context.Context, req backend.RunDetachedContainerRequest) (string, error) {
	args := []string{"run", "-d", "--name", req.Name}
	for _, key := range sortedKeys(req.Labels) {
		args = append(args, "--label", key+"="+req.Labels[key])
	}
	for _, mount := range req.Mounts {
		args = append(args, "--mount", mount)
	}
	if req.Init {
		args = append(args, "--init")
	}
	if req.Privileged {
		args = append(args, "--privileged")
	}
	for _, capAdd := range req.CapAdd {
		args = append(args, "--cap-add", capAdd)
	}
	for _, securityOpt := range req.SecurityOpt {
		args = append(args, "--security-opt", securityOpt)
	}
	for _, key := range sortedKeys(req.Env) {
		args = append(args, "-e", key+"="+req.Env[key])
	}
	args = append(args, req.ExtraArgs...)
	args = append(args, req.Image)
	args = append(args, req.Command...)
	return c.output(ctx, runOptions{Args: args, Stdin: req.Stdin})
}

func (c *Client) StartContainer(ctx context.Context, req backend.StartContainerRequest) error {
	return c.run(ctx, runOptions{Args: []string{"start", req.ContainerID}, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) RemoveContainer(ctx context.Context, req backend.RemoveContainerRequest) error {
	args := []string{"rm"}
	if req.Force {
		args = append(args, "-f")
	}
	args = append(args, req.ContainerID)
	return c.run(ctx, runOptions{Args: args, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) ListContainers(ctx context.Context, req backend.ListContainersRequest) (string, error) {
	args := []string{"ps"}
	if req.All {
		args = append(args, "-a")
	}
	if req.Quiet {
		args = append(args, "-q")
	}
	for _, key := range sortedKeys(req.Labels) {
		args = append(args, "--filter", "label="+key+"="+req.Labels[key])
	}
	return c.output(ctx, runOptions{Args: args, Dir: req.Dir})
}

func (c *Client) ProjectConfig(ctx context.Context, req backend.ProjectConfigRequest) (backend.ProjectConfig, error) {
	jsonArgs := append(c.composeBaseArgs(req.Target), "config", "--format", "json")
	output, err := c.output(ctx, runOptions{Binary: c.composeBinaryValue(), Args: jsonArgs, Dir: req.Target.Dir})
	if err == nil {
		return parseProjectConfig(output)
	}
	plainArgs := append(c.composeBaseArgs(req.Target), "config")
	plainOutput, plainErr := c.output(ctx, runOptions{Binary: c.composeBinaryValue(), Args: plainArgs, Dir: req.Target.Dir})
	if plainErr != nil {
		return backend.ProjectConfig{}, err
	}
	return parseProjectConfig(plainOutput)
}

func (c *Client) BuildProject(ctx context.Context, req backend.ProjectBuildRequest) error {
	args := c.composeBaseArgs(req.Target)
	args = append(args, "build")
	args = append(args, req.Services...)
	return c.run(ctx, runOptions{Binary: c.composeBinaryValue(), Args: args, Dir: req.Target.Dir, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) UpProject(ctx context.Context, req backend.ProjectUpRequest) error {
	target := req.Target
	if req.Override != nil {
		overridePath, err := writeProjectOverride(req.StateDir, req.Target.Service, *req.Override)
		if err != nil {
			return err
		}
		defer os.Remove(overridePath)
		target.Files = append(append([]string(nil), req.Target.Files...), overridePath)
	}
	args := c.composeBaseArgs(target)
	args = append(args, "up")
	if req.NoBuild {
		args = append(args, "--no-build")
	}
	if req.Detach {
		args = append(args, "-d")
	}
	args = append(args, req.Services...)
	return c.run(ctx, runOptions{Binary: c.composeBinaryValue(), Args: args, Dir: target.Dir, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) ProjectContainers(ctx context.Context, req backend.ProjectContainersRequest) ([]backend.ContainerInspect, *backend.ContainerInspect, error) {
	output, err := c.output(ctx, runOptions{Binary: c.composeBinaryValue(), Args: append(c.composeBaseArgs(req.Target), "ps", "-a", "-q"), Dir: req.Target.Dir})
	if err != nil {
		return nil, nil, err
	}
	inspects, err := inspectContainerList(ctx, output, c.InspectContainer)
	if err != nil {
		return nil, nil, err
	}
	if len(inspects) == 0 {
		return nil, nil, nil
	}
	if req.Target.Service == "" {
		return inspects, nil, nil
	}
	primaryOutput, err := c.output(ctx, runOptions{Binary: c.composeBinaryValue(), Args: append(c.composeBaseArgs(req.Target), "ps", "-q", req.Target.Service), Dir: req.Target.Dir})
	if err != nil {
		return nil, nil, err
	}
	primaryCandidates, err := inspectContainerList(ctx, primaryOutput, c.InspectContainer)
	if err != nil {
		return nil, nil, err
	}
	if len(primaryCandidates) == 0 {
		return inspects, nil, nil
	}
	best := bestContainer(primaryCandidates)
	return inspects, &best, nil
}

func (c *Client) Exec(ctx context.Context, req backend.ExecRequest) error {
	return c.run(ctx, runOptions{Args: execArgs(req), Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) ExecOutput(ctx context.Context, req backend.ExecRequest) (string, error) {
	return c.output(ctx, runOptions{Args: execArgs(req), Stdin: req.Stdin})
}

func (c *Client) ConnectContainer(ctx context.Context, containerID string, port int, stdin io.Reader, stdout io.Writer) error {
	var stderr strings.Builder
	err := c.run(ctx, runOptions{Args: []string{"exec", "-i", containerID, "/var/run/hatchctl/bridge/bin/hatchctl", "bridge", "helper", "connect", "--port", strconv.Itoa(port)}, Stdin: stdin, Stdout: stdout, Stderr: &stderr})
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("container bridge helper connect: %s", message)
	}
	return nil
}

func (c *Client) run(ctx context.Context, opts runOptions) error {
	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if shouldCombineStreams(opts.Args) {
			target := opts.Stdout
			if target == nil {
				target = opts.Stderr
			}
			combined := &capturingWriter{target: target}
			binary := valueOrDefault(opts.Binary, c.Binary)
			err := c.runner.Run(ctx, command.Command{Binary: binary, Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin, Stdout: combined, Stderr: combined})
			if err == nil {
				return nil
			}
			if !retryableExit255(err) || attempt == maxAttempts || ctx.Err() != nil {
				return &Error{Binary: binary, Args: append([]string(nil), opts.Args...), Stderr: combined.String(), Err: err}
			}
			if err := sleepWithContext(ctx, time.Duration(attempt)*time.Second); err != nil {
				return &Error{Binary: binary, Args: append([]string(nil), opts.Args...), Stderr: combined.String(), Err: err}
			}
			continue
		}
		stderr := opts.Stderr
		var stderrBuffer bytes.Buffer
		if stderr == nil {
			stderr = &stderrBuffer
		}
		binary := valueOrDefault(opts.Binary, c.Binary)
		err := c.runner.Run(ctx, command.Command{Binary: binary, Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin, Stdout: opts.Stdout, Stderr: stderr})
		if err == nil {
			return nil
		}
		if !retryableExit255(err) || attempt == maxAttempts || ctx.Err() != nil {
			return &Error{Binary: binary, Args: append([]string(nil), opts.Args...), Stderr: stderrBuffer.String(), Err: err}
		}
		if err := sleepWithContext(ctx, time.Duration(attempt)*time.Second); err != nil {
			return &Error{Binary: binary, Args: append([]string(nil), opts.Args...), Stderr: stderrBuffer.String(), Err: err}
		}
	}
	return nil
}

func (c *Client) output(ctx context.Context, opts runOptions) (string, error) {
	binary := valueOrDefault(opts.Binary, c.Binary)
	stdout, stderr, err := c.runner.Output(ctx, command.Command{Binary: binary, Args: opts.Args, Dir: opts.Dir, Env: opts.Env, Stdin: opts.Stdin})
	if err != nil {
		return "", &Error{Binary: binary, Args: append([]string(nil), opts.Args...), Stderr: stderr, Err: err}
	}
	return stdout, nil
}

func (c *Client) combinedOutput(ctx context.Context, args ...string) (string, error) {
	data, err := c.runner.CombinedOutput(ctx, command.Command{Binary: c.Binary, Args: args})
	if err != nil {
		return "", &Error{Binary: c.Binary, Args: append([]string(nil), args...), Stderr: data, Err: err}
	}
	return data, nil
}

func (c *Client) composeBaseArgs(target backend.ProjectTarget) []string {
	args := append([]string(nil), c.composeCommand...)
	if len(args) == 0 && c.composeBinaryValue() == c.Binary {
		args = []string{"compose"}
	}
	for _, file := range target.Files {
		args = append(args, "-f", file)
	}
	if target.Project != "" {
		args = append(args, "-p", target.Project)
	}
	return args
}

func (c *Client) composeBinaryValue() string {
	if c.composeBinary != "" {
		return c.composeBinary
	}
	return c.Binary
}

func valueOrDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func execArgs(req backend.ExecRequest) []string {
	args := []string{"exec"}
	if req.Interactive {
		args = append(args, "-i")
	}
	if req.TTY {
		args = append(args, "-t")
	}
	if req.User != "" {
		args = append(args, "-u", req.User)
	}
	if req.Workdir != "" {
		args = append(args, "--workdir", req.Workdir)
	}
	for _, key := range sortedKeys(req.Env) {
		args = append(args, "-e", key+"="+req.Env[key])
	}
	args = append(args, req.ContainerID)
	args = append(args, req.Command...)
	return args
}

func writeProjectOverride(stateDir string, serviceName string, override backend.ProjectOverride) (string, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(stateDir, projectOverrideFileName)
	contents, err := renderProjectOverride(serviceName, override)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

type composeOverrideFile struct {
	Services map[string]composeOverrideService `yaml:"services"`
	Volumes  map[string]composeOverrideVolume  `yaml:"volumes,omitempty"`
}

type yamlProjectConfig struct {
	Name     string                        `yaml:"name"`
	Services map[string]yamlProjectService `yaml:"services"`
}

type yamlProjectService struct {
	Image string                   `yaml:"image"`
	Build *yamlProjectServiceBuild `yaml:"build"`
}

type yamlProjectServiceBuild struct {
	Context    string            `yaml:"context"`
	Dockerfile string            `yaml:"dockerfile"`
	Target     string            `yaml:"target"`
	Args       map[string]string `yaml:"args"`
}

type composeOverrideService struct {
	PullPolicy  string                `yaml:"pull_policy,omitempty"`
	Labels      []string              `yaml:"labels,omitempty"`
	Environment []string              `yaml:"environment,omitempty"`
	Volumes     []composeServiceMount `yaml:"volumes,omitempty"`
	Init        bool                  `yaml:"init,omitempty"`
	Privileged  bool                  `yaml:"privileged,omitempty"`
	User        string                `yaml:"user,omitempty"`
	Command     []string              `yaml:"command,omitempty"`
	CapAdd      []string              `yaml:"cap_add,omitempty"`
	SecurityOpt []string              `yaml:"security_opt,omitempty"`
	Image       string                `yaml:"image,omitempty"`
}

type composeServiceMount struct {
	Type        string                     `yaml:"type,omitempty"`
	Source      string                     `yaml:"source,omitempty"`
	Target      string                     `yaml:"target,omitempty"`
	ReadOnly    bool                       `yaml:"read_only,omitempty"`
	Consistency string                     `yaml:"consistency,omitempty"`
	Bind        *composeBindMountOptions   `yaml:"bind,omitempty"`
	Volume      *composeVolumeMountOptions `yaml:"volume,omitempty"`
}

type composeBindMountOptions struct {
	Propagation    string `yaml:"propagation,omitempty"`
	CreateHostPath *bool  `yaml:"create_host_path,omitempty"`
	SELinux        string `yaml:"selinux,omitempty"`
}

type composeVolumeMountOptions struct {
	NoCopy  bool   `yaml:"nocopy,omitempty"`
	Subpath string `yaml:"subpath,omitempty"`
}

type composeOverrideVolume struct{}

func renderProjectOverride(serviceName string, override backend.ProjectOverride) (string, error) {
	service := composeOverrideService{PullPolicy: override.PullPolicy}
	for _, key := range sortedKeys(override.Labels) {
		service.Labels = append(service.Labels, key+"="+override.Labels[key])
	}
	for _, key := range sortedKeys(override.Environment) {
		service.Environment = append(service.Environment, key+"="+override.Environment[key])
	}
	for _, mount := range override.Mounts {
		service.Volumes = append(service.Volumes, composeServiceMountFromProjectMount(mount))
	}
	service.Init = override.Init
	service.Privileged = override.Privileged
	service.User = override.User
	service.Command = append([]string(nil), override.Command...)
	service.CapAdd = append([]string(nil), override.CapAdd...)
	service.SecurityOpt = append([]string(nil), override.SecurityOpt...)
	service.Image = override.Image
	file := composeOverrideFile{Services: map[string]composeOverrideService{serviceName: service}}
	if len(override.NamedVolumes) > 0 {
		file.Volumes = make(map[string]composeOverrideVolume, len(override.NamedVolumes))
		for _, name := range override.NamedVolumes {
			file.Volumes[name] = composeOverrideVolume{}
		}
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseProjectConfig(output string) (backend.ProjectConfig, error) {
	jsonOutput := sanitizeComposeJSONOutput(output)
	var config backend.ProjectConfig
	if err := json.Unmarshal([]byte(jsonOutput), &config); err == nil {
		return config, nil
	}
	var yamlConfig yamlProjectConfig
	if err := yaml.Unmarshal([]byte(output), &yamlConfig); err != nil {
		return backend.ProjectConfig{}, fmt.Errorf("parse project config: %w (output=%q)", err, truncateForError(strings.TrimSpace(output), 200))
	}
	config.Name = yamlConfig.Name
	if len(yamlConfig.Services) > 0 {
		config.Services = make(map[string]backend.ProjectService, len(yamlConfig.Services))
		for name, service := range yamlConfig.Services {
			config.Services[name] = backend.ProjectService{
				Image: service.Image,
				Build: projectServiceBuildFromYAML(service.Build),
			}
		}
	}
	return config, nil
}

func projectServiceBuildFromYAML(build *yamlProjectServiceBuild) *backend.ProjectServiceBuild {
	if build == nil {
		return nil
	}
	return &backend.ProjectServiceBuild{
		Context:        build.Context,
		DefinitionFile: build.Dockerfile,
		Target:         build.Target,
		Args:           build.Args,
	}
}

func composeServiceMountFromProjectMount(mount backend.ProjectMount) composeServiceMount {
	result := composeServiceMount{Type: mount.Type, Source: mount.Source, Target: mount.Target, ReadOnly: mount.ReadOnly, Consistency: mount.Consistency}
	if mount.Bind != nil {
		result.Bind = &composeBindMountOptions{Propagation: mount.Bind.Propagation, CreateHostPath: mount.Bind.CreateHostPath, SELinux: mount.Bind.SELinux}
	}
	if mount.Volume != nil {
		result.Volume = &composeVolumeMountOptions{NoCopy: mount.Volume.NoCopy, Subpath: mount.Volume.Subpath}
	}
	return result
}

func inspectContainerList(ctx context.Context, output string, inspect func(context.Context, string) (backend.ContainerInspect, error)) ([]backend.ContainerInspect, error) {
	ids := uniqueContainerIDs(output)
	inspects := make([]backend.ContainerInspect, 0, len(ids))
	for _, id := range ids {
		container, err := inspect(ctx, id)
		if err != nil {
			if backend.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		inspects = append(inspects, container)
	}
	return inspects, nil
}

func uniqueContainerIDs(output string) []string {
	ids := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		ids = append(ids, line)
	}
	return ids
}

func bestContainer(inspects []backend.ContainerInspect) backend.ContainerInspect {
	best := inspects[0]
	for _, candidate := range inspects[1:] {
		if candidate.State.Running != best.State.Running {
			if candidate.State.Running {
				best = candidate
			}
			continue
		}
		if candidate.ID < best.ID {
			best = candidate
		}
	}
	return best
}

func sanitizeComposeJSONOutput(output string) string {
	output = strings.TrimSpace(output)
	if strings.HasPrefix(output, "{") {
		return output
	}
	if start := strings.Index(output, "{"); start >= 0 {
		return strings.TrimSpace(output[start:])
	}
	return output
}

func truncateForError(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryableExit255(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.ExitCode() == 255
}

func shouldCombineStreams(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "build" {
		return true
	}
	for _, arg := range args[1:] {
		if arg == "build" || arg == "up" {
			return true
		}
	}
	return false
}

type capturingWriter struct {
	target  io.Writer
	mu      sync.Mutex
	capture bytes.Buffer
}

func (w *capturingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.target != nil {
		n, err := w.target.Write(p)
		if n > 0 {
			_, _ = w.capture.Write(p[:n])
		}
		return n, err
	}
	return w.capture.Write(p)
}

func (w *capturingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.capture.String()
}

func sortedKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
