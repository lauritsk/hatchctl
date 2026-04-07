package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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

const containerHelperBin = "/var/run/hatchctl/bridge/bin/hatchctl-bridge-helper"

type statusFile struct {
	SessionID   string          `json:"sessionId"`
	ContainerID string          `json:"containerId"`
	ControlPort int             `json:"controlPort"`
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

type bridgeHostService struct {
	session     *Session
	containerID string
	openURL     func(string) error
	forwardURL  func(int) (int, bool, error)
	execArgs    func(string, int) []string
	mu          sync.Mutex
	forwarded   map[int]forwardedPort
}

func Start(stateDir string, enabled bool, containerID string) (*Session, error) {
	if !enabled {
		return nil, nil
	}
	session, err := Prepare(stateDir, enabled)
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
	if err := waitForReady(session.Port, 5*time.Second); err != nil {
		return nil, err
	}
	session.Status = "running"
	return session, nil
}

func Serve(ctx context.Context, stateDir string, containerID string) error {
	session, err := Prepare(stateDir, true)
	if err != nil {
		return err
	}
	if session.Port == 0 {
		return fmt.Errorf("bridge port not configured")
	}
	if err := os.WriteFile(session.PIDPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return err
	}
	if err := writeStatus(session, containerID, "running", "", nil, 0, false); err != nil {
		return err
	}
	service := newBridgeHostService(session, containerID, defaultOpen)
	handler := service.handler()
	server := &http.Server{Addr: bridgeListenAddress(session), Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err = server.ListenAndServe()
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	_ = writeStatus(session, containerID, "error", err.Error(), nil, 0, false)
	return err
}

func newBridgeHostService(session *Session, containerID string, opener func(string) error) *bridgeHostService {
	service := &bridgeHostService{
		session:     session,
		containerID: containerID,
		openURL:     opener,
		forwarded:   map[int]forwardedPort{},
	}
	service.forwardURL = service.ensureForward
	service.execArgs = connectorExecArgs
	return service
}

func (s *bridgeHostService) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/open" {
			http.NotFound(w, r)
			return
		}
		payload := struct {
			URL   string `json:"url"`
			Token string `json:"token"`
		}{}
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			payload.URL = r.Form.Get("url")
			payload.Token = r.Form.Get("token")
		}
		if payload.Token != s.session.Token || payload.URL == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := url.ParseRequestURI(payload.URL); err != nil {
			http.Error(w, "invalid url", http.StatusBadRequest)
			return
		}
		rewritten, err := s.rewriteLocalURL(payload.URL)
		if err != nil {
			_ = writeStatus(s.session, s.containerID, "rewrite failed", err.Error(), nil, 0, false)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.openURL(rewritten); err != nil {
			_ = writeStatus(s.session, s.containerID, "open failed", err.Error(), s.forwardSnapshot(), 0, false)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = writeStatus(s.session, s.containerID, "open forwarded", "", s.forwardSnapshot(), 0, false)
		w.WriteHeader(http.StatusNoContent)
	})
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
	cmd := exec.Command("docker", s.execArgs(s.containerID, port)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(stdin, conn)
		_ = stdin.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, stdout)
		done <- struct{}{}
	}()
	<-done
	<-done
	_ = cmd.Wait()
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

func connectorExecArgs(containerID string, port int) []string {
	return []string{"exec", "-i", containerID, containerHelperBin, "connect", "--port", strconv.Itoa(port)}
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
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func Stop(stateDir string) error {
	session, err := Prepare(stateDir, true)
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

func waitForReady(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for bridge on port %d", port)
}

func writeBridgeConfig(session *Session, containerID string) error {
	config := map[string]any{
		"sessionId":   session.ID,
		"containerId": containerID,
		"controlPort": session.Port,
		"token":       session.Token,
		"host":        session.Host,
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
		ControlPort: session.Port,
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

func bridgeListenAddress(session *Session) string {
	port := 0
	host := "0.0.0.0"
	if session != nil {
		port = session.Port
		if session.Host == "127.0.0.1" || session.Host == "localhost" {
			host = "127.0.0.1"
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
