//go:build integration

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/docker"
)

func dockerClientForTest(t *testing.T) *docker.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}
	client := docker.NewClient("docker")
	dockerAvailabilityForTests.once.Do(func() {
		_, dockerAvailabilityForTests.err = client.Output(context.Background(), "version", "--format", "{{.Server.Version}}")
	})
	if dockerAvailabilityForTests.err != nil {
		t.Skipf("docker unavailable: %v", dockerAvailabilityForTests.err)
	}
	return client
}

func requireIntegrationCommands(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s unavailable: %v", name, err)
		}
	}
}

func sharedAlpineBaseImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, &cachedIntegrationFixtures.plainImage, "base", "FROM alpine:3.20\n")
}

func sharedAlpineWithCommandImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, &cachedIntegrationFixtures.plainWithCMDImage, "base-cmd", "FROM alpine:3.20\nCMD [\"/bin/sh\",\"-lc\",\"trap 'exit 0' TERM INT; while sleep 1000; do :; done\"]\n")
}

func sharedAppUserImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, &cachedIntegrationFixtures.appUserImage, "app-user", "FROM alpine:3.20\nRUN adduser -D -u 1000 app\nUSER app\n")
}

func sharedDataDirBaseImage(t *testing.T, client *docker.Client, ctx context.Context) string {
	t.Helper()
	return sharedTaggedImage(t, client, ctx, nil, metadataImageTagForKey("data-dir"), "FROM alpine:3.20\nRUN mkdir -p /usr/local/share\n")
}

func sharedTaggedImage(t *testing.T, client *docker.Client, ctx context.Context, slot *string, key string, dockerfile string) string {
	t.Helper()
	tag := metadataImageTagForKey(key)
	if slot != nil {
		cachedIntegrationFixtures.mu.Lock()
		if *slot == "" {
			*slot = tag
		}
		tag = *slot
		cachedIntegrationFixtures.mu.Unlock()
	}
	buildImageIfMissing(t, client, ctx, tag, dockerfile)
	return tag
}

func buildImageIfMissing(t *testing.T, client *docker.Client, ctx context.Context, tag string, dockerfile string) {
	t.Helper()
	if _, err := client.InspectImage(ctx, tag); err == nil {
		return
	}
	if err := client.Run(ctx, docker.RunOptions{Args: []string{"build", "-t", tag, sharedDockerBuildContext(t, dockerfile)}}); err != nil {
		t.Fatalf("build shared image %q: %v", tag, err)
	}
}

func sharedDockerBuildContext(t *testing.T, dockerfile string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func metadataImageTagForKey(key string) string {
	return "hatchctl-test-" + sanitizeName(key)
}

func workspaceKey(workspace string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(workspace))
	return fmt.Sprintf("%s-%x", sanitizeName(filepath.Base(workspace)), hash.Sum64())
}

func readJSONFile(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func writeLocalFeature(t *testing.T, dir string, manifest string, install string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devcontainer-feature.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte(install), 0o755); err != nil {
		t.Fatal(err)
	}
}

func envMap(values []string) map[string]string {
	result := map[string]string{}
	for _, value := range values {
		key, raw, ok := strings.Cut(value, "=")
		if ok {
			result[key] = raw
		}
	}
	return result
}

func assertDotfilesInstallCount(t *testing.T, runner *Runner, ctx context.Context, workspace string, want int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := runner.Exec(ctx, ExecOptions{
		Workspace: workspace,
		Args:      []string{"sh", "-lc", `wc -l < "$HOME/.config/hatchctl-dotfiles/count"`},
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("read dotfiles install count: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("unexpected count exit code %d (stderr: %s)", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != fmt.Sprintf("%d", want) {
		t.Fatalf("unexpected dotfiles install count %q want %d", got, want)
	}
}

func initGitRepoForTest(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if name == "install" || strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
		}
	}
	runGit("init")
	runGit("add", ".")
	runGit(
		"-c", "user.name=Test User",
		"-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
		"commit", "-m", "init",
	)
}

func cloneGitRepoBareForTest(t *testing.T, src string, dst string) {
	t.Helper()
	cmd := exec.Command("git", "clone", "--bare", src, dst)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone --bare: %v\n%s", err, string(output))
	}
}

func startGitDaemonForTest(t *testing.T, client *docker.Client, ctx context.Context, networkName string, image string, repoPath string) string {
	t.Helper()
	if _, err := client.Output(ctx, "network", "create", networkName); err != nil {
		t.Fatalf("create docker network: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"network", "rm", networkName}})
	})

	serverName := networkName + "-git"
	if _, err := client.Output(ctx,
		"run", "-d",
		"--name", serverName,
		"--network", networkName,
		"--network-alias", "dotfiles",
		"--mount", fmt.Sprintf("type=bind,source=%s,target=/srv/git/dotfiles.git,readonly", repoPath),
		image,
		"sh", "-lc", "exec git daemon --reuseaddr --base-path=/srv/git --export-all --verbose /srv/git",
	); err != nil {
		t.Fatalf("start git daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Run(ctx, docker.RunOptions{Args: []string{"rm", "-f", serverName}})
	})
	return serverName
}

func waitForGitRepoForTest(t *testing.T, client *docker.Client, ctx context.Context, networkName string, image string, serverName string, repoURL string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := client.Run(ctx, docker.RunOptions{Args: []string{
			"run", "--rm",
			"--network", networkName,
			image,
			"sh", "-lc", "git ls-remote " + quoteShell(repoURL) + " >/dev/null 2>&1",
		}})
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			logs, logErr := client.CombinedOutput(ctx, "logs", serverName)
			if logErr != nil {
				logs = fmt.Sprintf("(unable to read logs: %v)", logErr)
			}
			t.Fatalf("wait for git repo %s: %v\nserver logs:\n%s", repoURL, err, logs)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func sanitizeName(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return fmt.Sprintf("tmp-%d", os.Getpid())
	}
	return result
}
