package uidremap

import (
	"reflect"
	"testing"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/docker"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func TestEligibleRequiresNamedNonRootRemoteUser(t *testing.T) {
	t.Parallel()

	user, ok := Eligible(devcontainer.ResolvedConfig{Merged: spec.MergedConfig{RemoteUser: "vscode"}}, docker.ImageInspect{})
	if !ok || user != "vscode" {
		t.Fatalf("expected named remote user to be eligible, got user=%q ok=%t", user, ok)
	}
	for _, resolved := range []devcontainer.ResolvedConfig{
		{Merged: spec.MergedConfig{RemoteUser: "root"}},
		{Merged: spec.MergedConfig{RemoteUser: "1000"}},
		{Merged: spec.MergedConfig{UpdateRemoteUserUID: boolPtr(false), RemoteUser: "vscode"}},
	} {
		if _, ok := Eligible(resolved, docker.ImageInspect{}); ok {
			t.Fatalf("expected %#v to be ineligible", resolved)
		}
	}
}

func TestExecArgsRunsRemapScriptAsRoot(t *testing.T) {
	t.Parallel()

	got := ExecArgs("container-123", "vscode", 1001, 1002)
	want := []string{"exec", "-i", "-u", "root", "container-123", "sh", "-s", "--", "vscode", "1001", "1002"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected exec args %#v", got)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
