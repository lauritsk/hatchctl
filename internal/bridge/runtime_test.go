package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	if opened != "http://localhost:19090/cb" {
		t.Fatalf("unexpected rewritten url %q", opened)
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

func TestBridgeHostServiceConnectsToContainerPort(t *testing.T) {
	t.Parallel()
	session := &Session{ID: "session", StatusPath: filepath.Join(t.TempDir(), "bridge-status.json")}
	service := newBridgeHostService(session, "container", func(string) error { return nil })
	service.connectPort = func(containerID string, port int, stdin io.Reader, stdout io.Writer) error {
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
	service.connectPort = func(string, int, io.Reader, io.Writer) error {
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
	service.connectPort = func(_ string, port int, stdin io.Reader, stdout io.Writer) error {
		if port != 8080 {
			t.Fatalf("unexpected port %d", port)
		}
		payload, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append([]byte("reply:"), payload...))
		return err
	}

	hostPort, exact, err := service.ensureForward(8080)
	if err != nil {
		t.Fatalf("ensure forward: %v", err)
	}
	if !exact {
		t.Fatal("expected exact bridge host port")
	}
	if hostPort != 8080 {
		t.Fatalf("expected exact host port 8080, got %d", hostPort)
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
		_, ok := service.forwarded[8080]
		service.mu.Unlock()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected single-use forward to be released")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)), 200*time.Millisecond); err == nil {
		t.Fatal("expected single-use forward listener to be closed after first connection")
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

func readBridgeRequest(r io.Reader, request *bridgeRequest) error {
	return json.NewDecoder(r).Decode(request)
}
