package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunReturnsZeroForVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}
	if stdout.Len() == 0 {
		t.Fatal("expected version output")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", stderr.String())
	}
}

func TestRunReturnsOneAndPrintsErrorForInvalidCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"missing-command"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Fatalf("expected formatted error output, got %q", stderr.String())
	}
}
