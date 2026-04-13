package bridge

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lauritsk/hatchctl/internal/spec"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func TestReportFromSession(t *testing.T) {
	t.Parallel()

	if ReportFromSession(nil) != nil {
		t.Fatal("expected nil session to return nil report")
	}
	session := &Session{
		ID:         "bridge-session",
		Backend:    "docker",
		Enabled:    true,
		HelperArch: "arm64",
		Host:       "host.docker.internal",
		Port:       41000,
		StatePath:  "/state/bridge",
		ConfigPath: "/state/bridge/bridge-config.json",
		PIDPath:    "/state/bridge/bridge.pid",
		StatusPath: "/state/bridge/bridge-status.json",
		HelperPath: "/state/bridge/bin/devcontainer-open",
		MountPath:  "/var/run/hatchctl/bridge",
		BinPath:    "/var/run/hatchctl/bridge/bin",
		Status:     "running",
	}
	report := ReportFromSession(session)
	if report == nil || report.ID != session.ID || report.Backend != session.Backend || report.HelperPath != session.HelperPath || report.Status != "running" {
		t.Fatalf("unexpected report %#v", report)
	}
}

func TestPreviewReturnsNilWhenDisabledOrMissing(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	session, err := Preview(stateDir, false)
	if err != nil {
		t.Fatalf("preview disabled bridge: %v", err)
	}
	if session != nil {
		t.Fatalf("expected disabled preview to return nil, got %#v", session)
	}

	session, err = Preview(stateDir, true)
	if err != nil {
		t.Fatalf("preview missing session: %v", err)
	}
	if session != nil {
		t.Fatalf("expected missing session to return nil, got %#v", session)
	}
}

func TestInjectAddsBridgeEnvAndMountOnce(t *testing.T) {
	t.Parallel()

	session := &Session{StatePath: "/state/bridge", MountPath: "/var/run/hatchctl/bridge", BinPath: "/var/run/hatchctl/bridge/bin"}
	merged := spec.MergedConfig{
		ContainerEnv: map[string]string{"PATH": "/usr/bin"},
		Mounts:       []string{"type=bind,source=/state/bridge,target=/var/run/hatchctl/bridge"},
	}
	got := Inject(session, merged)
	if got.ContainerEnv["BROWSER"] != "/var/run/hatchctl/bridge/bin/devcontainer-open" {
		t.Fatalf("unexpected browser env %#v", got.ContainerEnv)
	}
	if got.ContainerEnv["DEVCONTAINER_BRIDGE_ENABLED"] != "true" {
		t.Fatalf("expected bridge env to be injected, got %#v", got.ContainerEnv)
	}
	if !strings.HasPrefix(got.ContainerEnv["PATH"], "/var/run/hatchctl/bridge/bin:") {
		t.Fatalf("expected bridge bin path to be prepended, got %q", got.ContainerEnv["PATH"])
	}
	if len(got.Mounts) != 1 {
		t.Fatalf("expected duplicate mount to be avoided, got %#v", got.Mounts)
	}
}

func TestDoctorReportsPersistedStatus(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	paths, err := storefs.EnsureWorkspaceBridgePaths(stateDir)
	if err != nil {
		t.Fatalf("ensure bridge paths: %v", err)
	}
	helperPath := filepath.Join(paths.BinDir, "devcontainer-open")
	if err := storefs.WriteBridgeExecutable(helperPath, []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := storefs.WriteBridgeSession(paths.Dir, Session{
		ID:         "bridge-session",
		Backend:    "docker",
		Enabled:    true,
		HelperArch: "arm64",
		Host:       "host.docker.internal",
		Port:       43123,
		StatePath:  paths.Dir,
		ConfigPath: paths.ConfigPath,
		PIDPath:    paths.PIDPath,
		StatusPath: paths.StatusPath,
		HelperPath: helperPath,
		MountPath:  containerBridgeMountPath,
		BinPath:    "/var/run/hatchctl/bridge/bin",
		Status:     "scaffolded",
	}); err != nil {
		t.Fatalf("write bridge session: %v", err)
	}
	if err := storefs.WriteBridgeStatus(paths.StatusPath, map[string]any{"lastEvent": "bridge ready"}); err != nil {
		t.Fatalf("write bridge status: %v", err)
	}

	report, err := Doctor(stateDir)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !report.Enabled || report.Status != "bridge ready" {
		t.Fatalf("unexpected bridge report %#v", report)
	}
	if report.ID != "bridge-session" || report.Backend != "docker" || report.Port != 43123 || report.HelperPath != helperPath {
		t.Fatalf("unexpected bridge report details %#v", report)
	}
}
