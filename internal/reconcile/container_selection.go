package reconcile

import (
	"context"
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend"
)

type containerInspector func(context.Context, string) (backend.ContainerInspect, error)

func inspectContainerList(ctx context.Context, output string, inspect containerInspector) ([]backend.ContainerInspect, error) {
	ids := uniqueContainerIDs(output)
	inspects := make([]backend.ContainerInspect, 0, len(ids))
	for _, id := range ids {
		container, err := inspect(ctx, id)
		if err != nil {
			if backend.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		inspects = append(inspects, container)
	}
	return inspects, nil
}

func selectBestContainer(ctx context.Context, output string, inspect containerInspector) (backend.ContainerInspect, error) {
	inspects, err := inspectContainerList(ctx, output, inspect)
	if err != nil {
		return backend.ContainerInspect{}, err
	}
	if len(inspects) == 0 {
		return backend.ContainerInspect{}, errManagedContainerNotFound
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

func bestContainer(inspects []backend.ContainerInspect) backend.ContainerInspect {
	best := inspects[0]
	for _, candidate := range inspects[1:] {
		if candidate.State.Running != best.State.Running {
			if candidate.State.Running {
				best = candidate
			}
			continue
		}
		if candidate.ID < best.ID {
			best = candidate
		}
	}
	return best
}

func inspectContainerWithEngine(engine engine) containerInspector {
	return func(ctx context.Context, id string) (backend.ContainerInspect, error) {
		return engine.InspectContainer(ctx, id)
	}
}

func inspectContainerWithObserverBackend(observer observerBackend) containerInspector {
	return func(ctx context.Context, id string) (backend.ContainerInspect, error) {
		return observer.InspectContainer(ctx, id)
	}
}
