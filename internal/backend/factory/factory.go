package factory

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend"
	backenddocker "github.com/lauritsk/hatchctl/internal/backend/docker"
	backendpodman "github.com/lauritsk/hatchctl/internal/backend/podman"
)

var backendOrder = []string{"docker", "podman"}

func NormalizeName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return "docker"
	}
	return normalized
}

func New(name string) (backend.Client, error) {
	switch NormalizeName(name) {
	case "auto":
		detected, err := DetectName()
		if err != nil {
			return nil, err
		}
		return New(detected)
	case "docker":
		return backenddocker.New("docker"), nil
	case "podman":
		return backendpodman.New("podman"), nil
	default:
		return nil, backend.UnsupportedBackendError{Name: name}
	}
}

func DetectName() (string, error) {
	for _, name := range backendOrder {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("auto backend detection found no supported backend in PATH (tried %s)", strings.Join(backendOrder, ", "))
}
