package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/security"
)

type imageVerificationPolicy struct {
	strict bool
	prompt verificationPrompter
	mu     sync.Mutex
	trust  map[string]struct{}
}

type verificationPrompter func(string) (bool, bool, error)

type verificationDecision int

const (
	verificationDecisionWarn verificationDecision = iota
	verificationDecisionRequireTrust
)

func newImageVerificationPolicy(stdin io.Reader, stderr io.Writer) *imageVerificationPolicy {
	return &imageVerificationPolicy{
		strict: envTruthy(security.CosignStrictEnvVar),
		prompt: newVerificationPrompter(stdin, stderr),
		trust:  map[string]struct{}{},
	}
}

func (p *imageVerificationPolicy) Check(ctx context.Context, ref string) security.VerificationResult {
	return security.VerifyImage(ctx, ref)
}

func (p *imageVerificationPolicy) ApplyImage(result security.VerificationResult, events ui.Sink) error {
	return p.apply(result, events, verificationDecisionWarn)
}

func (p *imageVerificationPolicy) ApplyFeature(source string, result security.VerificationResult, allowUnverified bool, events ui.Sink) error {
	if allowUnverified {
		return nil
	}
	if err := p.apply(result, events, verificationDecisionRequireTrust); err != nil {
		return fmt.Errorf("verify feature %q: %w", source, err)
	}
	return nil
}

func (p *imageVerificationPolicy) apply(result security.VerificationResult, events ui.Sink, decision verificationDecision) error {
	if result.Verified || result.Reason == "" {
		return nil
	}
	if p.strict {
		return errors.New(result.Error())
	}
	if p.approved(result.Ref) {
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

func (p *imageVerificationPolicy) approved(ref string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.trust[ref]
	return ok
}

func (p *imageVerificationPolicy) rememberApproval(ref string) {
	if ref == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trust[ref] = struct{}{}
}

func (p *imageVerificationPolicy) promptTrust(result security.VerificationResult, decision verificationDecision) (bool, bool, error) {
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

func newVerificationPrompter(stdin io.Reader, stderr io.Writer) verificationPrompter {
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
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, true, nil
		default:
			return false, true, nil
		}
	}
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
