package runtime

import (
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/security"
)

func TestImageVerificationPolicyApplyWarnsWhenNotStrict(t *testing.T) {
	t.Parallel()

	sink := &recordedSink{}
	policy := &imageVerificationPolicy{trust: map[string]struct{}{}}
	result := security.VerificationResult{Ref: "example.com/demo/app:latest", Reason: "no signatures found"}
	if err := policy.ApplyImage(result, sink); err != nil {
		t.Fatalf("apply verification policy: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0] != (ui.Event{Kind: ui.EventWarning, Message: result.Error()}) {
		t.Fatalf("unexpected warning events %#v", sink.events)
	}
}

func TestImageVerificationPolicyApplyFailsWhenStrict(t *testing.T) {
	t.Parallel()

	policy := &imageVerificationPolicy{strict: true, trust: map[string]struct{}{}}
	result := security.VerificationResult{Ref: "example.com/demo/app:latest", Reason: "no signatures found"}
	if err := policy.ApplyImage(result, nil); err == nil || !strings.Contains(err.Error(), "unable to verify example.com/demo/app:latest") {
		t.Fatalf("unexpected strict verification error %v", err)
	}
}

func TestImageVerificationPolicyApplyImageAllowsPromptedTrust(t *testing.T) {
	t.Parallel()

	policy := &imageVerificationPolicy{
		trust: map[string]struct{}{},
		prompt: func(prompt string) (bool, bool, error) {
			if !strings.Contains(prompt, "Continue with unsigned image for this run only") {
				t.Fatalf("unexpected prompt %q", prompt)
			}
			return true, true, nil
		},
	}
	result := security.VerificationResult{Ref: "example.com/demo/app:latest", Reason: "no signatures found"}
	if err := policy.ApplyImage(result, nil); err != nil {
		t.Fatalf("apply prompted image verification: %v", err)
	}
}

func TestImageVerificationPolicyApplyFeatureRequiresTrust(t *testing.T) {
	t.Parallel()

	policy := &imageVerificationPolicy{
		trust: map[string]struct{}{},
		prompt: func(prompt string) (bool, bool, error) {
			if !strings.Contains(prompt, "Trust unsigned feature for this run only") {
				t.Fatalf("unexpected prompt %q", prompt)
			}
			return false, true, nil
		},
	}
	result := security.VerificationResult{Ref: "ghcr.io/devcontainers/features/go@sha256:abc", Reason: "no signatures found"}
	if err := policy.ApplyFeature("ghcr.io/devcontainers/features/go:1", result, false, nil); err == nil || !strings.Contains(err.Error(), "user declined") {
		t.Fatalf("unexpected feature trust error %v", err)
	}
}

func TestVerifyResolvedFeaturesAppliesFeatureVerificationPolicy(t *testing.T) {
	t.Parallel()

	runner := &Runner{imageVerifier: &imageVerificationPolicy{strict: true, trust: map[string]struct{}{}}}
	resolved := devcontainer.ResolvedConfig{Features: []devcontainer.ResolvedFeature{{
		Source:       "ghcr.io/devcontainers/features/go:1",
		Verification: security.VerificationResult{Ref: "ghcr.io/devcontainers/features/go@sha256:abc", Reason: "no signatures found"},
	}}}
	if err := runner.verifyResolvedFeatures(resolved, nil); err == nil || !strings.Contains(err.Error(), "verify feature \"ghcr.io/devcontainers/features/go:1\"") {
		t.Fatalf("unexpected feature verification error %v", err)
	}
}
