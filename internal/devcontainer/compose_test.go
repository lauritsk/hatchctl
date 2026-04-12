package devcontainer

import (
	"testing"

	"github.com/lauritsk/hatchctl/internal/spec"
)

func TestParseMountSpecSupportsAliasesAndOptions(t *testing.T) {
	t.Parallel()

	mountSpec, ok := spec.ParseMountSpec("type=bind,src=/workspace,dst=/workspaces/demo,ro=1,bind-propagation=rshared,create-host-path=false")
	if !ok {
		t.Fatal("expected mount spec to parse")
	}
	if mountSpec.Type != "bind" || mountSpec.Source != "/workspace" || mountSpec.Target != "/workspaces/demo" {
		t.Fatalf("unexpected parsed mount %#v", mountSpec)
	}
	if !mountSpec.ReadOnly || mountSpec.BindPropagation != "rshared" {
		t.Fatalf("unexpected mount options %#v", mountSpec)
	}
	if mountSpec.CreateHostPath == nil || *mountSpec.CreateHostPath {
		t.Fatalf("expected create-host-path=false, got %#v", mountSpec.CreateHostPath)
	}
}
