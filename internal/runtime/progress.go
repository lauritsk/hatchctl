package runtime

import (
	"io"
	"sync"

	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/docker"
)

func (r *Runner) progressDockerRunOptions(events ui.Sink, label string, opts docker.RunOptions) docker.RunOptions {
	stdout, stderr := r.progressWriters(events, label, opts.Stdout, opts.Stderr)
	opts.Stdout = stdout
	opts.Stderr = stderr
	return opts
}

func (r *Runner) progressCommandIO(events ui.Sink, label string, streams commandIO) commandIO {
	stdout, stderr := r.progressWriters(events, label, streams.Stdout, streams.Stderr)
	streams.Stdout = stdout
	streams.Stderr = stderr
	return streams
}

func (r *Runner) progressWriters(events ui.Sink, label string, stdout io.Writer, stderr io.Writer) (io.Writer, io.Writer) {
	if events == nil || label == "" {
		return stdout, stderr
	}
	activate := &progressActivation{
		runner: r,
		events: events,
		label:  label,
	}
	return newProgressStreamWriter(stdout, activate), newProgressStreamWriter(stderr, activate)
}

type progressActivation struct {
	runner *Runner
	events ui.Sink
	label  string
	once   sync.Once
}

func (a *progressActivation) Trigger() {
	a.once.Do(func() {
		a.runner.clearProgress(a.events)
		a.events.Emit(ui.Event{Kind: ui.EventProgressOutput, Message: a.label})
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
