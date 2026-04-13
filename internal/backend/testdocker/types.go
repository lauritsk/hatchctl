package testdocker

import (
	"github.com/lauritsk/hatchctl/internal/backend"
	backenddocker "github.com/lauritsk/hatchctl/internal/backend/docker"
)

type (
	InspectConfig    = backend.InspectConfig
	ImageInspect     = backend.ImageInspect
	ContainerState   = backend.ContainerState
	ContainerMount   = backend.ContainerMount
	ContainerInspect = backend.ContainerInspect
	Error            = backenddocker.Error
)
