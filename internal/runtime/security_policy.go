package runtime

import (
	"context"
	"errors"
	"os"
	"strings"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/security"
)

type imageVerificationPolicy struct {
	strict bool
}

func newImageVerificationPolicy() imageVerificationPolicy {
	return imageVerificationPolicy{strict: envTruthy(security.CosignStrictEnvVar)}
}

func (p imageVerificationPolicy) Check(ctx context.Context, ref string) security.VerificationResult {
	return security.VerifyImage(ctx, ref)
}

func (p imageVerificationPolicy) Apply(result security.VerificationResult, events ui.Sink) error {
	if result.Verified || result.Reason == "" {
		return nil
	}
	if p.strict {
		return errors.New(result.Error())
	}
	if events != nil {
		events.Emit(ui.Event{Kind: ui.EventWarning, Message: result.Error()})
	}
	return nil
}

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
