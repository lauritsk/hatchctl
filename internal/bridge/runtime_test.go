package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/command"
)

type fakeBridgeRunner struct {
	runFunc   func(context.Context, command.Command) error
	startFunc func(command.StartOptions) (*os.Process, error)
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func (f fakeBridgeRunner) Run(ctx context.Context, cmd command.Command) error {
	if f.runFunc != nil {
		return f.runFunc(ctx, cmd)
	}
	return nil
}

func (f fakeBridgeRunner) Output(context.Context, command.Command) (string, string, error) {
	return "", "", nil
}

func (f fakeBridgeRunner) CombinedOutput(context.Context, command.Command) (string, error) {
	return "", nil
}

func (f fakeBridgeRunner) Start(opts command.StartOptions) (*os.Process, error) {
	if f.startFunc != nil {
		return f.startFunc(opts)
	}
	return nil, errors.New("unexpected start")
}

func TestBridgeHostServiceHandlesOpenRequest(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
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
	client, server := net.Pipe()
	defer client.Close()
	go service.handleConn(server)
	if err := writeBridgeRequest(client, bridgeRequest{Kind: "open", URL: "http://localhost:8080/cb"}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	response, err := readBridgeResponse(client)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !response.OK {
		t.Fatalf("unexpected response %#v", response)
	}
	if opened != "http://localhost:19090/cb" {
		t.Fatalf("unexpected rewritten url %q", opened)
	}
}

func TestDefaultOpenUsesConfiguredCommand(t *testing.T) {
	original := defaultBridgeRuntimeDeps
	t.Cleanup(func() { defaultBridgeRuntimeDeps = original })
	t.Setenv("HATCHCTL_BRIDGE_OPEN_COMMAND", "printf bridge-open")

	var got command.Command
	defaultBridgeRuntimeDeps = bridgeRuntimeDeps{hostCommand: fakeBridgeRunner{runFunc: func(_ context.Context, cmd command.Command) error {
		got = cmd
		return nil
	}}}

	if err := defaultOpen("https://example.com/callback"); err != nil {
		t.Fatalf("default open: %v", err)
	}
	if got.Binary != "/bin/sh" || len(got.Args) != 2 || got.Args[0] != "-lc" || got.Args[1] != "printf bridge-open" {
		t.Fatalf("unexpected open command %#v", got)
	}
	if !containsString(got.Env, "HATCHCTL_BRIDGE_URL=https://example.com/callback") {
		t.Fatalf("expected bridge url env, got %#v", got.Env)
	}
}

func TestDefaultOpenUsesOpenBinaryByDefault(t *testing.T) {
	original := defaultBridgeRuntimeDeps
	t.Cleanup(func() { defaultBridgeRuntimeDeps = original })
	t.Setenv("HATCHCTL_BRIDGE_OPEN_COMMAND", "")

	var got command.Command
	defaultBridgeRuntimeDeps = bridgeRuntimeDeps{hostCommand: fakeBridgeRunner{runFunc: func(_ context.Context, cmd command.Command) error {
		got = cmd
		return nil
	}}}

	if err := defaultOpen("https://example.com/default"); err != nil {
		t.Fatalf("default open: %v", err)
	}
	if got.Binary != "open" || len(got.Args) != 1 || got.Args[0] != "https://example.com/default" {
		t.Fatalf("unexpected open command %#v", got)
	}
}

func TestBridgeHostServiceRewritesEmbeddedLocalhostCallbackURL(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	var opened string
	service := newBridgeHostService(session, "container", func(target string) error {
		opened = target
		return nil
	})
	service.forwardURL = func(port int) (int, bool, error) {
		if port != 40443 {
			t.Fatalf("unexpected container port %d", port)
		}
		return 19090, false, nil
	}
	client, server := net.Pipe()
	defer client.Close()
	go service.handleConn(server)
	requestURL := "https://github.com/login/oauth/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%3A40443%2Fcallback&state=test"
	if err := writeBridgeRequest(client, bridgeRequest{Kind: "open", URL: requestURL}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	response, err := readBridgeResponse(client)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !response.OK {
		t.Fatalf("unexpected response %#v", response)
	}
	if opened != "https://github.com/login/oauth/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%3A19090%2Fcallback&state=test" {
		t.Fatalf("unexpected rewritten embedded callback url %q", opened)
	}
}

func TestBridgeHostServiceHandlesAuthenticatedStopRequest(t *testing.T) {
	t.Parallel()

	stopped := make(chan struct{}, 1)
	session := &Session{ID: "session", Token: "secret", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	service := newBridgeHostService(session, "container", func(string) error { return nil })
	service.stop = func() error {
		stopped <- struct{}{}
		return nil
	}
	client, server := net.Pipe()
	defer client.Close()
	go service.handleConn(server)

	if err := writeBridgeRequest(client, bridgeRequest{Kind: "stop", Token: "secret"}); err != nil {
		t.Fatalf("write stop request: %v", err)
	}
	response, err := readBridgeResponse(client)
	if err != nil {
		t.Fatalf("read stop response: %v", err)
	}
	if !response.OK {
		t.Fatalf("unexpected stop response %#v", response)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("expected stop callback to run")
	}
}

func TestHelperConnectCopiesTraffic(t *testing.T) {
	t.Parallel()

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

func TestCopyStreamsReturnsWriteErrors(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	errCh := make(chan error, 1)
	go func() {
		errCh <- copyStreams(client, bytes.NewBuffer(nil), failingWriter{err: errors.New("write failed")})
	}()
	if _, err := server.Write([]byte("hello")); err != nil {
		t.Fatalf("write server payload: %v", err)
	}
	_ = server.Close()
	if err := <-errCh; err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected stream write error, got %v", err)
	}
}

func TestBridgeHostServiceConnectsToContainerPort(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	service := newBridgeHostService(session, "container", func(string) error { return nil })
	service.connectPort = func(_ context.Context, containerID string, port int, stdin io.Reader, stdout io.Writer) error {
		if containerID != "container" {
			t.Fatalf("unexpected container id %q", containerID)
		}
		if port != 8080 {
			t.Fatalf("unexpected port %d", port)
		}
		data := make([]byte, len("hello"))
		_, err := io.ReadFull(stdin, data)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append([]byte("reply:"), data...))
		return err
	}
	client, server := net.Pipe()
	defer client.Close()
	go service.handleForwardConn(8080, server)
	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatalf("write data: %v", err)
	}
	data, err := io.ReadAll(client)
	if err != nil {
		t.Fatalf("read proxied data: %v", err)
	}
	if got := string(data); got != "reply:hello" {
		t.Fatalf("unexpected proxied data %q", got)
	}
	status, err := os.ReadFile(session.StatusPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read status file: %v", err)
	}
	if len(status) != 0 {
		t.Fatalf("expected no status update on successful forward, got %s", string(status))
	}
}

func TestBridgeHostServiceReportsForwardError(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	service := newBridgeHostService(session, "container", func(string) error { return nil })
	service.connectPort = func(context.Context, string, int, io.Reader, io.Writer) error {
		return fmt.Errorf("connect failed")
	}
	client, server := net.Pipe()
	go service.handleForwardConn(8080, server)
	_ = client.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(session.StatusPath)
		if err == nil {
			if !strings.Contains(string(data), `"lastEvent": "forward failed"`) || !strings.Contains(string(data), `"lastError": "connect failed"`) {
				t.Fatalf("unexpected status file %s", string(data))
			}
			break
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read status file: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("status file was not written")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBridgeHostServiceForwardsSingleUseExactPortWhenAvailable(t *testing.T) {
	t.Parallel()

	session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	service := newBridgeHostService(session, "container", func(string) error { return nil })
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	forwardPort := reserved.Addr().(*net.TCPAddr).Port
	if err := reserved.Close(); err != nil {
		t.Fatal(err)
	}
	service.connectPort = func(_ context.Context, _ string, port int, stdin io.Reader, stdout io.Writer) error {
		if port != forwardPort {
			t.Fatalf("unexpected port %d", port)
		}
		payload, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append([]byte("reply:"), payload...))
		return err
	}

	hostPort, exact, err := service.ensureForward(forwardPort)
	if err != nil {
		t.Fatalf("ensure forward: %v", err)
	}
	if !exact {
		t.Fatal("expected exact bridge host port")
	}
	if hostPort != forwardPort {
		t.Fatalf("expected exact host port %d, got %d", forwardPort, hostPort)
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)))
	if err != nil {
		t.Fatalf("dial forward: %v", err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		_ = conn.Close()
		t.Fatalf("write forward payload: %v", err)
	}
	_ = conn.(*net.TCPConn).CloseWrite()
	data, err := io.ReadAll(conn)
	_ = conn.Close()
	if err != nil {
		t.Fatalf("read forward payload: %v", err)
	}
	if got := string(data); got != "reply:hello" {
		t.Fatalf("unexpected forwarded response %q", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		service.mu.Lock()
		_, ok := service.forwarded[forwardPort]
		service.mu.Unlock()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected single-use forward to be released")
		}
		time.Sleep(10 * time.Millisecond)
	}

	deadline = time.Now().Add(2 * time.Second)
	for {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)))
		if err == nil {
			_ = listener.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected single-use forward listener to be closed after first connection: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		status, err := os.ReadFile(session.StatusPath)
		if err == nil && strings.Contains(string(status), `"lastEvent": "forward closed"`) {
			break
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read status file: %v", err)
		}
		if time.Now().After(deadline) {
			if err == nil {
				t.Fatalf("unexpected status file %s", string(status))
			}
			t.Fatalf("expected forward close status to be written")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHelperOpenUsesTCPBridge(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	requests := make(chan bridgeRequest, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var req bridgeRequest
		if err := readBridgeRequest(conn, &req); err == nil {
			requests <- req
			_ = writeBridgeResponse(conn, bridgeResponse{OK: true})
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := helperOpen([]string{"--host", "127.0.0.1", "--port", fmt.Sprintf("%d", port), "--token", "secret", "--url", "https://example.com/tcp"}); err != nil {
		t.Fatalf("helper open over tcp: %v", err)
	}
	req := <-requests
	if req.Kind != "open" || req.URL != "https://example.com/tcp" || req.Token != "secret" {
		t.Fatalf("unexpected request %#v", req)
	}
}

func TestHelperOpenRequiresHostAndPort(t *testing.T) {
	t.Parallel()
	err := helperOpen([]string{"--url", "https://example.com"})
	if err == nil || err.Error() != "open requires --host and --port" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestListenBridgeTCPBindsLoopbackOnly(t *testing.T) {
	t.Parallel()

	listener, err := listenBridgeTCP(0)
	if err != nil {
		t.Fatalf("listen bridge tcp: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr %#v", listener.Addr())
	}
	if got := addr.IP.String(); got != "127.0.0.1" {
		t.Fatalf("expected loopback listener, got %q", got)
	}
}

func TestHelperBinaryDataUsesConfiguredPath(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "hatchctl")
	if err := os.WriteFile(helperPath, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperBinaryEnvVar, helperPath)

	got, err := helperBinaryData("amd64")
	if err != nil {
		t.Fatalf("helper binary data: %v", err)
	}
	if string(got) != "helper" {
		t.Fatalf("unexpected helper data %q", string(got))
	}
}

func TestHelperBinaryDataUsesEmbeddedHelper(t *testing.T) {
	t.Setenv(helperBinaryEnvVar, "")
	original := embeddedHelpers
	embeddedHelpers = map[string][]byte{"amd64": []byte("embedded-helper")}
	t.Cleanup(func() { embeddedHelpers = original })

	got, err := helperBinaryData("amd64")
	if err != nil {
		t.Fatalf("helper binary data: %v", err)
	}
	if string(got) != "embedded-helper" {
		t.Fatalf("unexpected helper data %q", string(got))
	}
}

func TestHelperBinaryDataFailsWithoutEmbeddedHelperOrOverride(t *testing.T) {
	t.Setenv(helperBinaryEnvVar, "")
	original := embeddedHelpers
	embeddedHelpers = map[string][]byte{}
	t.Cleanup(func() { embeddedHelpers = original })

	_, err := helperBinaryData("amd64")
	if err == nil {
		t.Fatal("expected helper binary lookup to fail")
	}
	if got := err.Error(); got != "bridge helper arch \"amd64\" not embedded in this build; supported=[]; use a release binary or set HATCHCTL_BRIDGE_HELPER" {
		t.Fatalf("unexpected error %q", got)
	}
}

func TestPrepareAndRuntimeFilesUseOwnerOnlyPermissions(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "hatchctl")
	if err := os.WriteFile(helperPath, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperBinaryEnvVar, helperPath)

	stateDir := t.TempDir()
	session, err := Prepare(stateDir, true, "amd64")
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
	assertMode(filepath.Join(session.StatePath, "bin"), 0o755)
	assertMode(filepath.Join(session.StatePath, "session.json"), 0o600)
	assertMode(session.ConfigPath, 0o600)
	assertMode(session.StatusPath, 0o600)
	assertMode(session.HelperPath, 0o755)
	assertMode(filepath.Join(session.StatePath, "bin", "xdg-open"), 0o755)
	assertMode(filepath.Join(session.StatePath, "bin", "hatchctl"), 0o755)

	if err := os.WriteFile(session.PIDPath, []byte("123"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	assertMode(session.PIDPath, 0o600)
	if session.Host == "" || session.Port <= 0 || session.Token == "" {
		t.Fatalf("expected socket paths in session %#v", session)
	}
}

func TestStartUpdatesSessionStatusInTestProcess(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "hatchctl")
	if err := os.WriteFile(helperPath, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperBinaryEnvVar, helperPath)

	stateDir := t.TempDir()
	session, err := Prepare(stateDir, true, "amd64")
	if err != nil {
		t.Fatalf("prepare bridge session: %v", err)
	}

	started, err := Start(session, "container-123")
	if err != nil {
		t.Fatalf("start bridge session: %v", err)
	}
	if started == nil {
		t.Fatal("expected started session")
	}
	if runtime.GOOS != "darwin" {
		if started.Status != "scaffolded" {
			t.Fatalf("expected non-darwin start to remain scaffolded, got %#v", started)
		}
		return
	}
	if started.Status != "running" {
		t.Fatalf("expected started session to be running, got %#v", started)
	}
	data, err := os.ReadFile(started.StatusPath)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	if !strings.Contains(string(data), `"lastEvent": "running"`) {
		t.Fatalf("unexpected status file %s", string(data))
	}
}

func TestServeRespondsToPingAndStopsOnCancel(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "hatchctl")
	if err := os.WriteFile(helperPath, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperBinaryEnvVar, helperPath)

	stateDir := t.TempDir()
	session, err := Prepare(stateDir, true, "amd64")
	if err != nil {
		t.Fatalf("prepare bridge session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, stateDir, "container-123")
	}()

	if err := waitForBridgeTCP(session.Port, 2*time.Second); err != nil {
		cancel()
		t.Fatalf("wait for bridge tcp: %v", err)
	}
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(session.Port)))
	if err != nil {
		cancel()
		t.Fatalf("dial bridge tcp: %v", err)
	}
	if err := writeBridgeRequest(conn, bridgeRequest{Kind: "ping"}); err != nil {
		_ = conn.Close()
		cancel()
		t.Fatalf("write ping request: %v", err)
	}
	response, err := readBridgeResponse(conn)
	_ = conn.Close()
	if err != nil {
		cancel()
		t.Fatalf("read ping response: %v", err)
	}
	if !response.OK {
		cancel()
		t.Fatalf("unexpected ping response %#v", response)
	}

	data, err := os.ReadFile(session.StatusPath)
	if err != nil {
		cancel()
		t.Fatalf("read status file: %v", err)
	}
	if !strings.Contains(string(data), `"lastEvent": "running"`) {
		cancel()
		t.Fatalf("unexpected status file %s", string(data))
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not stop after context cancellation")
	}
}

func TestBridgeHostServiceRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "invalid json", payload: "not-json\n", want: "invalid request"},
		{name: "unknown kind", payload: `{"kind":"mystery"}` + "\n", want: "unknown request"},
		{name: "missing url", payload: `{"kind":"open"}` + "\n", want: "missing url"},
		{name: "invalid url", payload: `{"kind":"open","url":"://bad"}` + "\n", want: "invalid url"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
			service := newBridgeHostService(session, "container", func(string) error { return nil })
			client, server := net.Pipe()
			defer client.Close()
			go service.handleConn(server)
			if _, err := io.WriteString(client, tt.payload); err != nil {
				t.Fatalf("write payload: %v", err)
			}
			response, err := readBridgeResponse(client)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			if response.Error != tt.want {
				t.Fatalf("unexpected response %#v", response)
			}
		})
	}
}

