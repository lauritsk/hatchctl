package runtime

import (
	"context"
	"testing"

	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestSelectBestContainerIDPrefersRunningContainer(t *testing.T) {
	t.Parallel()

	backend := &fakeRuntimeBackend{
		inspectContainer: func(_ context.Context, containerID string) (docker.ContainerInspect, error) {
			switch containerID {
			case "stopped":
				return docker.ContainerInspect{ID: "stopped", Config: docker.InspectConfig{Labels: map[string]string{}}, State: docker.ContainerState{Status: "exited", Running: false}}, nil
			case "running":
				return docker.ContainerInspect{ID: "running", Config: docker.InspectConfig{Labels: map[string]string{}}, State: docker.ContainerState{Status: "running", Running: true}}, nil
			default:
				t.Fatalf("unexpected container inspect %q", containerID)
				return docker.ContainerInspect{}, nil
			}
		},
	}
	runner := newTestRunner(t, backend)

	id, err := runner.selectBestContainerID(context.Background(), "stopped\nrunning\n")
	if err != nil {
		t.Fatalf("select container: %v", err)
	}
	if id != "running" {
		t.Fatalf("expected running container, got %q", id)
	}
}
