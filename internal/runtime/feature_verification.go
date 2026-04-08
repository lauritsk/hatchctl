package runtime

import (
	"strings"

	"github.com/lauritsk/hatchctl/internal/security"
)

func allowInsecureFeatureVerification() bool {
	return envTruthy(security.AllowInsecureFeaturesEnvVar)
}

func isLoopbackOCIReference(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	host, _, ok := strings.Cut(ref, "/")
	if !ok {
		return false
	}
	host = strings.ToLower(host)
	return host == "localhost" || strings.HasPrefix(host, "localhost:") || strings.HasPrefix(host, "127.0.0.1:")
}