func TestBridgeHostServiceReturnsOpenError(t *testing.T) {
	t.Parallel()

	session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	service := newBridgeHostService(session, "container", func(string) error {
		return fmt.Errorf("open failed")
	})
	service.forwardURL = func(port int) (int, bool, error) {
		return port, true, nil
	}
	client, server := net.Pipe()
	defer client.Close()
	go service.handleConn(server)
	if err := writeBridgeRequest(client, bridgeRequest{Kind: "open", URL: "http://localhost:8080"}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	response, err := readBridgeResponse(client)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.Error != "open failed" {
		t.Fatalf("unexpected response %#v", response)
	}
	data, err := os.ReadFile(session.StatusPath)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	if !strings.Contains(string(data), `"lastEvent": "open failed"`) {
		t.Fatalf("unexpected status file %s", string(data))
	}
}

func TestRewriteLocalURLDefaultsPortsByScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		port int
		want string
	}{
		{name: "http", url: "http://localhost/path", port: 80, want: "http://localhost:19090/path"},
		{name: "https", url: "https://127.0.0.1/cb", port: 443, want: "https://127.0.0.1:19090/cb"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service := &bridgeHostService{}
			service.forwardURL = func(port int) (int, bool, error) {
				if port != tt.port {
					t.Fatalf("unexpected port %d", port)
				}
				return 19090, false, nil
			}
			rewritten, err := service.rewriteLocalURL(tt.url)
			if err != nil {
				t.Fatalf("rewrite url: %v", err)
			}
			if rewritten != tt.want {
				t.Fatalf("unexpected rewritten url %q", rewritten)
			}
		})
	}
}

