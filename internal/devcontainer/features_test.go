package devcontainer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	features, err := ResolveFeatures(configDir, t.TempDir(), map[string]any{
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

	features, err := ResolveFeatures(configDir, t.TempDir(), map[string]any{
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

func TestResolveFeaturesFetchesOCIRegistryFeature(t *testing.T) {
	layer := buildFeatureLayer(t, map[string]string{
		"devcontainer-feature.json": `{"id":"remote-tool","containerEnv":{"REMOTE":"yes"}}`,
		"install.sh":                "#!/bin/sh\nexit 0\n",
	})
	server := newFeatureRegistryServer(t, layer)
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")

	features, err := ResolveFeatures(t.TempDir(), t.TempDir(), map[string]any{
		registryHost + "/features/remote-tool:1": true,
	})
	if err != nil {
		t.Fatalf("resolve oci feature: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("expected one feature, got %d", len(features))
	}
	if got := features[0].Metadata.ID; got != "remote-tool" {
		t.Fatalf("unexpected feature id %q", got)
	}
	if got := features[0].Metadata.ContainerEnv["REMOTE"]; got != "yes" {
		t.Fatalf("unexpected feature env %#v", features[0].Metadata.ContainerEnv)
	}
	if _, err := os.Stat(filepath.Join(features[0].Path, "install.sh")); err != nil {
		t.Fatalf("expected extracted feature install script: %v", err)
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

func buildFeatureLayer(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, contents := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func newFeatureRegistryServer(t *testing.T, layer []byte) *httptest.Server {
	t.Helper()
	const token = "test-token"
	digest := "sha256:test-layer"
	manifestBody, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"config":        map[string]any{"digest": "sha256:test-config"},
		"layers": []map[string]any{{
			"mediaType": "application/vnd.devcontainers.layer.v1+tar+gzip",
			"digest":    digest,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/token"):
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
		case strings.HasSuffix(r.URL.Path, "/manifests/1"):
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.Header().Set("Www-Authenticate", `Bearer realm="`+serverURLFromRequest(r)+`/token",service="registry.test",scope="repository:features/remote-tool:pull"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Docker-Content-Digest", "sha256:test-manifest")
			_, _ = w.Write(manifestBody)
		case strings.HasSuffix(r.URL.Path, "/blobs/"+digest):
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.Header().Set("Www-Authenticate", `Bearer realm="`+serverURLFromRequest(r)+`/token",service="registry.test",scope="repository:features/remote-tool:pull"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write(layer)
		default:
			http.NotFound(w, r)
		}
	}))
}

func serverURLFromRequest(r *http.Request) string {
	return "http://" + r.Host
}
