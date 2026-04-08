package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

type bridgeFileStore interface {
	ReadSession(string) (*Session, error)
	SaveSession(string, *Session) error
	ReadPID(string) (int, error)
	WritePID(string, int) error
	WriteConfig(*Session, string) error
	WriteStatus(*Session, statusFile) error
	ReadStatus(string) ([]byte, error)
}

type filesystemBridgeStore struct{}

var fileStore bridgeFileStore = filesystemBridgeStore{}

func (filesystemBridgeStore) ReadSession(bridgeDir string) (*Session, error) {
	data, err := os.ReadFile(filepath.Join(bridgeDir, "session.json"))
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (filesystemBridgeStore) SaveSession(bridgeDir string, session *Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bridgeDir, "session.json"), data, 0o600)
}

func (filesystemBridgeStore) ReadPID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(bytesTrimSpace(data)))
	if err != nil {
		return 0, nil
	}
	return pid, nil
}

func (filesystemBridgeStore) WritePID(pidPath string, pid int) error {
	return os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o600)
}

func (filesystemBridgeStore) WriteConfig(session *Session, containerID string) error {
	config := map[string]any{
		"sessionId":   session.ID,
		"containerId": containerID,
		"host":        session.Host,
		"port":        session.Port,
		"statusPath":  session.StatusPath,
		"pidPath":     session.PIDPath,
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(session.ConfigPath, data, 0o600)
}

func (filesystemBridgeStore) WriteStatus(session *Session, status statusFile) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(session.StatusPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(session.StatusPath, data, 0o600)
}

func (filesystemBridgeStore) ReadStatus(statusPath string) ([]byte, error) {
	return os.ReadFile(statusPath)
}

func bytesTrimSpace(data []byte) string {
	start := 0
	for start < len(data) && isSpace(data[start]) {
		start++
	}
	end := len(data)
	for end > start && isSpace(data[end-1]) {
		end--
	}
	return string(data[start:end])
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}
