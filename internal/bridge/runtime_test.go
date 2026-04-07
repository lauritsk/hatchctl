package bridge

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenHandlerRequiresValidToken(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", Token: "secret", Port: 1234, StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	handler := newBridgeHostService(session, "container", func(string) error { return nil }).handler()
	req := httptest.NewRequest(http.MethodPost, "/open", nil)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d", res.Code)
	}
}

func TestOpenHandlerForwardsURL(t *testing.T) {
	t.Parallel()
	statusPath := filepath.Join(t.TempDir(), "bridge-status.json")
	session := &Session{ID: "session", Token: "secret", Port: 1234, StatusPath: statusPath}
	var opened string
	service := newBridgeHostService(session, "container", func(target string) error {
		opened = target
		return nil
	})
	service.forwardURL = func(port int) (int, bool, error) { return port + 1000, false, nil }
	handler := service.handler()
	req := httptest.NewRequest(http.MethodPost, "/open?url=https%3A%2F%2Fexample.com&token=secret", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d", res.Code)
	}
	if opened != "https://example.com" {
		t.Fatalf("unexpected opened url %q", opened)
	}
	if _, err := os.Stat(statusPath); err != nil {
		t.Fatalf("expected status file: %v", err)
	}
}

func TestOpenHandlerRewritesLocalhostURL(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", Token: "secret", Port: 1234, StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	var opened string
	service := newBridgeHostService(session, "container", func(target string) error {
		opened = target
		return nil
	})
	service.forwardURL = func(port int) (int, bool, error) {
		if port != 8080 {
			t.Fatalf("unexpected container port %d", port)
		}
		return 19090, false, nil
	}
	handler := service.handler()
	req := httptest.NewRequest(http.MethodPost, "/open?url=http%3A%2F%2Flocalhost%3A8080%2Fcb&token=secret", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d", res.Code)
	}
	if opened != "http://127.0.0.1:19090/cb" {
		t.Fatalf("unexpected rewritten url %q", opened)
	}
}

func TestHelperConnectCopiesTraffic(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		data, _ := io.ReadAll(conn)
		_, _ = conn.Write(append([]byte("echo:"), data...))
	}()

	stdin := bytes.NewBufferString("hello")
	var stdout bytes.Buffer
	port := listener.Addr().(*net.TCPAddr).Port
	if err := helperConnect([]string{"--port", fmt.Sprintf("%d", port)}, stdin, &stdout); err != nil {
		t.Fatalf("helper connect: %v", err)
	}
	if got := stdout.String(); got != "echo:hello" {
		t.Fatalf("unexpected helper output %q", got)
	}
}

func TestConnectorExecArgsUsesHelperBinary(t *testing.T) {
	t.Parallel()
	got := strings.Join(connectorExecArgs("container123", 8123), " ")
	want := fmt.Sprintf("exec -i container123 %s connect --port 8123", containerHelperBin)
	if got != want {
		t.Fatalf("unexpected connector exec args %q", got)
	}
}

func TestHelperBinaryDataUsesConfiguredPath(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), helperArtifactName())
	if err := os.WriteFile(helperPath, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperBinaryEnvVar, helperPath)

	got, err := helperBinaryData()
	if err != nil {
		t.Fatalf("helper binary data: %v", err)
	}
	if string(got) != "helper" {
		t.Fatalf("unexpected helper data %q", string(got))
	}
}

func TestHelperBinaryCandidatesDoNotUseRepoLayoutFallbacks(t *testing.T) {
	t.Parallel()
	for _, candidate := range helperBinaryCandidates() {
		if strings.Contains(candidate, ".dist/") || strings.Contains(candidate, ".dist\\") {
			t.Fatalf("unexpected repo-layout helper candidate %q", candidate)
		}
	}
}

func TestPrepareAndRuntimeFilesUseOwnerOnlyPermissions(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), helperArtifactName())
	if err := os.WriteFile(helperPath, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperBinaryEnvVar, helperPath)

	stateDir := t.TempDir()
	session, err := Prepare(stateDir, true)
	if err != nil {
		t.Fatalf("prepare bridge session: %v", err)
	}
	if err := writeBridgeConfig(session, "container"); err != nil {
		t.Fatalf("write bridge config: %v", err)
	}
	if err := writeStatus(session, "container", "running", "", nil, 0, false); err != nil {
		t.Fatalf("write bridge status: %v", err)
	}

	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("unexpected mode for %s: got %o want %o", path, got, want)
		}
	}

	assertMode(session.StatePath, 0o700)
	assertMode(filepath.Join(session.StatePath, "session.json"), 0o600)
	assertMode(session.ConfigPath, 0o600)
	assertMode(session.StatusPath, 0o600)

	if err := os.WriteFile(session.PIDPath, []byte("123"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	assertMode(session.PIDPath, 0o600)
}

func TestBridgeListenAddressNarrowsLocalhostSessions(t *testing.T) {
	t.Parallel()
	if got := bridgeListenAddress(&Session{Host: "localhost", Port: 8123}); got != "127.0.0.1:8123" {
		t.Fatalf("unexpected localhost listen address %q", got)
	}
	if got := bridgeListenAddress(&Session{Host: "host.docker.internal", Port: 8123}); got != "0.0.0.0:8123" {
		t.Fatalf("unexpected default listen address %q", got)
	}
}
