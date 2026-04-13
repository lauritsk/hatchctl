package podman

import (
	"os/exec"
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend/docker"
)

func New(binary string) *docker.Client {
	composeBinary, composeCommand := composeOptions(binary)
	return docker.NewWithOptions(docker.Options{
		Binary:              binary,
		RuntimeID:           "podman",
		BridgeHost:          "host.containers.internal",
		BuildDefinitionFile: "Containerfile",
		ComposeBinary:       composeBinary,
		ComposeCommand:      composeCommand,
	})
}

func composeOptions(binary string) (string, []string) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "podman"
	}
	if supportsNativeCompose(binary) {
		return "", []string{"compose"}
	}
	if _, err := exec.LookPath("podman-compose"); err == nil {
		return "podman-compose", nil
	}
	return "", []string{"compose"}
}

func supportsNativeCompose(binary string) bool {
	cmd := exec.Command(binary, "compose", "version")
	return cmd.Run() == nil
}
