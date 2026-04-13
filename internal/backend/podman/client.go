package podman

import "github.com/lauritsk/hatchctl/internal/backend/docker"

func New(binary string) *docker.Client {
	return docker.NewWithOptions(docker.Options{
		Binary:              binary,
		RuntimeID:           "podman",
		BridgeHost:          "host.containers.internal",
		BuildDefinitionFile: "Dockerfile",
		ComposeCommand:      []string{"compose"},
	})
}
