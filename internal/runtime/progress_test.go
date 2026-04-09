package runtime

import (
	"bytes"
	"testing"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func TestProgressDockerRunOptionsPrintsHeaderOnceOnFirstOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink := &recordedSink{}
	runner := &Runner{}
	opts := runner.progressDockerRunOptions(sink, phaseDotfiles, "Installing dotfiles", docker.RunOptions{Stdout: &stdout, Stderr: &stderr})

	if _, err := opts.Stdout.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if _, err := opts.Stderr.Write([]byte("warning\n")); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	if got := stdout.String(); got != "hello\n" {
		t.Fatalf("unexpected stdout %q", got)
	}
	if got := stderr.String(); got != "warning\n" {
		t.Fatalf("unexpected stderr %q", got)
	}
	if len(sink.events) != 2 || sink.events[0].Kind != ui.EventClear || sink.events[1].Kind != ui.EventProgressOutput || sink.events[1].Phase != phaseDotfiles || sink.events[1].Message != "Installing dotfiles" {
		t.Fatalf("unexpected events %#v", sink.events)
	}
}

func TestProgressCommandIODoesNotPrintHeaderWithoutOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink := &recordedSink{}
	runner := &Runner{}
	streams := runner.progressCommandIO(sink, phaseLifecycle, "Running initializeCommand lifecycle hook", commandIO{Stdout: &stdout, Stderr: &stderr})

	if streams.Stdout == nil || streams.Stderr == nil {
		t.Fatal("expected wrapped streams")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(sink.events) != 0 {
		t.Fatalf("unexpected events %#v", sink.events)
	}
}

func TestProgressCommandIOFallsBackToStdoutForHeader(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	sink := &recordedSink{}
	runner := &Runner{}
	streams := runner.progressCommandIO(sink, phaseLifecycle, "Running initializeCommand lifecycle hook", commandIO{Stdout: &stdout})

	if _, err := streams.Stdout.Write([]byte("output\n")); err != nil {
		t.Fatalf("write stdout: %v", err)
	}

	if got := stdout.String(); got != "output\n" {
		t.Fatalf("unexpected stdout %q", got)
	}
	if len(sink.events) != 2 || sink.events[0].Kind != ui.EventClear || sink.events[1].Kind != ui.EventProgressOutput || sink.events[1].Phase != phaseLifecycle || sink.events[1].Message != "Running initializeCommand lifecycle hook" {
		t.Fatalf("unexpected events %#v", sink.events)
	}
}
