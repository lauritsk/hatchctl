package display

import (
	"bytes"
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
