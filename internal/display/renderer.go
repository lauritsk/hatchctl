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

func (r *Renderer) TTY() bool {
	if r == nil {
		return false
	}
	return r.outTTY
}

type styles struct {
	title      lipgloss.Style
	label      lipgloss.Style
	text       lipgloss.Style
	muted      lipgloss.Style
	progress   lipgloss.Style
	debug      lipgloss.Style
	warning    lipgloss.Style
	success    lipgloss.Style
	frame      lipgloss.Style
	box        lipgloss.Style
	command    lipgloss.Style
	commandBox lipgloss.Style
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
			title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
			label:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
			text:       lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
			muted:      lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8")),
			progress:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")),
			debug:      lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8")),
			warning:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
			success:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")),
			frame:      lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
			box:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("6")).Padding(0, 1),
			command:    lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
			commandBox: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(0, 1),
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
	if r.outTTY {
		_, err := fmt.Fprintln(r.out, r.renderKeyValuesBox("Summary", values))
		return err
	}
	for _, value := range values {
		line := value.Key + ": " + value.Value
		if _, err := fmt.Fprintln(r.out, line); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) PrintSummary(title string, values []KeyValue) error {
	r.pauseProgress()
	if !r.outTTY {
		if title != "" {
			if _, err := fmt.Fprintln(r.out, title); err != nil {
				return err
			}
		}
		for _, value := range values {
			if _, err := fmt.Fprintf(r.out, "%s: %s\n", value.Key, value.Value); err != nil {
				return err
			}
		}
		return nil
	}
	_, err := fmt.Fprintln(r.out, r.renderKeyValuesBox(title, values))
	return err
}

func (r *Renderer) PrintCommandList(title string, commands []string) error {
	r.pauseProgress()
	if !r.outTTY {
		if title != "" {
			if _, err := fmt.Fprintln(r.out, title+":"); err != nil {
				return err
			}
		}
		for _, command := range commands {
			if _, err := fmt.Fprintln(r.out, "  "+command); err != nil {
				return err
			}
		}
		return nil
	}
	lines := make([]string, 0, len(commands)+1)
	if title != "" {
		lines = append(lines, r.styles.title.Render(title))
	}
	for _, command := range commands {
		lines = append(lines, r.styles.command.Render("$ ")+r.styles.text.Render(command))
	}
	_, err := fmt.Fprintln(r.out, r.styles.commandBox.Render(strings.Join(lines, "\n")))
	return err
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
	return r.styles.frame.Render("[run]") + " " + r.styles.progress.Render(message)
}

func (r *Renderer) styleWarning(message string) string {
	if !r.errTTY {
		return "warning: " + message
	}
	return r.styles.warning.Render("warning:") + " " + r.styles.text.Render(message)
}

func (r *Renderer) renderKeyValuesBox(title string, values []KeyValue) string {
	width := 0
	for _, value := range values {
		if w := lipgloss.Width(value.Key); w > width {
			width = w
		}
	}
	lines := make([]string, 0, len(values)+1)
	if title != "" {
		lines = append(lines, r.styles.title.Render(title))
	}
	for _, value := range values {
		key := r.styles.label.Render(fmt.Sprintf("%-*s", width, value.Key))
		lines = append(lines, key+"  "+r.styles.text.Render(value.Value))
	}
	return r.styles.box.Render(strings.Join(lines, "\n"))
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
