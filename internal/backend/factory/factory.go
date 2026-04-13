package factory

import (
	"strings"

	"github.com/lauritsk/hatchctl/internal/backend"
	backenddocker "github.com/lauritsk/hatchctl/internal/backend/docker"
)

func NormalizeName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "docker"
	}
	return strings.TrimSpace(name)
}

func New(name string) (backend.Client, error) {
	switch NormalizeName(name) {
	case "docker":
		return backenddocker.New("docker"), nil
	default:
		return nil, backend.UnsupportedBackendError{Name: name}
	}
}
