package dockercli

import (
	"context"
	"io"
	"sort"

	"github.com/lauritsk/hatchctl/internal/docker"
)

type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type CommandRequest struct {
	Args []string
	Dir  string
	Env  []string
	Streams
}

type InspectImageRequest struct {
	Reference string
}

type InspectContainerRequest struct {
	ContainerID string
}

type BuildImageRequest struct {
	ContextDir   string
	Dockerfile   string
	Tag          string
	Labels       map[string]string
	BuildArgs    map[string]string
	Target       string
	ExtraOptions []string
	Streams
}

type RunDetachedContainerRequest struct {
	Name        string
	Labels      map[string]string
	Mounts      []string
	Init        bool
	Privileged  bool
	CapAdd      []string
	SecurityOpt []string
	Env         map[string]string
	ExtraArgs   []string
	Image       string
	Command     []string
	Streams
}

type StartContainerRequest struct {
	ContainerID string
	Streams
}

type RemoveContainerRequest struct {
	ContainerID string
	Force       bool
	Streams
}

type ListContainersRequest struct {
	All     bool
	Quiet   bool
	Filters []string
	Dir     string
}

type ComposeTarget struct {
	Files   []string
	Project string
	Dir     string
}

type ComposeConfigRequest struct {
	Target ComposeTarget
	Format string
}

type ComposeBuildRequest struct {
	Target   ComposeTarget
	Services []string
	Streams
}

type ComposeUpRequest struct {
	Target   ComposeTarget
	Services []string
	NoBuild  bool
	Detach   bool
	Streams
}

type ExecRequest struct {
	ContainerID string
	User        string
	Workdir     string
	Interactive bool
	TTY         bool
	Env         map[string]string
	Command     []string
	Streams
}

type dockerTransport interface {
	Run(context.Context, docker.RunOptions) error
	OutputOptions(context.Context, docker.RunOptions) (string, error)
	InspectImage(context.Context, string) (docker.ImageInspect, error)
	InspectContainer(context.Context, string) (docker.ContainerInspect, error)
}

type Client struct {
	transport dockerTransport
}

func New(transport dockerTransport) *Client {
	return &Client{transport: transport}
}

func (c *Client) Run(ctx context.Context, req CommandRequest) error {
	return c.transport.Run(ctx, docker.RunOptions{Args: req.Args, Dir: req.Dir, Env: req.Env, Stdin: req.Stdin, Stdout: req.Stdout, Stderr: req.Stderr})
}

func (c *Client) Output(ctx context.Context, req CommandRequest) (string, error) {
	return c.transport.OutputOptions(ctx, docker.RunOptions{Args: req.Args, Dir: req.Dir, Env: req.Env, Stdin: req.Stdin})
}

func (c *Client) InspectImage(ctx context.Context, req InspectImageRequest) (docker.ImageInspect, error) {
	return c.transport.InspectImage(ctx, req.Reference)
}

func (c *Client) InspectContainer(ctx context.Context, req InspectContainerRequest) (docker.ContainerInspect, error) {
	return c.transport.InspectContainer(ctx, req.ContainerID)
}

func (c *Client) BuildImage(ctx context.Context, req BuildImageRequest) error {
	args := []string{"build", "-f", req.Dockerfile, "-t", req.Tag}
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
	return c.Run(ctx, CommandRequest{Args: args, Streams: req.Streams})
}

func (c *Client) RunDetachedContainer(ctx context.Context, req RunDetachedContainerRequest) (string, error) {
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
	return c.Output(ctx, CommandRequest{Args: args, Streams: req.Streams})
}

func (c *Client) StartContainer(ctx context.Context, req StartContainerRequest) error {
	return c.Run(ctx, CommandRequest{Args: []string{"start", req.ContainerID}, Streams: req.Streams})
}

func (c *Client) RemoveContainer(ctx context.Context, req RemoveContainerRequest) error {
	args := []string{"rm"}
	if req.Force {
		args = append(args, "-f")
	}
	args = append(args, req.ContainerID)
	return c.Run(ctx, CommandRequest{Args: args, Streams: req.Streams})
}

func (c *Client) ListContainers(ctx context.Context, req ListContainersRequest) (string, error) {
	args := []string{"ps"}
	if req.All {
		args = append(args, "-a")
	}
	if req.Quiet {
		args = append(args, "-q")
	}
	for _, filter := range req.Filters {
		args = append(args, "--filter", filter)
	}
	return c.Output(ctx, CommandRequest{Args: args, Dir: req.Dir})
}

func (c *Client) ComposeConfig(ctx context.Context, req ComposeConfigRequest) (string, error) {
	args := composeBaseArgs(req.Target)
	args = append(args, "config")
	if req.Format != "" {
		args = append(args, "--format", req.Format)
	}
	return c.Output(ctx, CommandRequest{Args: args, Dir: req.Target.Dir})
}

func (c *Client) ComposeBuild(ctx context.Context, req ComposeBuildRequest) error {
	args := composeBaseArgs(req.Target)
	args = append(args, "build")
	args = append(args, req.Services...)
	return c.Run(ctx, CommandRequest{Args: args, Dir: req.Target.Dir, Streams: req.Streams})
}

func (c *Client) ComposeUp(ctx context.Context, req ComposeUpRequest) error {
	args := composeBaseArgs(req.Target)
	args = append(args, "up")
	if req.NoBuild {
		args = append(args, "--no-build")
	}
	if req.Detach {
		args = append(args, "-d")
	}
	args = append(args, req.Services...)
	return c.Run(ctx, CommandRequest{Args: args, Dir: req.Target.Dir, Streams: req.Streams})
}

func (c *Client) Exec(ctx context.Context, req ExecRequest) error {
	return c.Run(ctx, CommandRequest{Args: execArgs(req), Streams: req.Streams})
}

func (c *Client) ExecOutput(ctx context.Context, req ExecRequest) (string, error) {
	return c.Output(ctx, CommandRequest{Args: execArgs(req), Streams: req.Streams})
}

func execArgs(req ExecRequest) []string {
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

func composeBaseArgs(target ComposeTarget) []string {
	args := []string{"compose"}
	for _, file := range target.Files {
		args = append(args, "-f", file)
	}
	if target.Project != "" {
		args = append(args, "-p", target.Project)
	}
	return args
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
