package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lauritsk/hatchctl/internal/security"
)

type imageVerificationPolicy struct {
	strict bool
	stderr io.Writer
}

func newImageVerificationPolicy(stderr io.Writer) imageVerificationPolicy {
	return imageVerificationPolicy{strict: envTruthy(security.CosignStrictEnvVar), stderr: stderr}
}

func (p imageVerificationPolicy) Check(ctx context.Context, ref string) security.VerificationResult {
	return security.VerifyImage(ctx, ref)
}

func (p imageVerificationPolicy) Apply(result security.VerificationResult) error {
	if result.Verified || result.Reason == "" {
		return nil
	}
	if p.strict {
		return errors.New(result.Error())
	}
	if p.stderr != nil {
		_, _ = fmt.Fprintf(p.stderr, "warning: %s\n", result.Error())
	}
	return nil
}

func (p imageVerificationPolicy) Verify(ctx context.Context, ref string) error {
	return p.Apply(p.Check(ctx, ref))
}

func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
