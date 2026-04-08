package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	SocketPath string `json:"socketPath,omitempty"`
	HelperSock string `json:"helperSock,omitempty"`
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

const (
	containerBridgeMountPath = "/var/run/hatchctl/bridge"
	helperBinaryEnvVar       = "HATCHCTL_BRIDGE_HELPER"
	hostSocketName           = "bridge.sock"
	helperSocketName         = "helper.sock"
)

func Prepare(stateDir string, enabled bool) (*Session, error) {
	bridgeDir := filepath.Join(stateDir, "bridge")
	if err := os.MkdirAll(bridgeDir, 0o700); err != nil {
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
	if err := installHelperBinary(binPath); err != nil {
		return nil, err
	}
	session.Enabled = enabled
	if session.SocketPath == "" {
		session.SocketPath = filepath.Join(bridgeDir, hostSocketName)
	}
	if session.HelperSock == "" {
		session.HelperSock = filepath.Join(bridgeDir, helperSocketName)
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
	return session, applySession(session, merged), nil
}

func Preview(stateDir string, enabled bool, merged devcontainer.MergedConfig) (*Session, devcontainer.MergedConfig, error) {
	if !enabled {
		return nil, merged, nil
	}
	bridgeDir := filepath.Join(stateDir, "bridge")
	session, err := readSession(bridgeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, merged, nil
		}
		return nil, devcontainer.MergedConfig{}, err
	}
	return session, applySession(session, merged), nil
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
	return fmt.Sprintf(`#!/bin/sh
set -eu

if [ $# -lt 1 ]; then
  exit 1
fi

url="$1"
if [ -n "${DEVCONTAINER_BRIDGE_OPEN_COMMAND:-}" ]; then
  DEVCONTAINER_BRIDGE_URL="$url" exec /bin/sh -lc "$DEVCONTAINER_BRIDGE_OPEN_COMMAND"
fi

	exec %s bridge helper open --socket %s --url "$url"
`, containerHelperBin, filepath.ToSlash(filepath.Join(containerBridgeMountPath, hostSocketName)))
}

func xdgOpenShim() string {
	return "#!/bin/sh\nexec /var/run/hatchctl/bridge/bin/devcontainer-open \"$@\"\n"
}

func installHelperBinary(binPath string) error {
	helperPath := filepath.Join(binPath, "hatchctl")
	data, err := helperBinaryData()
	if err != nil {
		return err
	}
	if err := os.WriteFile(helperPath, data, 0o755); err != nil {
		return err
	}
	return os.Chmod(helperPath, 0o755)
}

func helperBinaryData() ([]byte, error) {
	if configured := os.Getenv(helperBinaryEnvVar); configured != "" {
		data, err := os.ReadFile(configured)
		if err != nil {
			return nil, fmt.Errorf("bridge helper %s=%q: %w", helperBinaryEnvVar, configured, err)
		}
		return data, nil
	}

	if data, ok := embeddedHelperBinary(runtime.GOARCH); ok {
		return data, nil
	}
	return nil, fmt.Errorf("bridge helper not available for host %s/%s; set %s for development builds or use a release build of hatchctl", runtime.GOOS, runtime.GOARCH, helperBinaryEnvVar)
}

func applySession(session *Session, merged devcontainer.MergedConfig) devcontainer.MergedConfig {
	containerEnv := cloneEnv(merged.ContainerEnv)
	containerEnv["BROWSER"] = filepath.ToSlash(filepath.Join(session.BinPath, "devcontainer-open"))
	containerEnv["DEVCONTAINER_BRIDGE_ENABLED"] = "true"
	containerEnv["DEVCONTAINER_BRIDGE_SOCKET"] = filepath.ToSlash(filepath.Join(session.MountPath, hostSocketName))
	containerEnv["DEVCONTAINER_BRIDGE_HELPER_SOCKET"] = filepath.ToSlash(filepath.Join(session.MountPath, helperSocketName))
	containerEnv["PATH"] = prependPath(session.BinPath, containerEnv["PATH"])
	mount := fmt.Sprintf("type=bind,source=%s,target=%s", session.StatePath, session.MountPath)
	merged.ContainerEnv = containerEnv
	merged.Mounts = appendMount(merged.Mounts, mount)
	return merged
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
		SocketPath: filepath.Join(bridgeDir, hostSocketName),
		HelperSock: filepath.Join(bridgeDir, helperSocketName),
		StatePath:  bridgeDir,
		ConfigPath: filepath.Join(bridgeDir, "bridge-config.json"),
		PIDPath:    filepath.Join(bridgeDir, "bridge.pid"),
		StatusPath: filepath.Join(bridgeDir, "bridge-status.json"),
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
	return os.WriteFile(path, data, 0o600)
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
