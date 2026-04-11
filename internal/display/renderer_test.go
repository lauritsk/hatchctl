package display

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
	s := &lineSpinner{w: &buf, styles: styles{frame: lipgloss.NewStyle().Foreground(lipgloss.Cyan)}}

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

func TestRendererProgressIncludesPhaseForNonTTY(t *testing.T) {
	t.Parallel()

	var errBuf bytes.Buffer
	r := NewRenderer(&bytes.Buffer{}, &errBuf, false)

	r.Emit(Event{Kind: EventProgress, Phase: "Resolve", Message: "Resolving development container"})

	if got := errBuf.String(); got != "==> [Resolve] Resolving development container\n" {
		t.Fatalf("unexpected phase progress output %q", got)
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

func TestRendererEventsAndTTYHelpers(t *testing.T) {
	t.Parallel()

	var nilRenderer *Renderer
	if nilRenderer.Events() != nil {
		t.Fatal("expected nil renderer to return nil events sink")
	}
	if nilRenderer.TTY() {
		t.Fatal("expected nil renderer to report non-tty")
	}

	r := &Renderer{jsonOut: true}
	if r.Events() != nil {
		t.Fatal("expected json renderer to suppress events sink")
	}
	r.jsonOut = false
	r.outTTY = true
	if r.Events() != r {
		t.Fatal("expected renderer to return itself as sink")
	}
	if !r.TTY() {
		t.Fatal("expected renderer to report tty")
	}
}

func TestRendererPrintHelpersForNonTTY(t *testing.T) {
	t.Parallel()

	var outBuf bytes.Buffer
	r := NewRenderer(&outBuf, &bytes.Buffer{}, false)

	if err := r.PrintText("hello"); err != nil {
		t.Fatalf("print text: %v", err)
	}
	if err := r.PrintKeyValues([]KeyValue{{Key: "Container", Value: "abc123"}, {Key: "Image", Value: "demo"}}); err != nil {
		t.Fatalf("print key values: %v", err)
	}
	if err := r.PrintJSON(map[string]any{"ok": true, "count": 2}); err != nil {
		t.Fatalf("print json: %v", err)
	}

	got := outBuf.String()
	for _, want := range []string{"hello\n", "Container: abc123\n", "Image: demo\n", "\"ok\": true", "\"count\": 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestRendererStderrAndCloseHelpers(t *testing.T) {
	t.Parallel()

	var errBuf bytes.Buffer
	spinner := &lineSpinner{w: &errBuf, done: make(chan struct{}), running: true, cursorHidden: true, lastSize: 4}
	r := &Renderer{err: &bytes.Buffer{}, spinner: spinner}
	if r.Stderr() == nil {
		t.Fatal("expected stderr writer")
	}
	r.Close()
	if !spinner.stopped {
		t.Fatal("expected close to stop spinner")
	}
	if spinner.cursorHidden {
		t.Fatal("expected close to restore cursor visibility")
	}

	if (&Renderer{}).Stderr() != nil {
		t.Fatal("expected renderer without stderr to return nil")
	}
}

func TestRendererNonTTYStyleHelpers(t *testing.T) {
	t.Parallel()

	r := &Renderer{}
	if got := r.styleDebug("debug line"); got != "debug line" {
		t.Fatalf("unexpected debug style %q", got)
	}
	if got := r.styleWarning("watch out"); got != "warning: watch out" {
		t.Fatalf("unexpected warning style %q", got)
	}
	if got := r.spinnerProgressMessage("", "Working"); got != "Working" {
		t.Fatalf("unexpected spinner message %q", got)
	}
	if got := r.spinnerProgressMessage("Resolve", "Working"); got != "[Resolve] Working" {
		t.Fatalf("unexpected phased spinner message %q", got)
	}
}

func TestLineSpinnerSetMessageAndWriteLine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	s := &lineSpinner{w: &buf, done: make(chan struct{})}
	s.SetMessage("loading")
	if !s.running || s.message != "loading" {
		t.Fatalf("expected spinner to start with message, got running=%v message=%q", s.running, s.message)
	}
	s.SetMessage("")
	if s.running {
		t.Fatal("expected empty message to stop spinner")
	}
	s.WriteLine("done")
	if got := buf.String(); !strings.Contains(got, "done\n") {
		t.Fatalf("unexpected spinner line output %q", got)
	}
}
