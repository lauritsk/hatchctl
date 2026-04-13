package bridge

import (
	"github.com/lauritsk/hatchctl/internal/backend"
	backendfactory "github.com/lauritsk/hatchctl/internal/backend/factory"
	"github.com/lauritsk/hatchctl/internal/command"
)

type bridgeRuntimeDeps struct {
	hostCommand      command.Runner
	backend          backend.Client
	containerConnect containerConnectRunner
}

var defaultBridgeRuntimeDeps = newDefaultBridgeRuntimeDeps()

func newDefaultBridgeRuntimeDeps() bridgeRuntimeDeps {
	deps, _ := newBridgeRuntimeDeps("docker")
	return deps
}

func newBridgeRuntimeDeps(name string) (bridgeRuntimeDeps, error) {
	client, err := newBridgeBackend(name)
	if err != nil {
		return bridgeRuntimeDeps{}, err
	}
	return bridgeRuntimeDeps{
		hostCommand:      command.Local{},
		backend:          client,
		containerConnect: containerConnectWithBackend(client),
	}, nil
}

func newBridgeBackend(name string) (backend.Client, error) {
	return backendfactory.New(name)
}
