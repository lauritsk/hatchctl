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

func TestProjectConfigFallsBackToPlainComposeConfig(t *testing.T) {
	t.Parallel()

	client := &Client{Binary: "podman", runner: fakeRunner{
		outputFn: func(cmd command.Command) (string, string, error) {
			switch strings.Join(cmd.Args, " ") {
			case "compose -p demo config --format json":
				return "", "unknown flag: --format", errors.New("unknown flag")
			case "compose -p demo config":
				return "name: demo\nservices:\n  app:\n    image: alpine:3.23\n", "", nil
			default:
				t.Fatalf("unexpected output command %#v", cmd.Args)
			}
			return "", "", nil
		},
	}}

	config, err := client.ProjectConfig(context.Background(), backend.ProjectConfigRequest{Target: backend.ProjectTarget{Project: "demo"}})
	if err != nil {
		t.Fatalf("project config: %v", err)
	}
	if config.Name != "demo" || config.Services["app"].Image != "alpine:3.23" {
		t.Fatalf("unexpected config %#v", config)
	}
}

func TestProjectContainersReturnsPrimaryServiceContainer(t *testing.T) {
	t.Parallel()

	client := &Client{Binary: "docker", runner: fakeRunner{
		outputFn: func(cmd command.Command) (string, string, error) {
			switch strings.Join(cmd.Args, " ") {
			case "compose -p demo ps -a -q":
				return "db\napp\n", "", nil
			case "compose -p demo ps -q app":
				return "app\n", "", nil
			default:
				t.Fatalf("unexpected output command %#v", cmd.Args)
			}
			return "", "", nil
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

func TestProjectConfigUsesExternalComposeBinary(t *testing.T) {
	t.Parallel()

	client := &Client{Binary: "podman", composeBinary: "podman-compose", runner: fakeRunner{
		outputFn: func(cmd command.Command) (string, string, error) {
			if cmd.Binary != "podman-compose" {
				t.Fatalf("unexpected command binary %q", cmd.Binary)
			}
			if strings.Join(cmd.Args, " ") != "-p demo config --format json" {
				t.Fatalf("unexpected output command %#v", cmd.Args)
			}
			return `{"name":"demo","services":{"app":{"image":"alpine:3.23"}}}`, "", nil
		},
	}}

	config, err := client.ProjectConfig(context.Background(), backend.ProjectConfigRequest{Target: backend.ProjectTarget{Project: "demo"}})
	if err != nil {
		t.Fatalf("project config: %v", err)
	}
	if config.Name != "demo" || config.Services["app"].Image != "alpine:3.23" {
		t.Fatalf("unexpected config %#v", config)
	}
}

func TestProjectConfigReturnsFallbackErrorWhenPlainComposeAlsoFails(t *testing.T) {
	t.Parallel()

	client := &Client{Binary: "podman", runner: fakeRunner{
		outputFn: func(cmd command.Command) (string, string, error) {
			switch strings.Join(cmd.Args, " ") {
			case "compose -p demo config --format json":
				return "", "unknown flag: --format", errors.New("json config failed")
			case "compose -p demo config":
				return "", "compose file missing", errors.New("plain config failed")
			default:
				t.Fatalf("unexpected output command %#v", cmd.Args)
			}
			return "", "", nil
		},
	}}

	_, err := client.ProjectConfig(context.Background(), backend.ProjectConfigRequest{Target: backend.ProjectTarget{Project: "demo"}})
	if err == nil {
		t.Fatal("expected project config to fail")
	}
	if !strings.Contains(err.Error(), "compose file missing") {
		t.Fatalf("expected fallback compose error, got %v", err)
	}
	if strings.Contains(err.Error(), "unknown flag: --format") {
		t.Fatalf("expected fallback error to replace json format error, got %v", err)
	}
}

func TestErrorNotFoundMatchesPodmanMessage(t *testing.T) {
	t.Parallel()

	err := &Error{Binary: "podman", Stderr: "[]\nError: alpine:3.23: image not known", Err: errors.New("exit status 125")}
	if !err.NotFound() {
		t.Fatal("expected podman image not known error to count as not found")
	}
}
