package plan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

func BenchmarkBuildWorkspacePlan(b *testing.B) {
	workspace := b.TempDir()
	configDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		b.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"alpine:3.23","workspaceFolder":"/workspaces/demo"}`), 0o644); err != nil {
		b.Fatalf("write config: %v", err)
	}
	req := BuildWorkspacePlanRequest{
		Workspace:      workspace,
		ConfigPath:     configPath,
		FeatureTimeout: 90 * time.Second,
		LockfilePolicy: devcontainer.FeatureLockfilePolicyAuto,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan, err := BuildWorkspacePlan(req)
		if err != nil {
			b.Fatalf("build workspace plan: %v", err)
		}
		if !plan.Valid() {
			b.Fatal("expected valid workspace plan")
		}
	}
}
