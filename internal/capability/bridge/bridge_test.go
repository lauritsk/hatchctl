package bridgecap

import (
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestEnabledFromInspectPrefersContainerLabel(t *testing.T) {
	t.Parallel()

	inspect := &docker.ContainerInspect{Config: docker.InspectConfig{Labels: map[string]string{devcontainer.BridgeEnabledLabel: "true"}}}
	if !EnabledFromInspect(inspect, devcontainer.State{}) {
		t.Fatal("expected bridge label to enable capability")
	}
	if !EnabledFromInspect(nil, devcontainer.State{BridgeEnabled: true}) {
		t.Fatal("expected persisted bridge state to enable capability")
	}
	if EnabledFromInspect(nil, devcontainer.State{}) {
		t.Fatal("expected bridge capability to stay disabled")
	}
}
