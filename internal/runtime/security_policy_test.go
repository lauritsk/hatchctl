package runtime

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/security"
)

func TestImageVerificationPolicyApplyWarnsWhenNotStrict(t *testing.T) {
	var stderr bytes.Buffer
	policy := imageVerificationPolicy{stderr: &stderr}
	result := security.VerificationResult{Ref: "example.com/demo/app:latest", Reason: "no signatures found"}
	if err := policy.Apply(result); err != nil {
		t.Fatalf("apply verification policy: %v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "warning: unable to verify example.com/demo/app:latest: no signatures found") {
		t.Fatalf("unexpected warning output %q", got)
	}
}

func TestImageVerificationPolicyApplyFailsWhenStrict(t *testing.T) {
	policy := imageVerificationPolicy{strict: true}
	result := security.VerificationResult{Ref: "example.com/demo/app:latest", Reason: "no signatures found"}
	if err := policy.Apply(result); err == nil || !strings.Contains(err.Error(), "unable to verify example.com/demo/app:latest") {
		t.Fatalf("unexpected strict verification error %v", err)
	}
}

func TestVerifyResolvedFeaturesAppliesFeatureVerificationPolicy(t *testing.T) {
	runner := &Runner{imageVerifier: imageVerificationPolicy{strict: true}}
	resolved := devcontainer.ResolvedConfig{Features: []devcontainer.ResolvedFeature{{
		Source:       "ghcr.io/devcontainers/features/go:1",
		Verification: security.VerificationResult{Ref: "ghcr.io/devcontainers/features/go@sha256:abc", Reason: "no signatures found"},
	}}}
	if err := runner.verifyResolvedFeatures(resolved); err == nil || !strings.Contains(err.Error(), "verify feature \"ghcr.io/devcontainers/features/go:1\"") {
		t.Fatalf("unexpected feature verification error %v", err)
	}
}
