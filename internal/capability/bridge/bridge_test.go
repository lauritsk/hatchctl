package bridgecap

import (
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func TestEnabledFromInspectPrefersContainerLabel(t *testing.T) {
	t.Parallel()

	inspect := &backend.ContainerInspect{Config: backend.InspectConfig{Labels: map[string]string{devcontainer.BridgeEnabledLabel: "true"}}}
	if !EnabledFromInspect(inspect, storefs.WorkspaceState{}) {
		t.Fatal("expected bridge label to enable capability")
	}
	if !EnabledFromInspect(nil, storefs.WorkspaceState{BridgeEnabled: true}) {
		t.Fatal("expected persisted bridge state to enable capability")
	}
	if EnabledFromInspect(nil, storefs.WorkspaceState{}) {
		t.Fatal("expected bridge capability to stay disabled")
	}
}
