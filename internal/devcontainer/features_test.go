package devcontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFeaturesOrdersDependenciesAndInstallsAfter(t *testing.T) {
	configDir := t.TempDir()
	writeFeatureFixture(t, filepath.Join(configDir, "alpha"), `{
		"id": "alpha",
		"mounts": ["type=volume,source=alpha,target=/workspace-tools"]
	}`)
	writeFeatureFixture(t, filepath.Join(configDir, "beta"), `{
		"id": "beta",
		"dependsOn": {"alpha": true},
		"mounts": ["type=volume,source=beta,target=/workspace-tools"]
	}`)
	writeFeatureFixture(t, filepath.Join(configDir, "gamma"), `{
		"id": "gamma",
		"installsAfter": ["beta"]
	}`)

	features, err := ResolveFeatures(configDir, map[string]any{
		"./gamma": true,
		"./beta":  true,
		"./alpha": true,
	})
	if err != nil {
		t.Fatalf("resolve features: %v", err)
	}
	if got := strings.Join(featureIDs(features), ","); got != "alpha,beta,gamma" {
		t.Fatalf("unexpected feature order %q", got)
	}
	merged := MergeMetadata(Config{}, featureMetadataForTest(features))
	if got := strings.Join(merged.Mounts, ","); got != "type=volume,source=beta,target=/workspace-tools" {
		t.Fatalf("unexpected merged mounts %q", got)
	}
}

func TestResolveFeaturesMaterializesOptionEnvironment(t *testing.T) {
	configDir := t.TempDir()
	writeFeatureFixture(t, filepath.Join(configDir, "tool"), `{
		"id": "tool",
		"options": {
			"version": {"default": "stable"},
			"other-option": {"default": false}
		}
	}`)

	features, err := ResolveFeatures(configDir, map[string]any{
		"./tool": map[string]any{"other-option": true},
	})
	if err != nil {
		t.Fatalf("resolve features: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("expected one feature, got %d", len(features))
	}
	if got := features[0].Options["VERSION"]; got != "stable" {
		t.Fatalf("unexpected VERSION option %q", got)
	}
	if got := features[0].Options["OTHER_OPTION"]; got != "true" {
		t.Fatalf("unexpected OTHER_OPTION %q", got)
	}
	if got := featureOptionEnvName("1bad-name"); got != "_1BAD_NAME" {
		t.Fatalf("unexpected env name %q", got)
	}
}

func writeFeatureFixture(t *testing.T, dir string, manifest string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devcontainer-feature.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func featureIDs(features []ResolvedFeature) []string {
	ids := make([]string, 0, len(features))
	for _, feature := range features {
		ids = append(ids, feature.Metadata.ID)
	}
	return ids
}

func featureMetadataForTest(features []ResolvedFeature) []MetadataEntry {
	entries := make([]MetadataEntry, 0, len(features))
	for _, feature := range features {
		entries = append(entries, feature.Metadata)
	}
	return entries
}
