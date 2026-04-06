package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

type Session struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Token      string `json:"token,omitempty"`
	StatePath  string `json:"statePath"`
	ConfigPath string `json:"configPath,omitempty"`
	PIDPath    string `json:"pidPath,omitempty"`
	StatusPath string `json:"statusPath,omitempty"`
	HelperPath string `json:"helperPath"`
	MountPath  string `json:"mountPath"`
	BinPath    string `json:"binPath"`
	Status     string `json:"status"`
}

type Report = Session

const containerBridgeMountPath = "/var/run/hatchctl/bridge"

func Prepare(stateDir string, enabled bool) (*Session, error) {
	bridgeDir := filepath.Join(stateDir, "bridge")
	if err := os.MkdirAll(bridgeDir, 0o755); err != nil {
		return nil, err
	}
	session, err := loadOrCreateSession(bridgeDir, enabled)
	if err != nil {
		return nil, err
	}
	binPath := filepath.Join(bridgeDir, "bin")
	if err := os.MkdirAll(binPath, 0o755); err != nil {
		return nil, err
	}
	helperPath := filepath.Join(binPath, "devcontainer-open")
	if err := os.WriteFile(helperPath, []byte(openShim()), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(binPath, "xdg-open"), []byte(xdgOpenShim()), 0o755); err != nil {
		return nil, err
	}
	session.Enabled = enabled
	if enabled && runtime.GOOS == "darwin" {
		if session.Host == "" {
			session.Host = "host.docker.internal"
		}
		if session.Token == "" {
			session.Token = randomToken(24)
		}
		if session.Port == 0 {
			port, err := findFreePort()
			if err != nil {
				return nil, err
			}
			session.Port = port
		}
	}
	session.StatePath = bridgeDir
	session.HelperPath = helperPath
	session.MountPath = containerBridgeMountPath
	session.BinPath = filepath.ToSlash(filepath.Join(containerBridgeMountPath, "bin"))
	if session.Status == "" {
		session.Status = "disabled"
		if enabled {
			session.Status = "scaffolded"
		}
	}
	if err := saveSession(bridgeDir, session); err != nil {
		return nil, err
	}
	return session, nil
}

func Apply(stateDir string, enabled bool, merged devcontainer.MergedConfig) (*Session, devcontainer.MergedConfig, error) {
	if !enabled {
		return nil, merged, nil
	}
	session, err := Prepare(stateDir, enabled)
	if err != nil {
		return nil, devcontainer.MergedConfig{}, err
	}
	if session == nil {
		return session, merged, nil
	}
	containerEnv := cloneEnv(merged.ContainerEnv)
	containerEnv["BROWSER"] = filepath.ToSlash(filepath.Join(session.BinPath, "devcontainer-open"))
	containerEnv["DEVCONTAINER_BRIDGE_ENABLED"] = "true"
	containerEnv["PATH"] = prependPath(session.BinPath, containerEnv["PATH"])
	if session.Host != "" {
		containerEnv["DEVCONTAINER_BRIDGE_HOST"] = session.Host
	}
	if session.Port != 0 {
		containerEnv["DEVCONTAINER_BRIDGE_PORT"] = fmt.Sprintf("%d", session.Port)
	}
	if session.Token != "" {
		containerEnv["DEVCONTAINER_BRIDGE_TOKEN"] = session.Token
	}

	mount := fmt.Sprintf("type=bind,source=%s,target=%s", session.StatePath, session.MountPath)
	merged.ContainerEnv = containerEnv
	merged.Mounts = appendMount(merged.Mounts, mount)
	return session, merged, nil
}

