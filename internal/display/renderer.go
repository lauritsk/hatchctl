package display

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type EventKind string

const (
	EventProgress       EventKind = "progress"
	EventProgressOutput EventKind = "progress_output"
	EventWarning        EventKind = "warning"
	EventDebug          EventKind = "debug"
	EventClear          EventKind = "clear"
)

type Event struct {
	Kind    EventKind
	Message string
}

type Sink interface {
	Emit(Event)
}

type KeyValue struct {
	Key   string
	Value string
}

type Renderer struct {
	out     io.Writer
	err     io.Writer
	jsonOut bool
	outTTY  bool
	errTTY  bool
	styles  styles
	spinner *lineSpinner
}

type streamWriter struct {
	renderer *Renderer
	target   io.Writer
}

type fdWriter interface {
	Fd() uintptr
}

func (r *Renderer) Events() Sink {
	if r == nil || r.jsonOut {
		return nil
	}
	return r
}

type styles struct {
	label    lipgloss.Style
	text     lipgloss.Style
	progress lipgloss.Style
	debug    lipgloss.Style
	frame    lipgloss.Style
}

func NewRenderer(out io.Writer, err io.Writer, jsonOut bool) *Renderer {
	outTTY := isTerminal(out)
	errTTY := isTerminal(err)
	r := &Renderer{
		out:     out,
		err:     err,
		jsonOut: jsonOut,
		outTTY:  outTTY,
		errTTY:  errTTY,
		styles: styles{
			label:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
			text:     lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
			progress: lipgloss.NewStyle().Bold(true),
			debug:    lipgloss.NewStyle().Faint(true),
			frame:    lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		},
	}
	if !jsonOut && errTTY {
		r.spinner = newLineSpinner(err, r.styles)
	}
	return r
}

func (r *Renderer) Emit(event Event) {
	if r == nil || r.jsonOut {
		return
	}
	if r.spinner != nil {
		switch event.Kind {
		case EventProgress:
			if event.Message == "" {
				return
			}
			r.spinner.SetMessage(event.Message)
		case EventProgressOutput:
			if event.Message == "" {
				return
			}
			r.spinner.WriteLine(r.styleProgress(event.Message))
		case EventWarning:
			if event.Message == "" {
				return
			}
			r.spinner.WriteLine(r.styleWarning(event.Message))
		case EventDebug:
			if event.Message == "" {
				return
			}
			r.spinner.WriteLine(r.styleDebug(event.Message))
		case EventClear:
			r.spinner.Clear()
		}
		return
	}
	switch event.Kind {
	case EventProgress:
		if event.Message == "" {
			return
		}
		_, _ = fmt.Fprintf(r.err, "==> %s\n", event.Message)
	case EventWarning:
		if event.Message == "" {
			return
		}
		_, _ = fmt.Fprintf(r.err, "warning: %s\n", event.Message)
	case EventDebug:
		if event.Message == "" {
			return
		}
		_, _ = fmt.Fprintln(r.err, event.Message)
	}
}

func (r *Renderer) PrintText(text string) error {
	r.pauseProgress()
	if r.outTTY {
		text = r.styles.text.Render(text)
	}
	_, err := fmt.Fprintln(r.out, text)
	return err
}

func (r *Renderer) Stdout() io.Writer {
	if r == nil {
		return nil
	}
	target := r.out
	if r.jsonOut {
		target = r.err
	}
	if target == nil {
		return nil
	}
	return &streamWriter{renderer: r, target: target}
}

func (r *Renderer) Stderr() io.Writer {
	if r == nil || r.err == nil {
		return nil
	}
	return &streamWriter{renderer: r, target: r.err}
}

