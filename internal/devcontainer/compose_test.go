package devcontainer

import "testing"

func TestParseMountSpecSupportsAliasesAndOptions(t *testing.T) {
	t.Parallel()

	spec, ok := ParseMountSpec("type=bind,src=/workspace,dst=/workspaces/demo,ro=1,bind-propagation=rshared,create-host-path=false")
	if !ok {
		t.Fatal("expected mount spec to parse")
	}
	if spec.Type != "bind" || spec.Source != "/workspace" || spec.Target != "/workspaces/demo" {
		t.Fatalf("unexpected parsed mount %#v", spec)
	}
	if !spec.ReadOnly || spec.BindPropagation != "rshared" {
		t.Fatalf("unexpected mount options %#v", spec)
	}
	if spec.CreateHostPath == nil || *spec.CreateHostPath {
		t.Fatalf("expected create-host-path=false, got %#v", spec.CreateHostPath)
	}
}
