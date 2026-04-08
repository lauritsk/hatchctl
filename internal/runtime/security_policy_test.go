package runtime

import (
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/security"
)

func TestImageVerificationPolicyApplyWarnsWhenNotStrict(t *testing.T) {
	sink := &recordedSink{}
	policy := imageVerificationPolicy{}
	result := security.VerificationResult{Ref: "example.com/demo/app:latest", Reason: "no signatures found"}
	if err := policy.Apply(result, sink); err != nil {
		t.Fatalf("apply verification policy: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0] != (ui.Event{Kind: ui.EventWarning, Message: result.Error()}) {
		t.Fatalf("unexpected warning events %#v", sink.events)
	}
}

func TestImageVerificationPolicyApplyFailsWhenStrict(t *testing.T) {
	policy := imageVerificationPolicy{strict: true}
	result := security.VerificationResult{Ref: "example.com/demo/app:latest", Reason: "no signatures found"}
	if err := policy.Apply(result, nil); err == nil || !strings.Contains(err.Error(), "unable to verify example.com/demo/app:latest") {
		t.Fatalf("unexpected strict verification error %v", err)
	}
}

func TestVerifyResolvedFeaturesAppliesFeatureVerificationPolicy(t *testing.T) {
	runner := &Runner{imageVerifier: imageVerificationPolicy{strict: true}}
	resolved := devcontainer.ResolvedConfig{Features: []devcontainer.ResolvedFeature{{
		Source:       "ghcr.io/devcontainers/features/go:1",
		Verification: security.VerificationResult{Ref: "ghcr.io/devcontainers/features/go@sha256:abc", Reason: "no signatures found"},
	}}}
	if err := runner.verifyResolvedFeatures(resolved, nil); err == nil || !strings.Contains(err.Error(), "verify feature \"ghcr.io/devcontainers/features/go:1\"") {
		t.Fatalf("unexpected feature verification error %v", err)
	}
}
