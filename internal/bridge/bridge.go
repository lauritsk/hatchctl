package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/fileutil"
)

type Session struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	HelperArch string `json:"helperArch,omitempty"`
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

type Report struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	HelperArch string `json:"helperArch,omitempty"`
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	StatePath  string `json:"statePath"`
	ConfigPath string `json:"configPath,omitempty"`
	PIDPath    string `json:"pidPath,omitempty"`
	StatusPath string `json:"statusPath,omitempty"`
	HelperPath string `json:"helperPath"`
	MountPath  string `json:"mountPath"`
	BinPath    string `json:"binPath"`
	Status     string `json:"status"`
}

func ReportFromSession(session *Session) *Report {
	if session == nil {
		return nil
	}
	return &Report{
		ID:         session.ID,
		Enabled:    session.Enabled,
		HelperArch: session.HelperArch,
		Host:       session.Host,
		Port:       session.Port,
		StatePath:  session.StatePath,
		ConfigPath: session.ConfigPath,
		PIDPath:    session.PIDPath,
		StatusPath: session.StatusPath,
		HelperPath: session.HelperPath,
		MountPath:  session.MountPath,
		BinPath:    session.BinPath,
		Status:     session.Status,
	}
}

const (
	containerBridgeMountPath = "/var/run/hatchctl/bridge"
	helperBinaryEnvVar       = "HATCHCTL_BRIDGE_HELPER"
	containerBridgeHost      = "host.docker.internal"
)

func Prepare(stateDir string, enabled bool, helperArch string) (*Session, error) {
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
	if session.Host == "" {
		session.Host = containerBridgeHost
	}
	if session.Port == 0 {
		port, err := reserveBridgePort()
		if err != nil {
			return nil, err
		}
		session.Port = port
	}
	if session.Token == "" {
		session.Token = randomToken(18)
	}
	if err := fileutil.WriteFile(helperPath, []byte(openShim(session)), 0o755); err != nil {
		return nil, err
	}
	if err := fileutil.WriteFile(filepath.Join(binPath, "xdg-open"), []byte(xdgOpenShim()), 0o755); err != nil {
		return nil, err
	}
	resolvedHelperArch := normalizeHelperArch(helperArch)
	if session.HelperArch != "" {
		resolvedHelperArch = normalizeHelperArch(session.HelperArch)
	}
	if err := installHelperBinary(binPath, resolvedHelperArch); err != nil {
		return nil, err
	}
	session.Enabled = enabled
	session.HelperArch = resolvedHelperArch
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

func Preview(stateDir string, enabled bool) (*Session, error) {
	if !enabled {
		return nil, nil
	}
	bridgeDir := filepath.Join(stateDir, "bridge")
	session, err := readSession(bridgeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return session, nil
}

func Inject(session *Session, merged devcontainer.MergedConfig) devcontainer.MergedConfig {
	if session == nil {
		return merged
	}
	return applySession(session, merged)
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
		if data, err := fileStore.ReadStatus(session.StatusPath); err == nil {
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
		HelperArch: sessionHelperArch(session),
		Host:       sessionHost(session),
		Port:       sessionPort(session),
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

func openShim(session *Session) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu

if [ $# -lt 1 ]; then
  exit 1
fi

url="$1"
if [ -n "${DEVCONTAINER_BRIDGE_OPEN_COMMAND:-}" ]; then
  DEVCONTAINER_BRIDGE_URL="$url" exec /bin/sh -lc "$DEVCONTAINER_BRIDGE_OPEN_COMMAND"
fi

	exec %s bridge helper open --host %s --port %d --token %s --url "$url"
`, containerHelperBin, session.Host, session.Port, session.Token)
}

func xdgOpenShim() string {
	return "#!/bin/sh\nexec /var/run/hatchctl/bridge/bin/devcontainer-open \"$@\"\n"
}

func installHelperBinary(binPath string, helperArch string) error {
	helperPath := filepath.Join(binPath, "hatchctl")
	data, err := helperBinaryData(helperArch)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(helperPath, data, 0o755)
}

func helperBinaryData(helperArch string) ([]byte, error) {
	if configured := os.Getenv(helperBinaryEnvVar); configured != "" {
		data, err := os.ReadFile(configured)
		if err != nil {
			return nil, fmt.Errorf("bridge helper %s=%q: %w", helperBinaryEnvVar, configured, err)
		}
		return data, nil
	}
	helperArch = normalizeHelperArch(helperArch)
	data := embeddedHelpers[helperArch]
	if len(data) > 0 {
		return data, nil
	}
	supported := slices.Sorted(maps.Keys(embeddedHelpers))
	if helperArch == "" {
		return nil, fmt.Errorf("bridge helper not embedded in this build; use a release binary or set %s", helperBinaryEnvVar)
	}
	return nil, fmt.Errorf("bridge helper arch %q not embedded in this build; supported=%v; use a release binary or set %s", helperArch, supported, helperBinaryEnvVar)
}

func normalizeHelperArch(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return runtime.GOARCH
	}
	return value
}

func applySession(session *Session, merged devcontainer.MergedConfig) devcontainer.MergedConfig {
	containerEnv := cloneEnv(merged.ContainerEnv)
	containerEnv["BROWSER"] = filepath.ToSlash(filepath.Join(session.BinPath, "devcontainer-open"))
	containerEnv["DEVCONTAINER_BRIDGE_ENABLED"] = "true"
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
	return fileStore.ReadSession(bridgeDir)
}

func saveSession(bridgeDir string, session *Session) error {
	return fileStore.SaveSession(bridgeDir, session)
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

func sessionConfigPath(session *Session) string {
	if session == nil {
		return ""
	}
	return session.ConfigPath
}

func sessionHelperArch(session *Session) string {
	if session == nil {
		return ""
	}
	return session.HelperArch
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

func reserveBridgePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
