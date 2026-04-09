package bridge

import (
	"github.com/lauritsk/hatchctl/internal/command"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type bridgeRuntimeDeps struct {
	hostCommand      command.Runner
	docker           *docker.Client
	store            bridgeFileStore
	containerConnect containerConnectRunner
}

var defaultBridgeRuntimeDeps = newDefaultBridgeRuntimeDeps()

func newDefaultBridgeRuntimeDeps() bridgeRuntimeDeps {
	client := docker.NewClient("docker")
	return bridgeRuntimeDeps{
		hostCommand:      command.Local{},
		docker:           client,
		store:            filesystemBridgeStore{},
		containerConnect: containerConnectWithDocker(client),
	}
}

func bridgeStore() bridgeFileStore {
	return defaultBridgeRuntimeDeps.store
}