func TestRewriteLocalURLRewritesEmbeddedLocalhostQueryValues(t *testing.T) {
	t.Parallel()

	service := &bridgeHostService{}
	service.forwardURL = func(port int) (int, bool, error) {
		if port != 40443 {
			t.Fatalf("unexpected port %d", port)
		}
		return 19090, false, nil
	}

	rewritten, err := service.rewriteLocalURL("https://github.com/login/oauth/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A40443%2Fcallback")
	if err != nil {
		t.Fatalf("rewrite url: %v", err)
	}
	if rewritten != "https://github.com/login/oauth/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A19090%2Fcallback" {
		t.Fatalf("unexpected rewritten url %q", rewritten)
	}
}

func TestRewriteLocalURLPreservesLoopbackHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "localhost", url: "http://localhost:8080/callback", want: "http://localhost:19090/callback"},
		{name: "ipv4", url: "http://127.0.0.1:8080/callback", want: "http://127.0.0.1:19090/callback"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := &bridgeHostService{}
			service.forwardURL = func(port int) (int, bool, error) {
				if port != 8080 {
					t.Fatalf("unexpected port %d", port)
				}
				return 19090, false, nil
			}

			rewritten, err := service.rewriteLocalURL(tt.url)
			if err != nil {
				t.Fatalf("rewrite url: %v", err)
			}
			if rewritten != tt.want {
				t.Fatalf("unexpected rewritten url %q", rewritten)
			}
		})
	}
}

