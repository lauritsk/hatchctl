package policy

import (
	"github.com/lauritsk/hatchctl/internal/security"
)

func AllowInsecureFeatureVerification() bool {
	return envTruthy(security.AllowInsecureFeaturesEnvVar)
}
