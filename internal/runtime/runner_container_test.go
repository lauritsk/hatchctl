package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestSelectBestContainerIDPrefersRunningContainer(t *testing.T) {
	t.Parallel()

	runner := NewRunner(docker.NewClient(fakeRuntimeDockerBinary(t, `
if [ "$1" = "inspect" ] && [ "$2" = "stopped" ]; then
  printf '[{"Id":"stopped","Config":{"Labels":{},"Env":[]},"State":{"Status":"exited","Running":false}}]'
  exit 0
fi
if [ "$1" = "inspect" ] && [ "$2" = "running" ]; then
  printf '[{"Id":"running","Config":{"Labels":{},"Env":[]},"State":{"Status":"running","Running":true}}]'
  exit 0
fi
printf 'unexpected args: %s\n' "$*" >&2
exit 99
`)))

	id, err := runner.selectBestContainerID(context.Background(), "stopped\nrunning\n")
	if err != nil {
		t.Fatalf("select container: %v", err)
	}
	if id != "running" {
		t.Fatalf("expected running container, got %q", id)
	}
}

func fakeRuntimeDockerBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker binary: %v", err)
	}
	return path
}