func TestRewriteLocalURLRejectsInvalidPorts(t *testing.T) {
	t.Parallel()

	service := &bridgeHostService{}
	_, err := service.rewriteLocalURL("http://localhost:0")
	if err == nil || err.Error() != `invalid localhost port "0"` {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestListenForwardPortFallsBackWhenExactPortBusy(t *testing.T) {
	t.Parallel()

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port

	listener, exact, err := listenForwardPort(port)
	if err != nil {
		t.Fatalf("listen forward port: %v", err)
	}
	defer listener.Close()
	if exact {
		t.Fatal("expected randomized bridge host port")
	}
	if got := listener.Addr().(*net.TCPAddr).Port; got == port {
		t.Fatalf("expected randomized host port, got exact port %d", got)
	}
}

func TestWaitForBridgeTCPTimesOut(t *testing.T) {
	t.Parallel()

	err := waitForBridgeTCP(1, 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for bridge tcp listener") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestStopExistingTerminatesPIDFromFile(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	pidPath := filepath.Join(t.TempDir(), "bridge.pid")
	statusPath := filepath.Join(t.TempDir(), "bridge-status.json")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	if err := os.WriteFile(statusPath, []byte(fmt.Sprintf(`{"sessionId":"session","pid":%d}`, cmd.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write status file: %v", err)
	}

	if err := stopExisting(&Session{ID: "session", PIDPath: pidPath, StatusPath: statusPath}); err != nil {
		t.Fatalf("stop existing process: %v", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	select {
	case err := <-waitCh:
		if err != nil && !strings.Contains(err.Error(), "terminated") && !strings.Contains(err.Error(), "no child processes") {
			t.Fatalf("wait for stopped process: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected pid %d to stop after SIGTERM", cmd.Process.Pid)
	}
	cmd.Process = nil
}

func TestStopExistingRefusesUnexpectedPIDWithoutBridgeStatus(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	pidPath := filepath.Join(t.TempDir(), "bridge.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	err := stopExisting(&Session{ID: "session", PIDPath: pidPath})
	if err == nil || !strings.Contains(err.Error(), "refusing to signal unexpected bridge process") {
		t.Fatalf("expected refusal for unexpected process, got %v", err)
	}
	if !isPIDRunning(cmd.Process.Pid) {
		t.Fatalf("expected pid %d to still be running", cmd.Process.Pid)
	}
}

func TestStopExistingIgnoresCurrentProcessAndMissingPIDFile(t *testing.T) {
	t.Parallel()

	missing := &Session{PIDPath: filepath.Join(t.TempDir(), "missing.pid")}
	if err := stopExisting(missing); err != nil {
		t.Fatalf("stop existing with missing pid file: %v", err)
	}

	pidPath := filepath.Join(t.TempDir(), "bridge.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatalf("write current pid file: %v", err)
	}
	if err := stopExisting(&Session{PIDPath: pidPath}); err != nil {
		t.Fatalf("stop existing current pid: %v", err)
	}
}

func TestStopReturnsNilInTestBinary(t *testing.T) {
	t.Parallel()

	if err := Stop(t.TempDir()); err != nil {
		t.Fatalf("stop in test process: %v", err)
	}
}

func TestIsPIDRunningAndClosedListenerHelpers(t *testing.T) {
	t.Parallel()

	if !isPIDRunning(os.Getpid()) {
		t.Fatal("expected current pid to be running")
	}
	if isPIDRunning(0) {
		t.Fatal("expected pid 0 not to be running")
	}
	if !isClosedListener(net.ErrClosed) {
		t.Fatal("expected net.ErrClosed to be recognized")
	}
	if !isClosedListener(errors.New("use of closed network connection")) {
		t.Fatal("expected closed network connection error to be recognized")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func readBridgeRequest(r io.Reader, request *bridgeRequest) error {
	return json.NewDecoder(r).Decode(request)
}
