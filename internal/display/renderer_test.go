package display

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLineSpinnerHidesAndRestoresCursor(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	s := &lineSpinner{w: &buf}

	s.renderLocked("x")
	s.clearLocked()

	out := buf.String()
	if strings.Count(out, "\x1b[?25l") != 1 {
		t.Fatalf("expected cursor hide sequence once, got %q", out)
	}
	if strings.Count(out, "\x1b[?25h") != 1 {
		t.Fatalf("expected cursor show sequence once, got %q", out)
	}
}

func TestLineSpinnerUsesVisibleWidthForPadding(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	s := &lineSpinner{w: &buf, styles: styles{frame: lipgloss.NewStyle().Foreground(lipgloss.Color("6"))}}

	first := s.styles.frame.Render("dot") + " message"
	second := "x"

	s.renderLocked(first)
	s.renderLocked(second)

	out := buf.String()
	if !strings.Contains(out, "\rx"+strings.Repeat(" ", lipgloss.Width(first)-lipgloss.Width(second))) {
		t.Fatalf("expected redraw to pad to prior visible width, got %q", out)
	}
}

func TestRendererClearEventStopsSpinner(t *testing.T) {
	t.Parallel()

	spinner := &lineSpinner{running: true}
	r := &Renderer{spinner: spinner}

	r.Emit(Event{Kind: EventClear})

	if spinner.running {
		t.Fatal("expected clear event to stop spinner")
	}
}

func TestRendererWarningEventPrintsWarningPrefix(t *testing.T) {
	t.Parallel()

	var errBuf bytes.Buffer
	r := NewRenderer(&bytes.Buffer{}, &errBuf, false)

	r.Emit(Event{Kind: EventWarning, Message: "be careful"})

	if got := errBuf.String(); got != "warning: be careful\n" {
		t.Fatalf("unexpected warning output %q", got)
	}
}

func TestRendererProgressOutputDoesNotDuplicateNonTTYProgress(t *testing.T) {
	t.Parallel()

	var errBuf bytes.Buffer
	r := NewRenderer(&bytes.Buffer{}, &errBuf, false)

	r.Emit(Event{Kind: EventProgress, Message: "Installing dotfiles"})
	r.Emit(Event{Kind: EventProgressOutput, Message: "Installing dotfiles"})

	if got := errBuf.String(); got != "==> Installing dotfiles\n" {
		t.Fatalf("unexpected progress output %q", got)
	}
}

func TestRendererStdoutRedirectsToStderrForJSON(t *testing.T) {
	t.Parallel()

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	r := NewRenderer(&outBuf, &errBuf, true)

	if _, err := r.Stdout().Write([]byte("docker build log\n")); err != nil {
		t.Fatalf("write stdout: %v", err)
	}

	if outBuf.Len() != 0 {
		t.Fatalf("expected json stdout to stay clean, got %q", outBuf.String())
	}
	if got := errBuf.String(); got != "docker build log\n" {
		t.Fatalf("unexpected redirected output %q", got)
	}
}

func TestRendererManagedWriterClearsSpinner(t *testing.T) {
	t.Parallel()

	spinner := &lineSpinner{running: true}
	var outBuf bytes.Buffer
	r := &Renderer{out: &outBuf, spinner: spinner}

	if _, err := r.Stdout().Write([]byte("command output\n")); err != nil {
		t.Fatalf("write managed stdout: %v", err)
	}

	if spinner.running {
		t.Fatal("expected managed writer to clear spinner")
	}
	if got := outBuf.String(); got != "command output\n" {
		t.Fatalf("unexpected managed output %q", got)
	}
}

func TestRendererManagedWriterPreservesFileDescriptor(t *testing.T) {
	t.Parallel()

	file, err := os.CreateTemp(t.TempDir(), "renderer-out")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()

	r := &Renderer{out: file}
	writer, ok := r.Stdout().(interface{ Fd() uintptr })
	if !ok {
		t.Fatal("expected managed writer to expose Fd")
	}
	if got, want := writer.Fd(), file.Fd(); got != want {
		t.Fatalf("unexpected file descriptor %d want %d", got, want)
	}
}

func TestRendererPrintSummaryUsesBoxLayoutForTTY(t *testing.T) {
	t.Parallel()

	var outBuf bytes.Buffer
	r := &Renderer{
		out:    &outBuf,
		outTTY: true,
		styles: NewRenderer(&bytes.Buffer{}, &bytes.Buffer{}, false).styles,
	}

	err := r.PrintSummary("Devcontainer Ready", []KeyValue{{Key: "Container", Value: "abc123"}, {Key: "Image", Value: "demo"}})
	if err != nil {
		t.Fatalf("print summary: %v", err)
	}

	got := outBuf.String()
	for _, want := range []string{"Devcontainer Ready", "Container", "abc123", "Image", "demo"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected summary to contain %q, got %q", want, got)
		}
	}
	if strings.Contains(got, "development container details") {
		t.Fatalf("expected summary to avoid hardcoded subtitle, got %q", got)
	}
}

func TestRendererPrintCommandListUsesTTYCommandStyling(t *testing.T) {
	t.Parallel()

	var outBuf bytes.Buffer
	r := &Renderer{
		out:    &outBuf,
		outTTY: true,
		styles: NewRenderer(&bytes.Buffer{}, &bytes.Buffer{}, false).styles,
	}

	err := r.PrintCommandList("Next", []string{"hatchctl exec", "hatchctl exec -- pwd"})
	if err != nil {
		t.Fatalf("print command list: %v", err)
	}

	got := outBuf.String()
	for _, want := range []string{"Next", "$ ", "hatchctl exec", "hatchctl exec -- pwd"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected command list to contain %q, got %q", want, got)
		}
	}
}
