package reconcile

import (
	"io"
	"sync"

	ui "github.com/lauritsk/hatchctl/internal/display"
)

func (e *Executor) progressCommandIO(events ui.Sink, phase string, label string, streams commandIO) commandIO {
	stdout, stderr := e.progressWriters(events, phase, label, streams.Stdout, streams.Stderr)
	streams.Stdout = stdout
	streams.Stderr = stderr
	return streams
}

func (e *Executor) progressWriters(events ui.Sink, phase string, label string, stdout io.Writer, stderr io.Writer) (io.Writer, io.Writer) {
	if events == nil || label == "" {
		return stdout, stderr
	}
	activate := &progressActivation{executor: e, events: events, phase: phase, label: label}
	return newProgressStreamWriter(stdout, activate), newProgressStreamWriter(stderr, activate)
}

type progressActivation struct {
	executor *Executor
	events   ui.Sink
	phase    string
	label    string
	once     sync.Once
}

func (a *progressActivation) Trigger() {
	a.once.Do(func() {
		a.executor.clearProgress(a.events)
		a.events.Emit(ui.Event{Kind: ui.EventProgressOutput, Phase: a.phase, Message: a.label})
	})
}

type progressStreamWriter struct {
	writer io.Writer
	start  *progressActivation
}

func newProgressStreamWriter(writer io.Writer, start *progressActivation) io.Writer {
	if writer == nil {
		return nil
	}
	return &progressStreamWriter{writer: writer, start: start}
}

func (w *progressStreamWriter) Write(p []byte) (int, error) {
	if len(p) > 0 && w.start != nil {
		w.start.Trigger()
	}
	return w.writer.Write(p)
}
