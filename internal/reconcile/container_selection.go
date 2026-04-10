package reconcile

import (
	"context"
	"sort"
	"strings"

	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
)

type containerInspector func(context.Context, string) (docker.ContainerInspect, error)

func inspectContainerList(ctx context.Context, output string, inspect containerInspector) ([]docker.ContainerInspect, error) {
	ids := uniqueContainerIDs(output)
	inspects := make([]docker.ContainerInspect, 0, len(ids))
	for _, id := range ids {
		container, err := inspect(ctx, id)
		if err != nil {
			if docker.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		inspects = append(inspects, container)
	}
	return inspects, nil
}

func selectBestContainer(ctx context.Context, output string, inspect containerInspector) (docker.ContainerInspect, error) {
	inspects, err := inspectContainerList(ctx, output, inspect)
	if err != nil {
		return docker.ContainerInspect{}, err
	}
	if len(inspects) == 0 {
		return docker.ContainerInspect{}, errManagedContainerNotFound
	}
	return bestContainer(inspects), nil
}

func uniqueContainerIDs(output string) []string {
	ids := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		ids = append(ids, line)
	}
	return ids
}

func bestContainer(inspects []docker.ContainerInspect) docker.ContainerInspect {
	ordered := append([]docker.ContainerInspect(nil), inspects...)
	sort.Slice(ordered, func(i int, j int) bool {
		if ordered[i].State.Running != ordered[j].State.Running {
			return ordered[i].State.Running
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered[0]
}

func inspectContainerWithEngine(engine engine) containerInspector {
	return func(ctx context.Context, id string) (docker.ContainerInspect, error) {
		return engine.InspectContainer(ctx, dockercli.InspectContainerRequest{ContainerID: id})
	}
}

func inspectContainerWithObserverBackend(backend backend) containerInspector {
	return func(ctx context.Context, id string) (docker.ContainerInspect, error) {
		return backend.InspectContainer(ctx, id)
	}
}