func Doctor(stateDir string) (Report, error) {
	bridgeDir := filepath.Join(stateDir, "bridge")
	helperPath := filepath.Join(bridgeDir, "bin", "devcontainer-open")
	_, err := os.Stat(helperPath)
	status := "not configured"
	enabled := false
	session, sessionErr := readSession(bridgeDir)
	if sessionErr != nil && !os.IsNotExist(sessionErr) {
		return Report{}, sessionErr
	}
	if err == nil {
		enabled = true
		status = "scaffolded"
	}
	if err != nil && !os.IsNotExist(err) {
		return Report{}, err
	}
	if session != nil && session.Status != "" {
		status = session.Status
	}
	if session != nil && session.StatusPath != "" {
		if data, err := os.ReadFile(session.StatusPath); err == nil {
			var bridgeStatus struct {
				PID       int    `json:"pid"`
				LastEvent string `json:"lastEvent"`
			}
			if json.Unmarshal(data, &bridgeStatus) == nil {
				if bridgeStatus.PID > 0 && processRunning(bridgeStatus.PID) {
					status = "running"
				} else if bridgeStatus.LastEvent != "" {
					status = bridgeStatus.LastEvent
				}
			}
		}
	}
	return Report{
		ID:         valueOrDefault(sessionID(session), devcontainer.ContainerName(stateDir, helperPath)),
		Enabled:    enabled,
		Host:       sessionHost(session),
		Port:       sessionPort(session),
		Token:      sessionToken(session),
		StatePath:  bridgeDir,
		ConfigPath: sessionConfigPath(session),
		PIDPath:    sessionPIDPath(session),
		StatusPath: sessionStatusPath(session),
		HelperPath: helperPath,
		MountPath:  containerBridgeMountPath,
		BinPath:    filepath.ToSlash(filepath.Join(containerBridgeMountPath, "bin")),
		Status:     status,
	}, nil
}

func openShim() string {
	launcher := "open"
	if runtime.GOOS != "darwin" {
		launcher = "xdg-open"
	}
	return fmt.Sprintf(`#!/bin/sh
set -eu

if [ $# -lt 1 ]; then
  exit 1
fi

url="$1"
if [ -n "${DEVCONTAINER_BRIDGE_OPEN_COMMAND:-}" ]; then
  DEVCONTAINER_BRIDGE_URL="$url" exec /bin/sh -lc "$DEVCONTAINER_BRIDGE_OPEN_COMMAND"
fi

exec %s "$url"
`, launcher)
}

func xdgOpenShim() string {
	return "#!/bin/sh\nexec /var/run/hatchctl/bridge/bin/devcontainer-open \"$@\"\n"
}

func loadOrCreateSession(bridgeDir string, enabled bool) (*Session, error) {
	session, err := readSession(bridgeDir)
	if err == nil {
		return session, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	session = &Session{
		ID:         randomToken(12),
		Host:       "host.docker.internal",
		Port:       0,
		Token:      randomToken(24),
		StatePath:  bridgeDir,
		ConfigPath: filepath.Join(bridgeDir, "bridge-config.json"),
		PIDPath:    filepath.Join(bridgeDir, "bridge.pid"),
		StatusPath: filepath.Join(bridgeDir, "bridge-status.json"),
	}
	if enabled && runtime.GOOS == "darwin" {
		port, err := findFreePort()
		if err != nil {
			return nil, err
		}
		session.Port = port
	}
	if err := saveSession(bridgeDir, session); err != nil {
		return nil, err
	}
	return session, nil
}

func readSession(bridgeDir string) (*Session, error) {
	path := filepath.Join(bridgeDir, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func saveSession(bridgeDir string, session *Session) error {
	path := filepath.Join(bridgeDir, "session.json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func findFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", listener.Addr())
	}
	return addr.Port, nil
}

func randomToken(bytesLen int) string {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return devcontainer.ManagedByValue
	}
	return hex.EncodeToString(b)
}

func valueOrDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func sessionID(session *Session) string {
	if session == nil {
		return ""
	}
	return session.ID
}

func sessionHost(session *Session) string {
	if session == nil {
		return ""
	}
	return session.Host
}

func sessionPort(session *Session) int {
	if session == nil {
		return 0
	}
	return session.Port
}

func sessionToken(session *Session) string {
	if session == nil {
		return ""
	}
	return session.Token
}

func sessionConfigPath(session *Session) string {
	if session == nil {
		return ""
	}
	return session.ConfigPath
}

func sessionPIDPath(session *Session) string {
	if session == nil {
		return ""
	}
	return session.PIDPath
}

func sessionStatusPath(session *Session) string {
	if session == nil {
		return ""
	}
	return session.StatusPath
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func cloneEnv(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func prependPath(prefix string, existing string) string {
	if existing == "" {
		return prefix + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	for _, entry := range filepath.SplitList(existing) {
		if entry == prefix {
			return existing
		}
	}
	return prefix + ":" + existing
}

func appendMount(mounts []string, mount string) []string {
	for _, existing := range mounts {
		if existing == mount {
			return mounts
		}
	}
	return append(mounts, mount)
}
