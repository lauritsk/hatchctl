package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const containerHelperBin = "/var/run/hatchctl/bridge/bin/hatchctl"

type statusFile struct {
	SessionID   string          `json:"sessionId"`
	ContainerID string          `json:"containerId"`
	PID         int             `json:"pid,omitempty"`
	Forwarded   []forwardStatus `json:"forwarded,omitempty"`
	LastPort    int             `json:"lastForwardedPort,omitempty"`
	LastExact   bool            `json:"lastExactPort,omitempty"`
	LastEvent   string          `json:"lastEvent,omitempty"`
	LastError   string          `json:"lastError,omitempty"`
	UpdatedAt   string          `json:"updatedAt"`
}

type forwardStatus struct {
	ContainerPort int  `json:"containerPort"`
	HostPort      int  `json:"hostPort"`
	ExactPort     bool `json:"exactPort"`
}

type forwardedPort struct {
	server   net.Listener
	hostPort int
	exact    bool
}

type bridgeRequest struct {
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
	Port int    `json:"port,omitempty"`
}

type bridgeResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type bridgeHostService struct {
	session     *Session
	containerID string
	openURL     func(string) error
	forwardURL  func(int) (int, bool, error)
	mu          sync.Mutex
	forwarded   map[int]forwardedPort
}

func Start(stateDir string, enabled bool, helperArch string, containerID string) (*Session, error) {
	if !enabled {
		return nil, nil
	}
	session, err := Prepare(stateDir, enabled, helperArch)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "darwin" {
		session.Status = "scaffolded"
		return session, nil
	}
	if err := stopExisting(session); err != nil {
		return nil, err
	}
	if err := writeBridgeConfig(session, containerID); err != nil {
		return nil, err
	}
	if err := writeStatus(session, containerID, "starting", "", nil, 0, false); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(exe, ".test") {
		session.Status = "running"
		if err := writeStatus(session, containerID, "running", "", nil, 0, false); err != nil {
			return nil, err
		}
		return session, nil
	}
	cmd := exec.Command(exe, "bridge", "serve", "--state-dir", stateDir, "--container-id", containerID)
	cmd.Env = os.Environ()
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	_ = cmd.Process.Release()
	if err := waitForSocket(session.SocketPath, 5*time.Second); err != nil {
		return nil, err
	}
	if err := ensureHelperService(session, containerID); err != nil {
		return nil, err
	}
	if err := waitForHelper(session.HelperSock, 5*time.Second); err != nil {
		return nil, err
	}
	session.Status = "running"
	return session, nil
}

func Serve(ctx context.Context, stateDir string, containerID string) error {
	session, err := Prepare(stateDir, true, "")
	if err != nil {
		return err
	}
	listener, err := listenUnixSocket(session.SocketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(session.SocketPath)
	}()
	if err := os.WriteFile(session.PIDPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return err
	}
	if err := writeStatus(session, containerID, "running", "", nil, 0, false); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	service := newBridgeHostService(session, containerID, defaultOpen)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || isClosedListener(err) {
				return nil
			}
			_ = writeStatus(session, containerID, "error", err.Error(), nil, 0, false)
			return err
		}
		go service.handleConn(conn)
	}
}

func newBridgeHostService(session *Session, containerID string, opener func(string) error) *bridgeHostService {
	service := &bridgeHostService{
		session:     session,
		containerID: containerID,
		openURL:     opener,
		forwarded:   map[int]forwardedPort{},
	}
	service.forwardURL = service.ensureForward
	return service
}

func (s *bridgeHostService) handleConn(conn net.Conn) {
	defer conn.Close()
	var request bridgeRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		_ = writeBridgeResponse(conn, bridgeResponse{Error: "invalid request"})
		return
	}
	switch request.Kind {
	case "ping":
		_ = writeBridgeResponse(conn, bridgeResponse{OK: true})
	case "open":
		if request.URL == "" {
			_ = writeBridgeResponse(conn, bridgeResponse{Error: "missing url"})
			return
		}
		if _, err := url.ParseRequestURI(request.URL); err != nil {
			_ = writeBridgeResponse(conn, bridgeResponse{Error: "invalid url"})
			return
		}
		rewritten, err := s.rewriteLocalURL(request.URL)
		if err != nil {
			_ = writeStatus(s.session, s.containerID, "rewrite failed", err.Error(), nil, 0, false)
			_ = writeBridgeResponse(conn, bridgeResponse{Error: err.Error()})
			return
		}
		if err := s.openURL(rewritten); err != nil {
			_ = writeStatus(s.session, s.containerID, "open failed", err.Error(), s.forwardSnapshot(), 0, false)
			_ = writeBridgeResponse(conn, bridgeResponse{Error: err.Error()})
			return
		}
		_ = writeStatus(s.session, s.containerID, "open forwarded", "", s.forwardSnapshot(), 0, false)
		_ = writeBridgeResponse(conn, bridgeResponse{OK: true})
	default:
		_ = writeBridgeResponse(conn, bridgeResponse{Error: "unknown request"})
	}
}

func (s *bridgeHostService) rewriteLocalURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw, nil
	}
	if parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return raw, nil
	}
	portText := parsed.Port()
	if portText == "" {
		if parsed.Scheme == "https" {
			portText = "443"
		} else {
			portText = "80"
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return "", fmt.Errorf("invalid localhost port %q", portText)
	}
	hostPort, _, err := s.forwardURL(port)
	if err != nil {
		return "", err
	}
	parsed.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort))
	return parsed.String(), nil
}

