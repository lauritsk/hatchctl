package policy

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/lauritsk/hatchctl/internal/spec"
)

type eventSink struct {
	events []ui.Event
}

func (s *eventSink) Emit(event ui.Event) {
	s.events = append(s.events, event)
}

func TestEnsureWorkspaceTrustRejectsRiskySettings(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	resolved := devcontainer.ResolvedConfig{
		WorkspaceFolder: workspace,
		ConfigDir:       filepath.Join(workspace, ".devcontainer"),
		Config: devcontainer.Config{
			RunArgs: []string{"--privileged"},
			Build:   &devcontainer.BuildConfig{Dockerfile: "../../Dockerfile"},
		},
		Merged: devcontainer.MergedConfig{
			Privileged: true,
			Mounts:     []string{"type=bind,source=/tmp,target=/host-tmp"},
		},
	}

	err := EnsureWorkspaceTrust(resolved, false)
	if err == nil {
		t.Fatal("expected workspace trust error")
	}
	if !errors.Is(err, ErrWorkspaceTrustRequired) {
		t.Fatalf("expected trust-required error, got %v", err)
	}
	for _, fragment := range []string{"privileged container mode is enabled", "bind mounts requested", "docker run arguments request host-affecting settings", "dockerfile path resolves outside the workspace"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("expected %q in error %q", fragment, err)
		}
	}
}

func TestEnsureHostLifecycleAllowedRequiresExplicitTrust(t *testing.T) {
	t.Parallel()

	err := EnsureHostLifecycleAllowed(spec.LifecycleCommand{Kind: "string", Value: "echo hi", Exists: true}, false)
	if err == nil {
		t.Fatal("expected host lifecycle trust error")
	}
	if !errors.Is(err, ErrHostLifecycleNotAllowed) {
		t.Fatalf("expected host lifecycle error, got %v", err)
	}
	if err := EnsureHostLifecycleAllowed(spec.LifecycleCommand{}, false); err != nil {
		t.Fatalf("expected empty command to be allowed, got %v", err)
	}
}

func TestIsLoopbackOCIReference(t *testing.T) {
	t.Parallel()

	for _, ref := range []string{"localhost/repo:latest", "localhost:5000/repo:latest", "127.0.0.1:5000/repo:latest"} {
		if !IsLoopbackOCIReference(ref) {
			t.Fatalf("expected %q to be loopback", ref)
		}
	}
	if IsLoopbackOCIReference("ghcr.io/example/repo:latest") {
		t.Fatal("expected remote registry to require verification")
	}
}

func TestImageVerificationPolicyWarnsForUnverifiedImages(t *testing.T) {
	t.Parallel()

	sink := &eventSink{}
	policy := NewImageVerificationPolicyWithPrompt(false, nil)
	result := security.VerificationResult{Ref: "ghcr.io/example/demo:latest", Reason: "no signatures found"}

	if err := policy.ApplyImage(result, sink); err != nil {
		t.Fatalf("apply image policy: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].Kind != ui.EventWarning {
		t.Fatalf("expected warning event, got %#v", sink.events)
	}
	if !strings.Contains(sink.events[0].Message, result.Ref) {
		t.Fatalf("expected warning to mention image ref, got %#v", sink.events[0])
	}
}

func TestImageVerificationPolicyRejectsUnverifiedFeaturesWithoutApproval(t *testing.T) {
	t.Parallel()

	policy := NewImageVerificationPolicyWithPrompt(false, func(string) (bool, bool, error) {
		return false, false, nil
	})
	result := security.VerificationResult{Ref: "ghcr.io/example/feature:latest", Reason: "no signatures found"}

	err := policy.ApplyFeature("ghcr.io/example/feature:latest", result, false, nil)
	if err == nil {
		t.Fatal("expected feature verification error")
	}
	if !strings.Contains(err.Error(), result.Ref) {
		t.Fatalf("expected feature error to mention ref, got %v", err)
	}
	if policy.Check(context.Background(), "") != (security.VerificationResult{Verified: true}) {
		t.Fatal("expected empty refs to verify trivially")
	}
}
