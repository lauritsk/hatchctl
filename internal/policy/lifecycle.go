package policy

import (
	"errors"
	"fmt"

	"github.com/lauritsk/hatchctl/internal/spec"
)

var ErrHostLifecycleNotAllowed = errors.New("host lifecycle commands require explicit trust")

func HostLifecycleTrustRequired(command spec.LifecycleCommand) bool {
	return !command.Empty()
}

func EnsureHostLifecycleAllowed(command spec.LifecycleCommand, allow bool) error {
	if command.Empty() || allow {
		return nil
	}
	return fmt.Errorf("%w; rerun with --allow-host-lifecycle or set HATCHCTL_ALLOW_HOST_LIFECYCLE=1", ErrHostLifecycleNotAllowed)
}