func (r *Renderer) PrintKeyValues(values []KeyValue) error {
	r.pauseProgress()
	for _, value := range values {
		line := value.Key + ": " + value.Value
		if r.outTTY {
			line = r.styles.label.Render(value.Key+":") + " " + r.styles.text.Render(value.Value)
		}
		if _, err := fmt.Fprintln(r.out, line); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) PrintJSON(value any) error {
	r.pauseProgress()
	enc := json.NewEncoder(r.out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func (r *Renderer) Close() {
	if r == nil || r.spinner == nil {
		return
	}
	r.spinner.Stop()
}

func (r *Renderer) pauseProgress() {
	if r.spinner != nil {
		r.spinner.Clear()
	}
}

func (w *streamWriter) Write(p []byte) (int, error) {
	if w == nil || w.target == nil {
		return len(p), nil
	}
	if w.renderer != nil {
		w.renderer.pauseProgress()
	}
	return w.target.Write(p)
}

func (w *streamWriter) Fd() uintptr {
	if w == nil || w.target == nil {
		return 0
	}
	f, ok := w.target.(fdWriter)
	if !ok {
		return 0
	}
	return f.Fd()
}

func (r *Renderer) styleDebug(message string) string {
	if !r.errTTY {
		return message
	}
	return r.styles.debug.Render(message)
}

func (r *Renderer) styleProgress(message string) string {
	if !r.errTTY {
		return "==> " + message
	}
	return r.styles.progress.Render("==> " + message)
}

func (r *Renderer) styleWarning(message string) string {
	if !r.errTTY {
		return "warning: " + message
	}
	return r.styles.debug.Render("warning: " + message)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(fdWriter)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

type lineSpinner struct {
	w      io.Writer
	styles styles

	mu           sync.Mutex
	message      string
	running      bool
	stopped      bool
	cursorHidden bool
	lastSize     int
	done         chan struct{}
}

func newLineSpinner(w io.Writer, styles styles) *lineSpinner {
	s := &lineSpinner{w: w, styles: styles, done: make(chan struct{})}
	go s.run()
	return s
}

func (s *lineSpinner) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
	if message == "" {
		s.clearLocked()
		s.running = false
		return
	}
	s.running = true
}

func (s *lineSpinner) WriteLine(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
	_, _ = fmt.Fprintln(s.w, line)
}

func (s *lineSpinner) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
	s.running = false
}

func (s *lineSpinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.clearLocked()
	s.mu.Unlock()
	close(s.done)
}

func (s *lineSpinner) run() {
	frames := spinner.MiniDot.Frames
	interval := spinner.MiniDot.FPS
	if interval <= 0 {
		interval = 80 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	index := 0
	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			if s.stopped {
				s.mu.Unlock()
				return
			}
			if s.running && s.message != "" {
				frame := frames[index%len(frames)]
				line := s.styles.frame.Render(frame) + " " + s.styles.progress.Render(s.message)
				s.renderLocked(line)
				index++
			}
			s.mu.Unlock()
		case <-s.done:
			return
		}
	}
}

func (s *lineSpinner) renderLocked(line string) {
	s.hideCursorLocked()
	padding := ""
	lineWidth := lipgloss.Width(line)
	if extra := s.lastSize - lineWidth; extra > 0 {
		padding = strings.Repeat(" ", extra)
	}
	_, _ = fmt.Fprintf(s.w, "\r%s%s", line, padding)
	s.lastSize = lineWidth
}

func (s *lineSpinner) clearLocked() {
	s.showCursorLocked()
	if s.lastSize == 0 {
		return
	}
	_, _ = fmt.Fprintf(s.w, "\r%s\r", strings.Repeat(" ", s.lastSize))
	s.lastSize = 0
}

func (s *lineSpinner) hideCursorLocked() {
	if s.cursorHidden {
		return
	}
	_, _ = fmt.Fprint(s.w, "\x1b[?25l")
	s.cursorHidden = true
}

func (s *lineSpinner) showCursorLocked() {
	if !s.cursorHidden {
		return
	}
	_, _ = fmt.Fprint(s.w, "\x1b[?25h")
	s.cursorHidden = false
}
