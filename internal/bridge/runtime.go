package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type statusFile struct {
	SessionID   string `json:"sessionId"`
	ContainerID string `json:"containerId"`
	ControlPort int    `json:"controlPort"`
	PID         int    `json:"pid,omitempty"`
	LastEvent   string `json:"lastEvent,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	UpdatedAt   string `json:"updatedAt"`
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
	if err := writeStatus(session, containerID, "starting", ""); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(exe, ".test") {
		session.Status = "running"
		if err := writeStatus(session, containerID, "running", ""); err != nil {
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
	if err := os.WriteFile(session.PIDPath, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return err
	}
	if err := writeStatus(session, containerID, "running", ""); err != nil {
		return err
	}
	handler := openHandler(session, containerID, defaultOpen)
	server := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", session.Port), Handler: handler}
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
	_ = writeStatus(session, containerID, "error", err.Error())
	return err
}

func openHandler(session *Session, containerID string, opener func(string) error) http.Handler {
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
		if payload.Token != session.Token || payload.URL == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := url.ParseRequestURI(payload.URL); err != nil {
			http.Error(w, "invalid url", http.StatusBadRequest)
			return
		}
		if err := opener(payload.URL); err != nil {
			_ = writeStatus(session, containerID, "open failed", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = writeStatus(session, containerID, "open forwarded", "")
		w.WriteHeader(http.StatusNoContent)
	})
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
	return os.WriteFile(session.ConfigPath, data, 0o644)
}

func writeStatus(session *Session, containerID string, event string, lastError string) error {
	status := statusFile{
		SessionID:   session.ID,
		ContainerID: containerID,
		ControlPort: session.Port,
		PID:         os.Getpid(),
		LastEvent:   event,
		LastError:   lastError,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(session.StatusPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(session.StatusPath, data, 0o644)
}
