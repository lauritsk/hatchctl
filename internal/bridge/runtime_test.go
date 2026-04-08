package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

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

func TestHelperServeConnectsToContainerLocalhost(t *testing.T) {
	t.Parallel()
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpListener.Close()
	go func() {
		conn, err := tcpListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		data, _ := io.ReadAll(conn)
		_, _ = conn.Write(append([]byte("reply:"), data...))
	}()

	unixPath := testSocketPath(t, "helper.sock")
	unixListener, err := listenUnixSocket(unixPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = unixListener.Close()
		_ = os.Remove(unixPath)
	}()
	go func() {
		conn, err := unixListener.Accept()
		if err != nil {
			return
		}
		handleHelperConn(conn)
	}()

	conn, err := net.Dial("unix", unixPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	port := tcpListener.Addr().(*net.TCPAddr).Port
	if err := writeBridgeRequest(conn, bridgeRequest{Kind: "connect", Port: port}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	response, err := readBridgeResponse(conn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !response.OK {
		t.Fatalf("unexpected response %#v", response)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write data: %v", err)
	}
	closeWrite(conn)
	data, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read proxied data: %v", err)
	}
	if got := string(data); got != "reply:hello" {
		t.Fatalf("unexpected proxied data %q", got)
	}
}

func TestHelperOpenUsesBridgeSocket(t *testing.T) {
	t.Parallel()
	path := testSocketPath(t, "bridge.sock")
	listener, err := listenUnixSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(path)
	}()
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
	if err := helperOpen([]string{"--socket", path, "--url", "https://example.com"}); err != nil {
		t.Fatalf("helper open: %v", err)
	}
	req := <-requests
	if req.Kind != "open" || req.URL != "https://example.com" {
		t.Fatalf("unexpected request %#v", req)
	}
}

func TestHelperBinaryDataUsesConfiguredPath(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "hatchctl")
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

func TestPrepareAndRuntimeFilesUseOwnerOnlyPermissions(t *testing.T) {
	helperPath := filepath.Join(t.TempDir(), "hatchctl")
	if err := os.WriteFile(helperPath, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperBinaryEnvVar, helperPath)

	stateDir := t.TempDir()
	session, err := Prepare(stateDir, true)
	if err != nil {
		t.Fatalf("prepare bridge session: %v", err)
	}
	session.SocketPath = testSocketPath(t, "bridge.sock")
	session.HelperSock = testSocketPath(t, "helper.sock")
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

	listener, err := listenUnixSocket(session.SocketPath)
	if err != nil {
		t.Fatalf("listen bridge socket: %v", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(session.SocketPath)
	}()
	assertMode(session.SocketPath, 0o600)

	if err := os.WriteFile(session.PIDPath, []byte("123"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	assertMode(session.PIDPath, 0o600)
	if session.SocketPath == "" || session.HelperSock == "" {
		t.Fatalf("expected socket paths in session %#v", session)
	}
}

func readBridgeRequest(r io.Reader, request *bridgeRequest) error {
	return json.NewDecoder(r).Decode(request)
}

func testSocketPath(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "hct-bridge-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, name)
}
