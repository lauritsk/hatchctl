package runtime

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/docker"
)

type containerEngine interface {
	Run(context.Context, docker.RunOptions) error
	Output(context.Context, ...string) (string, error)
	OutputOptions(context.Context, docker.RunOptions) (string, error)
	InspectImage(context.Context, string) (docker.ImageInspect, error)
	InspectContainer(context.Context, string) (docker.ContainerInspect, error)
}
