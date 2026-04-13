package docker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/command"
)

type fakeRunner struct {
	outputFn         func(command.Command) (string, string, error)
	combinedOutputFn func(command.Command) (string, error)
}

func (f fakeRunner) Run(context.Context, command.Command) error {
	return errors.New("unexpected run")
}

func (f fakeRunner) Output(_ context.Context, cmd command.Command) (string, string, error) {
	if f.outputFn == nil {
		return "", "", errors.New("unexpected output")
	}
	return f.outputFn(cmd)
}

func (f fakeRunner) CombinedOutput(_ context.Context, cmd command.Command) (string, error) {
	if f.combinedOutputFn == nil {
		return "", errors.New("unexpected combined output")
	}
	return f.combinedOutputFn(cmd)
}

func (f fakeRunner) Start(command.StartOptions) (*os.Process, error) {
	return nil, errors.New("unexpected start")
}

func TestRenderProjectOverrideIncludesServiceDetails(t *testing.T) {
	t.Parallel()

	contents, err := renderProjectOverride("app", backend.ProjectOverride{
		PullPolicy:  "never",
		Labels:      map[string]string{"a": "1"},
		Environment: map[string]string{"B": "2"},
		Mounts: []backend.ProjectMount{{
			Type:     "volume",
			Source:   "deps",
			Target:   "/deps",
			ReadOnly: true,
		}},
		Image:        "managed-image",
		NamedVolumes: []string{"deps"},
	})
	if err != nil {
		t.Fatalf("render project override: %v", err)
	}
	for _, want := range []string{"app:", "pull_policy: never", "a=1", "B=2", "image: managed-image", "source: deps", "target: /deps", "read_only: true", "volumes:", "deps:"} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected override to contain %q, got:\n%s", want, contents)
		}
	}
}

func TestProjectContainersReturnsPrimaryServiceContainer(t *testing.T) {
	t.Parallel()

	client := &Client{Binary: "docker", runner: fakeRunner{
		outputFn: func(cmd command.Command) (string, string, error) {
			if strings.Join(cmd.Args, " ") != "ps -a -q --filter label=com.docker.compose.project=demo" {
				t.Fatalf("unexpected output command %#v", cmd.Args)
			}
			return "db\napp\n", "", nil
		},
		combinedOutputFn: func(cmd command.Command) (string, error) {
			if len(cmd.Args) != 2 || cmd.Args[0] != "inspect" {
				t.Fatalf("unexpected inspect command %#v", cmd.Args)
			}
			switch cmd.Args[1] {
			case "db":
				return `[{"Id":"db","Config":{"Labels":{"com.docker.compose.service":"db"}},"State":{"Running":true}}]`, nil
			case "app":
				return `[{"Id":"app","Config":{"Labels":{"com.docker.compose.service":"app"}},"State":{"Running":true}}]`, nil
			default:
				t.Fatalf("unexpected inspect target %q", cmd.Args[1])
				return "", nil
			}
		},
	}}

	inspects, primary, err := client.ProjectContainers(context.Background(), backend.ProjectContainersRequest{Target: backend.ProjectTarget{Project: "demo", Service: "app"}})
	if err != nil {
		t.Fatalf("project containers: %v", err)
	}
	if len(inspects) != 2 {
		t.Fatalf("expected 2 containers, got %#v", inspects)
	}
	if primary == nil || primary.ID != "app" {
		t.Fatalf("unexpected primary %#v", primary)
	}
}
