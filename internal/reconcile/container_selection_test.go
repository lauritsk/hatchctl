package reconcile

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/lauritsk/hatchctl/internal/docker"
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

	best, err := selectBestContainer(context.Background(), "stopped\nrunning\n", func(_ context.Context, id string) (docker.ContainerInspect, error) {
		switch id {
		case "stopped":
			return docker.ContainerInspect{ID: id, State: docker.ContainerState{Running: false}}, nil
		case "running":
			return docker.ContainerInspect{ID: id, State: docker.ContainerState{Running: true}}, nil
		default:
			return docker.ContainerInspect{}, errors.New("unexpected container")
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

	best, err := selectBestContainer(context.Background(), "missing\nlive\n", func(_ context.Context, id string) (docker.ContainerInspect, error) {
		if id == "missing" {
			return docker.ContainerInspect{}, &docker.Error{Stderr: "No such object", Err: os.ErrNotExist}
		}
		return docker.ContainerInspect{ID: id, State: docker.ContainerState{Running: true}}, nil
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

	best := bestContainer([]docker.ContainerInspect{
		{ID: "zulu", State: docker.ContainerState{Running: true}},
		{ID: "alpha", State: docker.ContainerState{Running: true}},
		{ID: "bravo", State: docker.ContainerState{Running: false}},
	})
	if best.ID != "alpha" {
		t.Fatalf("expected lowest running container id, got %#v", best)
	}
}
