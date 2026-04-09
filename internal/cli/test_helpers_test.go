package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

type appHarness struct {
	t      testing.TB
	app    *App
	out    bytes.Buffer
	errOut bytes.Buffer
}

func newAppHarness(t testing.TB, service stubService) *appHarness {
	t.Helper()
	isolateConfigHome(t)
	h := &appHarness{t: t}
	h.app = NewWithService(&h.out, &h.errOut, service)
	return h
}

func (h *appHarness) run(args ...string) error {
	h.t.Helper()
	return h.app.Run(context.Background(), args)
}

func (h *appHarness) stdout() string {
	h.t.Helper()
	return h.out.String()
}

func (h *appHarness) stderr() string {
	h.t.Helper()
	return h.errOut.String()
}

func isolateConfigHome(t testing.TB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func assertContainsAll(t testing.TB, got string, want ...string) {
	t.Helper()
	for _, part := range want {
		if !strings.Contains(got, part) {
			t.Fatalf("expected output to contain %q, got %q", part, got)
		}
	}
}
