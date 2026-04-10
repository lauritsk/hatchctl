package command

import (
	"context"
	"strings"
	"testing"
)

func TestLocalOutputTrimsStdoutAndPreservesStderr(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := Local{}.Output(context.Background(), Command{
		Binary: "sh",
		Args:   []string{"-c", "printf 'hello\\n'; printf 'warning\\n' >&2"},
	})
	if err != nil {
		t.Fatalf("run output command: %v", err)
	}
	if stdout != "hello" {
		t.Fatalf("unexpected stdout %q", stdout)
	}
	if stderr != "warning\n" {
		t.Fatalf("unexpected stderr %q", stderr)
	}
}

func TestLocalCombinedOutputTrimsTrailingWhitespace(t *testing.T) {
	t.Parallel()

	output, err := Local{}.CombinedOutput(context.Background(), Command{
		Binary: "sh",
		Args:   []string{"-c", "printf 'hello\\n\\n'"},
	})
	if err != nil {
		t.Fatalf("run combined output command: %v", err)
	}
	if output != "hello" {
		t.Fatalf("unexpected combined output %q", output)
	}
}

func TestAppendEnvReturnsIndependentSlice(t *testing.T) {
	t.Parallel()

	base := []string{"A=1"}
	combined := AppendEnv(base, "B=2")
	combined[0] = "A=changed"
	if base[0] != "A=1" {
		t.Fatalf("expected base slice to remain unchanged, got %#v", base)
	}
	if got := strings.Join(combined, ","); got != "A=changed,B=2" {
		t.Fatalf("unexpected combined env %q", got)
	}
}
