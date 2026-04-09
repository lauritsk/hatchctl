package policy

import (
	"os"
	"strings"
)

func envTruthy(name string) bool {
	return envTruthyValue(os.Getenv(name))
}

func envTruthyValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
