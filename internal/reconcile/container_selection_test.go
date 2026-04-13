package reconcile

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	backenddocker "github.com/lauritsk/hatchctl/internal/backend/docker"
)

func TestUniqueContainerIDsDeduplicatesAndSkipsEmptyLines(t *testing.T) {
	t.Parallel()

	got := uniqueContainerIDs("\nalpha\nalpha\n beta \n\n")
	want := []string{"alpha", "beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ids %#v", got)
	}
}

func TestSelectBestContainerPrefersRunningContainer(t *testing.T) {
	t.Parallel()

	best, err := selectBestContainer(context.Background(), "stopped\nrunning\n", func(_ context.Context, id string) (backend.ContainerInspect, error) {
		switch id {
		case "stopped":
			return backend.ContainerInspect{ID: id, State: backend.ContainerState{Running: false}}, nil
		case "running":
			return backend.ContainerInspect{ID: id, State: backend.ContainerState{Running: true}}, nil
		default:
			return backend.ContainerInspect{}, errors.New("unexpected container")
		}
	})
	if err != nil {
		t.Fatalf("select best container: %v", err)
	}
	if best.ID != "running" {
		t.Fatalf("expected running container, got %#v", best)
	}
}

func TestSelectBestContainerSkipsMissingContainers(t *testing.T) {
	t.Parallel()

	best, err := selectBestContainer(context.Background(), "missing\nlive\n", func(_ context.Context, id string) (backend.ContainerInspect, error) {
		if id == "missing" {
			return backend.ContainerInspect{}, &backenddocker.Error{Stderr: "No such object", Err: os.ErrNotExist}
		}
		return backend.ContainerInspect{ID: id, State: backend.ContainerState{Running: true}}, nil
	})
	if err != nil {
		t.Fatalf("select best container: %v", err)
	}
	if best.ID != "live" {
		t.Fatalf("expected remaining live container, got %#v", best)
	}
}

func TestBestContainerUsesLowestIDForTie(t *testing.T) {
	t.Parallel()

	best := bestContainer([]backend.ContainerInspect{
		{ID: "zulu", State: backend.ContainerState{Running: true}},
		{ID: "alpha", State: backend.ContainerState{Running: true}},
		{ID: "bravo", State: backend.ContainerState{Running: false}},
	})
	if best.ID != "alpha" {
		t.Fatalf("expected lowest running container id, got %#v", best)
	}
}
