package backend

import (
	"context"
	"fmt"
	"io"
)

type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type InspectConfig struct {
	Labels map[string]string `json:"Labels"`
	User   string            `json:"User"`
	Env    []string          `json:"Env"`
}

type ImageInspect struct {
	ID           string        `json:"Id"`
	Architecture string        `json:"Architecture"`
	Os           string        `json:"Os"`
	Config       InspectConfig `json:"Config"`
}

type ContainerState struct {
	Status  string `json:"Status"`
	Running bool   `json:"Running"`
}

type ContainerMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

type ContainerInspect struct {
	ID     string           `json:"Id"`
	Name   string           `json:"Name"`
	Image  string           `json:"Image"`
	Config InspectConfig    `json:"Config"`
	State  ContainerState   `json:"State"`
	Mounts []ContainerMount `json:"Mounts"`
}

type BuildImageRequest struct {
	ContextDir     string
	DefinitionFile string
	Tag            string
	Labels         map[string]string
	BuildArgs      map[string]string
	Target         string
	ExtraOptions   []string
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
	All    bool
	Quiet  bool
	Labels map[string]string
	Dir    string
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

type ProjectTarget struct {
	Files   []string
	Project string
	Service string
	Dir     string
}

type ProjectConfigRequest struct {
	Target ProjectTarget
}

type ProjectBuildRequest struct {
	Target   ProjectTarget
	Services []string
	Streams
}

type ProjectUpRequest struct {
	Target   ProjectTarget
	Services []string
	NoBuild  bool
	Detach   bool
	Override *ProjectOverride
	StateDir string
	Streams
}

type ProjectContainersRequest struct {
	Target ProjectTarget
}

type ProjectConfig struct {
	Name     string                    `json:"name"`
	Services map[string]ProjectService `json:"services"`
}

type ProjectService struct {
	Image string               `json:"image"`
	Build *ProjectServiceBuild `json:"build"`
}

type ProjectServiceBuild struct {
	Context        string            `json:"context"`
	DefinitionFile string            `json:"dockerfile"`
	Target         string            `json:"target"`
	Args           map[string]string `json:"args"`
}

func (b *ProjectServiceBuild) Enabled() bool {
	return b != nil && b.Context != ""
}

type ProjectOverride struct {
	PullPolicy   string
	Labels       map[string]string
	Environment  map[string]string
	Mounts       []ProjectMount
	Init         bool
	Privileged   bool
	User         string
	Command      []string
	CapAdd       []string
	SecurityOpt  []string
	Image        string
	NamedVolumes []string
}

type ProjectMount struct {
	Type        string
	Source      string
	Target      string
	ReadOnly    bool
	Consistency string
	Bind        *ProjectBindMount
	Volume      *ProjectVolumeMount
}

type ProjectBindMount struct {
	Propagation    string
	CreateHostPath *bool
	SELinux        string
}

type ProjectVolumeMount struct {
	NoCopy  bool
	Subpath string
}

type Capabilities struct {
	Bridge          bool
	ProjectServices bool
}

type Client interface {
	ID() string
	Capabilities() Capabilities
	BridgeHost() string
	BuildDefinitionFileName() string
	InspectImage(context.Context, string) (ImageInspect, error)
	InspectContainer(context.Context, string) (ContainerInspect, error)
	BuildImage(context.Context, BuildImageRequest) error
	PullImage(context.Context, PullImageRequest) error
	RunDetachedContainer(context.Context, RunDetachedContainerRequest) (string, error)
	StartContainer(context.Context, StartContainerRequest) error
	RemoveContainer(context.Context, RemoveContainerRequest) error
	ListContainers(context.Context, ListContainersRequest) (string, error)
	ProjectConfig(context.Context, ProjectConfigRequest) (ProjectConfig, error)
	BuildProject(context.Context, ProjectBuildRequest) error
	UpProject(context.Context, ProjectUpRequest) error
	ProjectContainers(context.Context, ProjectContainersRequest) ([]ContainerInspect, *ContainerInspect, error)
	Exec(context.Context, ExecRequest) error
	ExecOutput(context.Context, ExecRequest) (string, error)
	ConnectContainer(context.Context, string, int, io.Reader, io.Writer) error
}

type notFoundMarker interface {
	error
	NotFound() bool
}

type exitCodeMarker interface {
	error
	ExitCode() (int, bool)
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	marked, ok := err.(notFoundMarker)
	return ok && marked.NotFound()
}

func ExitCode(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	marked, ok := err.(exitCodeMarker)
	if !ok {
		return 0, false
	}
	return marked.ExitCode()
}

type UnsupportedBackendError struct {
	Name string
}

func (e UnsupportedBackendError) Error() string {
	return fmt.Sprintf("unsupported backend %q", e.Name)
}

type UnsupportedCapabilityError struct {
	Backend    string
	Capability string
}

func (e UnsupportedCapabilityError) Error() string {
	return fmt.Sprintf("backend %q does not support %s", e.Backend, e.Capability)
}