func (s *bridgeHostService) ensureForward(port int) (int, bool, error) {
	s.mu.Lock()
	if forward, ok := s.forwarded[port]; ok {
		s.mu.Unlock()
		return forward.hostPort, forward.exact, nil
	}
	s.mu.Unlock()

	listener, exact, err := listenForwardPort(port)
	if err != nil {
		return 0, false, err
	}
	hostPort := listener.Addr().(*net.TCPAddr).Port
	forward := forwardedPort{server: listener, hostPort: hostPort, exact: exact}

	s.mu.Lock()
	s.forwarded[port] = forward
	snapshot := s.forwardSnapshotLocked()
	s.mu.Unlock()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.handleForwardConn(port, conn)
		}
	}()

	_ = writeStatus(s.session, s.containerID, forwardEvent(port, hostPort), "", snapshot, port, exact)
	return hostPort, exact, nil
}

func (s *bridgeHostService) handleForwardConn(port int, conn net.Conn) {
	defer conn.Close()
	helperConn, err := net.Dial("unix", s.session.HelperSock)
	if err != nil {
		return
	}
	defer helperConn.Close()
	if err := writeBridgeRequest(helperConn, bridgeRequest{Kind: "connect", Port: port}); err != nil {
		return
	}
	response, err := readBridgeResponse(helperConn)
	if err != nil || !response.OK {
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(helperConn, conn)
		closeWrite(helperConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, helperConn)
		closeWrite(conn)
		done <- struct{}{}
	}()
	<-done
	<-done
}

func (s *bridgeHostService) forwardSnapshot() []forwardStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forwardSnapshotLocked()
}

func (s *bridgeHostService) forwardSnapshotLocked() []forwardStatus {
	result := make([]forwardStatus, 0, len(s.forwarded))
	for containerPort, forward := range s.forwarded {
		result = append(result, forwardStatus{ContainerPort: containerPort, HostPort: forward.hostPort, ExactPort: forward.exact})
	}
	sort.Slice(result, func(i int, j int) bool {
		return result[i].ContainerPort < result[j].ContainerPort
	})
	return result
}

func listenForwardPort(port int) (net.Listener, bool, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err == nil {
		return listener, true, nil
	}
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, false, err
	}
	return listener, false, nil
}

func forwardEvent(containerPort int, hostPort int) string {
	if containerPort == hostPort {
		return fmt.Sprintf("forwarded %d on exact host port", containerPort)
	}
	return fmt.Sprintf("forwarded %d on host port %d", containerPort, hostPort)
}

func defaultOpen(target string) error {
	if command := os.Getenv("HATCHCTL_BRIDGE_OPEN_COMMAND"); command != "" {
		cmd := exec.Command("/bin/sh", "-lc", command)
		cmd.Env = append(os.Environ(), "HATCHCTL_BRIDGE_URL="+target)
		return cmd.Run()
	}
	return exec.Command("open", target).Run()
}

func stopExisting(session *Session) error {
	data, err := os.ReadFile(session.PIDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	_ = process.Signal(syscall.SIGTERM)
	for range 20 {
		if !isPIDRunning(pid) {
			_ = os.Remove(session.SocketPath)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func Stop(stateDir string) error {
	session, err := Prepare(stateDir, true, "")
	if err != nil {
		return err
	}
	return stopExisting(session)
}

func isPIDRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func ensureHelperService(session *Session, containerID string) error {
	if err := pingSocket(session.HelperSock); err == nil {
		return nil
	}
	_ = os.Remove(session.HelperSock)
	cmd := exec.Command(
		"docker", "exec", "-d", containerID,
		containerHelperBin, "bridge", "helper", "serve",
		"--socket", filepath.ToSlash(filepath.Join(containerBridgeMountPath, helperSocketName)),
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := pingSocket(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for bridge socket %s", path)
}

func waitForHelper(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := pingSocket(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for bridge helper socket %s", path)
}

func pingSocket(path string) error {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := writeBridgeRequest(conn, bridgeRequest{Kind: "ping"}); err != nil {
		return err
	}
	response, err := readBridgeResponse(conn)
	if err != nil {
		return err
	}
	if !response.OK {
		return errors.New(response.Error)
	}
	return nil
}

func listenUnixSocket(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return listener, nil
}

func writeBridgeConfig(session *Session, containerID string) error {
	config := map[string]any{
		"sessionId":   session.ID,
		"containerId": containerID,
		"socketPath":  session.SocketPath,
		"helperSock":  session.HelperSock,
		"statusPath":  session.StatusPath,
		"pidPath":     session.PIDPath,
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(session.ConfigPath, data, 0o600)
}

func writeStatus(session *Session, containerID string, event string, lastError string, forwarded []forwardStatus, lastPort int, lastExact bool) error {
	status := statusFile{
		SessionID:   session.ID,
		ContainerID: containerID,
		PID:         os.Getpid(),
		Forwarded:   forwarded,
		LastPort:    lastPort,
		LastExact:   lastExact,
		LastEvent:   event,
		LastError:   lastError,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(session.StatusPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(session.StatusPath, data, 0o600)
}

func writeBridgeRequest(w io.Writer, request bridgeRequest) error {
	return json.NewEncoder(w).Encode(request)
}

func writeBridgeResponse(w io.Writer, response bridgeResponse) error {
	response.OK = response.Error == ""
	return json.NewEncoder(w).Encode(response)
}

func readBridgeResponse(r io.Reader) (bridgeResponse, error) {
	var response bridgeResponse
	if err := json.NewDecoder(r).Decode(&response); err != nil {
		return bridgeResponse{}, err
	}
	return response, nil
}

func closeWrite(conn net.Conn) {
	type closeWriter interface{ CloseWrite() error }
	if closer, ok := conn.(closeWriter); ok {
		_ = closer.CloseWrite()
	}
}

func isClosedListener(err error) bool {
	return errors.Is(err, net.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
}
