package testdockercli

import "github.com/lauritsk/hatchctl/internal/backend"

type Streams = backend.Streams

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

type PullImageRequest struct {
	Reference string
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
