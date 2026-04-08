package runtime

import (
	"bytes"
	"testing"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

type recordedSink struct {
	events []ui.Event
}

func (s *recordedSink) Emit(event ui.Event) {
	s.events = append(s.events, event)
}

func TestProgressDockerRunOptionsPrintsHeaderOnceOnFirstOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink := &recordedSink{}
	runner := &Runner{}
	opts := runner.progressDockerRunOptions(sink, "Installing dotfiles", docker.RunOptions{Stdout: &stdout, Stderr: &stderr})

	if _, err := opts.Stdout.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if _, err := opts.Stderr.Write([]byte("warning\n")); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	if got := stdout.String(); got != "hello\n" {
		t.Fatalf("unexpected stdout %q", got)
	}
	if got := stderr.String(); got != "==> Installing dotfiles\nwarning\n" {
		t.Fatalf("unexpected stderr %q", got)
	}
	if len(sink.events) != 1 || sink.events[0].Kind != ui.EventClear {
		t.Fatalf("unexpected events %#v", sink.events)
	}
}

func TestProgressCommandIODoesNotPrintHeaderWithoutOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink := &recordedSink{}
	runner := &Runner{}
	streams := runner.progressCommandIO(sink, "Running initializeCommand", commandIO{Stdout: &stdout, Stderr: &stderr})

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
	streams := runner.progressCommandIO(sink, "Running initializeCommand", commandIO{Stdout: &stdout})

	if _, err := streams.Stdout.Write([]byte("output\n")); err != nil {
		t.Fatalf("write stdout: %v", err)
	}

	if got := stdout.String(); got != "==> Running initializeCommand\noutput\n" {
		t.Fatalf("unexpected stdout %q", got)
	}
	if len(sink.events) != 1 || sink.events[0].Kind != ui.EventClear {
		t.Fatalf("unexpected events %#v", sink.events)
	}
}
