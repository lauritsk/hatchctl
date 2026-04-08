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
