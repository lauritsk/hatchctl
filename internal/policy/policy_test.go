package policy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestImageVerificationPolicyTrustsApprovedFeatureForRun(t *testing.T) {
	t.Parallel()

	var prompts []string
	policy := NewImageVerificationPolicyWithPrompt(false, func(prompt string) (bool, bool, error) {
		prompts = append(prompts, prompt)
		return true, true, nil
	})
	result := security.VerificationResult{Ref: "ghcr.io/example/feature:latest", Reason: "no signatures found"}

	if err := policy.ApplyFeature(result.Ref, result, false, nil); err != nil {
		t.Fatalf("apply feature policy: %v", err)
	}
	if !policy.Approved(result.Ref) {
		t.Fatalf("expected %q to be approved", result.Ref)
	}
	if len(prompts) != 1 || !strings.Contains(prompts[0], "Trust unsigned feature") {
		t.Fatalf("unexpected prompts %#v", prompts)
	}
	if err := policy.ApplyFeature(result.Ref, result, false, nil); err != nil {
		t.Fatalf("expected approved feature to skip re-prompt, got %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected approved feature to reuse trust, got %d prompts", len(prompts))
	}
}

func TestImageVerificationPolicyDisablePromptRejectsWithoutPrompting(t *testing.T) {
	t.Parallel()

	prompted := false
	policy := NewImageVerificationPolicyWithPrompt(false, func(string) (bool, bool, error) {
		prompted = true
		return true, true, nil
	})
	policy.DisablePrompt()
	result := security.VerificationResult{Ref: "ghcr.io/example/image:latest", Reason: "no signatures found"}

	err := policy.ApplyFeature(result.Ref, result, false, nil)
	if err == nil {
		t.Fatal("expected feature verification error")
	}
	if prompted {
		t.Fatal("expected disabled prompt to skip prompting")
	}
	if !strings.Contains(err.Error(), result.Ref) {
		t.Fatalf("expected error to mention image ref, got %v", err)
	}
}

func TestImageVerificationPolicyCloneWithIOCopiesState(t *testing.T) {
	t.Parallel()

	signers := []security.TrustedSigner{{Issuer: "https://issuer.example", SubjectRegExp: "^repo$"}}
	policy := NewImageVerificationPolicyWithPrompt(false, nil, "ghcr.io/example/base@sha256:abc", "ghcr.io/example/feature@sha256:def")
	policy.SetTrustedSigners(signers)

	clone := policy.CloneWithIO(bytes.NewBuffer(nil), io.Discard)
	if clone == policy {
		t.Fatal("expected CloneWithIO to create a distinct policy")
	}
	if !reflect.DeepEqual(clone.signers, signers) {
		t.Fatalf("unexpected cloned signers %#v", clone.signers)
	}
	if got := clone.TrustedRefs(); !slices.Equal(got, []string{"ghcr.io/example/base@sha256:abc", "ghcr.io/example/feature@sha256:def"}) {
		t.Fatalf("unexpected cloned trusted refs %#v", got)
	}
	clone.TrustRefs("ghcr.io/example/other@sha256:123")
	if policy.Approved("ghcr.io/example/other@sha256:123") {
		t.Fatal("expected cloned trust updates to stay isolated")
	}
}

func TestNewVerificationPrompterReadsTerminalInput(t *testing.T) {
	t.Parallel()

	stdin, err := os.CreateTemp(t.TempDir(), "prompt-stdin")
	if err != nil {
		t.Fatalf("create stdin temp file: %v", err)
	}
	defer stdin.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "prompt-stderr")
	if err != nil {
		t.Fatalf("create stderr temp file: %v", err)
	}
	defer stderr.Close()
	if _, err := stdin.WriteString("YeS\n"); err != nil {
		t.Fatalf("seed stdin: %v", err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatalf("rewind stdin: %v", err)
	}

	restore := SetIsTerminalForTest(func(int) bool { return true })
	defer restore()

	prompt := NewVerificationPrompter(stdin, stderr)
	if prompt == nil {
		t.Fatal("expected terminal streams to enable prompting")
	}
	approved, prompted, err := prompt("Trust this image? ")
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if !approved || !prompted {
		t.Fatalf("expected yes input to approve, got approved=%v prompted=%v", approved, prompted)
	}
	data, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if string(data) != "Trust this image? " {
		t.Fatalf("unexpected prompt output %q", string(data))
	}
}

func TestNewVerificationPrompterReturnsNilForNonTerminalStreams(t *testing.T) {
	t.Parallel()

	if prompt := NewVerificationPrompter(strings.NewReader("yes\n"), io.Discard); prompt != nil {
		t.Fatal("expected non-terminal streams to disable prompting")
	}
}
