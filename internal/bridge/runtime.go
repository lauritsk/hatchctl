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
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lauritsk/hatchctl/internal/command"
	"github.com/lauritsk/hatchctl/internal/docker"
)

const containerHelperBin = "/var/run/hatchctl/bridge/bin/hatchctl"

const bridgeStartupTimeout = 5 * time.Second

type containerConnectRunner func(string, int, io.Reader, io.Writer) error

var runContainerConnect containerConnectRunner = defaultContainerConnectRunner

var (
	hostCommandRunner command.Runner = command.Local{}
	dockerCLI                        = docker.NewClient("docker")
)

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
	Kind  string `json:"kind"`
	URL   string `json:"url,omitempty"`
	Port  int    `json:"port,omitempty"`
	Token string `json:"token,omitempty"`
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
	connectPort containerConnectRunner
	mu          sync.Mutex
	forwarded   map[int]forwardedPort
}

func Start(session *Session, containerID string) (*Session, error) {
	if session == nil || !session.Enabled {
		return nil, nil
	}
	stateDir := filepath.Dir(session.StatePath)
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
	proc, err := hostCommandRunner.Start(command.StartOptions{
		Command: command.Command{
			Binary: exe,
			Args:   []string{"bridge", "serve", "--state-dir", stateDir, "--container-id", containerID},
			Env:    os.Environ(),
		},
		SysProcAttr: &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		return nil, err
	}
	_ = proc.Release()
	if err := waitForBridgeTCP(session.Port, bridgeStartupTimeout); err != nil {
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
	tcpListener, err := listenBridgeTCP(session.Port)
	if err != nil {
		return err
	}
	defer func() {
		_ = tcpListener.Close()
	}()
	if err := fileStore.WritePID(session.PIDPath, os.Getpid()); err != nil {
		return err
	}
	if err := writeStatus(session, containerID, "running", "", nil, 0, false); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = tcpListener.Close()
	}()
	service := newBridgeHostService(session, containerID, defaultOpen)
	return service.serveListener(ctx, tcpListener)
}

func (s *bridgeHostService) serveListener(ctx context.Context, listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || isClosedListener(err) {
				return nil
			}
			_ = writeStatus(s.session, s.containerID, "error", err.Error(), nil, 0, false)
			return err
		}
		go s.handleConn(conn)
	}
}

func newBridgeHostService(session *Session, containerID string, opener func(string) error) *bridgeHostService {
	service := &bridgeHostService{
		session:     session,
		containerID: containerID,
		openURL:     opener,
		connectPort: runContainerConnect,
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
		if s.session.Token != "" && request.Token != s.session.Token {
			_ = writeBridgeResponse(conn, bridgeResponse{Error: "unauthorized"})
			return
		}
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
	rewritten, changed, err := s.rewriteParsedLocalURL(parsed)
	if err != nil {
		return "", err
	}
	if changed {
		return rewritten.String(), nil
	}

	query := parsed.Query()
	queryChanged := false
	for key, values := range query {
		updated := make([]string, len(values))
		for i, value := range values {
			rewrittenValue, changed, err := s.rewriteNestedLocalURL(value)
			if err != nil {
				return "", err
			}
			updated[i] = rewrittenValue
			queryChanged = queryChanged || changed
		}
		query[key] = updated
	}
	if !queryChanged {
		return raw, nil
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (s *bridgeHostService) rewriteNestedLocalURL(raw string) (string, bool, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw, false, nil
	}
	rewritten, changed, err := s.rewriteParsedLocalURL(parsed)
	if err != nil {
		return "", false, err
	}
	if !changed {
		return raw, false, nil
	}
	return rewritten.String(), true, nil
}

func (s *bridgeHostService) rewriteParsedLocalURL(parsed *url.URL) (*url.URL, bool, error) {
	host := parsed.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return parsed, false, nil
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
		return nil, false, fmt.Errorf("invalid localhost port %q", portText)
	}
	hostPort, _, err := s.forwardURL(port)
	if err != nil {
		return nil, false, err
	}
	rewritten := *parsed
	rewritten.Host = net.JoinHostPort(host, strconv.Itoa(hostPort))
	return &rewritten, true, nil
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
	if err := s.connectPort(s.containerID, port, conn, conn); err != nil {
		_ = writeStatus(s.session, s.containerID, "forward failed", err.Error(), s.forwardSnapshot(), port, false)
		return
	}
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
	if openCommand := os.Getenv("HATCHCTL_BRIDGE_OPEN_COMMAND"); openCommand != "" {
		return hostCommandRunner.Run(context.Background(), command.Command{Binary: "/bin/sh", Args: []string{"-lc", openCommand}, Env: command.AppendEnv(os.Environ(), "HATCHCTL_BRIDGE_URL="+target)})
	}
	return hostCommandRunner.Run(context.Background(), command.Command{Binary: "open", Args: []string{target}})
}

func stopExisting(session *Session) error {
	pid, err := fileStore.ReadPID(session.PIDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if pid <= 0 {
		return nil
	}
	if pid == os.Getpid() {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	_ = process.Signal(syscall.SIGTERM)
	for range 20 {
		if !isPIDRunning(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func Stop(stateDir string) error {
	exe, err := os.Executable()
	if err == nil && strings.HasSuffix(exe, ".test") {
		return nil
	}
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

func waitForBridgeTCP(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for bridge tcp listener on port %d", port)
}

func defaultContainerConnectRunner(containerID string, port int, stdin io.Reader, stdout io.Writer) error {
	var stderr strings.Builder
	err := dockerCLI.Run(context.Background(), docker.RunOptions{Args: []string{"exec", "-i", containerID, containerHelperBin, "bridge", "helper", "connect", "--port", strconv.Itoa(port)}, Stdin: stdin, Stdout: stdout, Stderr: &stderr})
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("docker exec bridge helper connect: %s", message)
	}
	return nil
}

func listenBridgeTCP(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

func writeBridgeConfig(session *Session, containerID string) error {
	return fileStore.WriteConfig(session, containerID)
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
	return fileStore.WriteStatus(session, status)
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
