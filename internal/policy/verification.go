package policy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/security"
	"golang.org/x/term"
)

type ImageVerificationPolicy struct {
	strict bool
	prompt VerificationPrompter
	mu     sync.Mutex
	trust  map[string]struct{}
}

type VerificationPrompter func(string) (bool, bool, error)

type verificationDecision int

const (
	verificationDecisionWarn verificationDecision = iota
	verificationDecisionRequireTrust
)

func NewImageVerificationPolicy(stdin io.Reader, stderr io.Writer) *ImageVerificationPolicy {
	return &ImageVerificationPolicy{
		strict: envTruthy(security.CosignStrictEnvVar),
		prompt: NewVerificationPrompter(stdin, stderr),
		trust:  map[string]struct{}{},
	}
}

func NewImageVerificationPolicyWithPrompt(strict bool, prompt VerificationPrompter, trustedRefs ...string) *ImageVerificationPolicy {
	policy := &ImageVerificationPolicy{
		strict: strict,
		prompt: prompt,
		trust:  map[string]struct{}{},
	}
	for _, ref := range trustedRefs {
		policy.trust[ref] = struct{}{}
	}
	return policy
}

func (p *ImageVerificationPolicy) CloneWithIO(stdin io.Reader, stderr io.Writer) *ImageVerificationPolicy {
	if p == nil {
		return NewImageVerificationPolicy(stdin, stderr)
	}
	clone := &ImageVerificationPolicy{
		strict: p.strict,
		prompt: NewVerificationPrompter(stdin, stderr),
		trust:  map[string]struct{}{},
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for ref := range p.trust {
		clone.trust[ref] = struct{}{}
	}
	return clone
}

func (p *ImageVerificationPolicy) Check(ctx context.Context, ref string) security.VerificationResult {
	return security.VerifyImage(ctx, ref)
}

func (p *ImageVerificationPolicy) ApplyImage(result security.VerificationResult, events ui.Sink) error {
	return p.apply(result, events, verificationDecisionWarn)
}

func (p *ImageVerificationPolicy) ApplyFeature(source string, result security.VerificationResult, allowUnverified bool, events ui.Sink) error {
	if allowUnverified {
		return nil
	}
	if err := p.apply(result, events, verificationDecisionRequireTrust); err != nil {
		return fmt.Errorf("verify feature %q: %w", source, err)
	}
	return nil
}

func (p *ImageVerificationPolicy) Approved(ref string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.trust[ref]
	return ok
}

func (p *ImageVerificationPolicy) apply(result security.VerificationResult, events ui.Sink, decision verificationDecision) error {
	if result.Verified || result.Reason == "" {
		return nil
	}
	if p.strict {
		return errors.New(result.Error())
	}
	if p.Approved(result.Ref) {
		return nil
	}
	approved, prompted, err := p.promptTrust(result, decision)
	if err != nil {
		return err
	}
	if approved {
		p.rememberApproval(result.Ref)
		return nil
	}
	if decision == verificationDecisionRequireTrust {
		if prompted {
			return fmt.Errorf("user declined to trust %s", result.Ref)
		}
		return errors.New(result.Error())
	}
	if events != nil {
		events.Emit(ui.Event{Kind: ui.EventWarning, Message: result.Error()})
	}
	return nil
}

func (p *ImageVerificationPolicy) rememberApproval(ref string) {
	if ref == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trust[ref] = struct{}{}
}

func (p *ImageVerificationPolicy) promptTrust(result security.VerificationResult, decision verificationDecision) (bool, bool, error) {
	if p.prompt == nil {
		return false, false, nil
	}
	verb := "Continue with"
	target := "unsigned image"
	if decision == verificationDecisionRequireTrust {
		verb = "Trust"
		target = "unsigned feature"
	}
	message := fmt.Sprintf("%s\n%s %s for this run only? [y/N]: ", result.Error(), verb, target)
	return p.prompt(message)
}

func NewVerificationPrompter(stdin io.Reader, stderr io.Writer) VerificationPrompter {
	if !isTerminalStream(stdin) || !isTerminalStream(stderr) {
		return nil
	}
	reader := bufio.NewReader(stdin)
	return func(prompt string) (bool, bool, error) {
		if _, err := io.WriteString(stderr, prompt); err != nil {
			return false, false, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, true, err
		}
		switch lowerTrim(line) {
		case "y", "yes":
			return true, true, nil
		default:
			return false, true, nil
		}
	}
}

func isTerminalStream(stream any) bool {
	type fdStream interface{ Fd() uintptr }
	value, ok := stream.(fdStream)
	if !ok {
		return false
	}
	return isTerminal(int(value.Fd()))
}

var isTerminal = term.IsTerminal

func SetIsTerminalForTest(check func(int) bool) func() {
	previous := isTerminal
	isTerminal = check
	return func() {
		isTerminal = previous
	}
}

func lowerTrim(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
