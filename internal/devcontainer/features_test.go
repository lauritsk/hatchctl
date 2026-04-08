package devcontainer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var featureResolveOpts = FeatureResolveOptions{AllowNetwork: true, WriteLockFile: true}

func TestResolveFeaturesFrozenLockfileRequiresPinnedRemoteFeature(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".devcontainer.json")
	_, err := ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{
		"ghcr.io/devcontainers/features/go:1": true,
	}, FeatureResolveOptions{AllowNetwork: true, LockfilePolicy: FeatureLockfilePolicyFrozen})
	if err == nil || !strings.Contains(err.Error(), "requires a lockfile integrity in frozen lockfile mode") {
		t.Fatalf("expected frozen lockfile error, got %v", err)
	}
}

func TestResolveFeaturesUpdateLockfileRequiresNetwork(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".devcontainer.json")
	_, err := ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{
		"ghcr.io/devcontainers/features/go:1": true,
	}, FeatureResolveOptions{LockfilePolicy: FeatureLockfilePolicyUpdate})
	if err == nil || !strings.Contains(err.Error(), "requires network access in update lockfile mode") {
		t.Fatalf("expected update lockfile network error, got %v", err)
	}
}

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

	configPath := filepath.Join(configDir, "devcontainer.json")
	features, err := ResolveFeatures(context.Background(), configPath, configDir, t.TempDir(), map[string]any{
		"./gamma": true,
		"./beta":  true,
		"./alpha": true,
	}, featureResolveOpts)
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

	configPath := filepath.Join(configDir, "devcontainer.json")
	features, err := ResolveFeatures(context.Background(), configPath, configDir, t.TempDir(), map[string]any{
		"./tool": map[string]any{"other-option": true},
	}, featureResolveOpts)
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
	server, requests := newFeatureRegistryServer(t, layer)
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	configPath := filepath.Join(t.TempDir(), ".devcontainer", "devcontainer.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}

	features, err := ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{
		registryHost + "/features/remote-tool:1": true,
	}, featureResolveOpts)
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
	lockData, err := os.ReadFile(FeatureLockFilePath(configPath))
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	if !strings.Contains(string(lockData), registryHost+`/features/remote-tool:1`) || !strings.Contains(string(lockData), `"integrity": "sha256:test-manifest"`) {
		t.Fatalf("unexpected oci lockfile %s", string(lockData))
	}
	manifestRequests := *requests
	if manifestRequests["/v2/features/remote-tool/manifests/sha256:test-manifest"] != 0 {
		t.Fatalf("unexpected digest request on first resolve %#v", manifestRequests)
	}
	_, err = ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{
		registryHost + "/features/remote-tool:1": true,
	}, featureResolveOpts)
	if err != nil {
		t.Fatalf("resolve oci feature with lockfile: %v", err)
	}
	if (*requests)["/v2/features/remote-tool/manifests/sha256:test-manifest"] == 0 {
		t.Fatalf("expected digest-pinned manifest request, got %#v", *requests)
	}
}

func TestResolveFeaturesFetchesTarballFeatureAndPinsIntegrity(t *testing.T) {
	layer := buildFeatureLayer(t, map[string]string{
		"devcontainer-feature.json": `{"id":"tarball-tool","containerEnv":{"TARBALL":"yes"}}`,
		"install.sh":                "#!/bin/sh\nexit 0\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devcontainer-feature-tarball-tool.tgz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(layer)
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), ".devcontainer.json")
	featureURL := server.URL + "/devcontainer-feature-tarball-tool.tgz"

	features, err := ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{featureURL: true}, featureResolveOpts)
	if err != nil {
		t.Fatalf("resolve tarball feature: %v", err)
	}
	if len(features) != 1 || features[0].SourceKind != "direct-tarball" {
		t.Fatalf("unexpected tarball features %#v", features)
	}
	lockData, err := os.ReadFile(FeatureLockFilePath(configPath))
	if err != nil {
		t.Fatalf("read tarball lockfile: %v", err)
	}
	if !strings.Contains(string(lockData), featureURL) || !strings.Contains(string(lockData), `"integrity": "sha256:`) {
		t.Fatalf("unexpected tarball lockfile %s", string(lockData))
	}
	badIntegrity := fmt.Sprintf(`{"%s":{"resolved":"%s","integrity":"sha256:bad"}}`, featureURL, featureURL)
	if err := os.WriteFile(FeatureLockFilePath(configPath), []byte(badIntegrity), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{featureURL: true}, featureResolveOpts); err == nil || !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("expected tarball integrity mismatch, got %v", err)
	}
}

func TestResolveFeaturesFetchesDeprecatedGitHubShorthandFeature(t *testing.T) {
	layer := buildFeatureLayer(t, map[string]string{
		"devcontainer-feature.json": `{"id":"gh-tool","containerEnv":{"GITHUB":"yes"}}`,
		"install.sh":                "#!/bin/sh\nexit 0\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/owner/repo/releases/download/v1.2.3/feature.tgz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(layer)
	}))
	defer server.Close()

	previousBaseURL := githubReleaseBaseURL
	githubReleaseBaseURL = server.URL
	t.Cleanup(func() {
		githubReleaseBaseURL = previousBaseURL
	})

	configPath := filepath.Join(t.TempDir(), ".devcontainer.json")
	features, err := ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{"owner/repo/feature@v1.2.3": true}, featureResolveOpts)
	if err != nil {
		t.Fatalf("resolve github shorthand feature: %v", err)
	}
	if len(features) != 1 || features[0].SourceKind != "github-release" {
		t.Fatalf("unexpected github shorthand features %#v", features)
	}
	if features[0].Metadata.ContainerEnv["GITHUB"] != "yes" {
		t.Fatalf("unexpected feature metadata %#v", features[0].Metadata)
	}
	lockData, err := os.ReadFile(FeatureLockFilePath(configPath))
	if err != nil {
		t.Fatalf("read github shorthand lockfile: %v", err)
	}
	if !strings.Contains(string(lockData), `"resolved": "`+server.URL+`/owner/repo/releases/download/v1.2.3/feature.tgz"`) {
		t.Fatalf("unexpected github shorthand lockfile %s", string(lockData))
	}
}

func TestResolveFeaturesHonorsContextTimeoutForTarballs(t *testing.T) {
	previousTimeout := featureHTTPTimeout
	featureHTTPTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		featureHTTPTimeout = previousTimeout
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	configPath := filepath.Join(t.TempDir(), ".devcontainer.json")
	_, err := ResolveFeatures(ctx, configPath, filepath.Dir(configPath), t.TempDir(), map[string]any{server.URL + "/feature.tgz": true}, featureResolveOpts)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("unexpected timeout error %v", err)
	}
}

func TestResolveFeaturesRejectsOversizedTarballs(t *testing.T) {
	previousLimit := featureArtifactMaxBytes
	featureArtifactMaxBytes = 32
	t.Cleanup(func() {
		featureArtifactMaxBytes = previousLimit
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "64")
		_, _ = w.Write(bytes.Repeat([]byte("x"), 64))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), ".devcontainer.json")
	_, err := ResolveFeatures(context.Background(), configPath, filepath.Dir(configPath), cacheDir, map[string]any{server.URL + "/feature.tgz": true}, featureResolveOpts)
	if err == nil || !strings.Contains(err.Error(), "feature tarball exceeds 32 bytes") {
		t.Fatalf("expected oversized tarball error, got %v", err)
	}
	entries, readErr := os.ReadDir(cacheDir)
	if readErr != nil {
		t.Fatalf("read cache dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no cached artifacts after oversized tarball, got %d entries", len(entries))
	}
}

func TestParseFeatureSourceClassifiesInputs(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	localFeature := filepath.Join(configDir, "tool")
	if err := os.MkdirAll(localFeature, 0o755); err != nil {
		t.Fatalf("mkdir local feature: %v", err)
	}

	local, err := parseFeatureSource(configDir, "./tool")
	if err != nil || local.Kind != "file-path" {
		t.Fatalf("expected local feature classification, got %#v err=%v", local, err)
	}
	tarball, err := parseFeatureSource(configDir, "https://example.com/feature.tgz")
	if err != nil || tarball.Kind != "direct-tarball" {
		t.Fatalf("expected tarball classification, got %#v err=%v", tarball, err)
	}
	github, err := parseFeatureSource(configDir, "owner/repo/feature@v1.2.3")
	if err != nil || github.Kind != "github-release" {
		t.Fatalf("expected github classification, got %#v err=%v", github, err)
	}
	oci, err := parseFeatureSource(configDir, "ghcr.io/devcontainers/features/go:1")
	if err != nil || oci.Kind != "oci" {
		t.Fatalf("expected oci classification, got %#v err=%v", oci, err)
	}
}

func TestExtractFeatureLayerSkipsNonLocalAndDotDotArchivePaths(t *testing.T) {
	dstDir := t.TempDir()
	parentDir := filepath.Dir(dstDir)
	layer := buildFeatureLayerEntries(t, []featureLayerEntry{
		{Name: "ok.txt", Contents: "ok"},
		{Name: "../outside.txt", Contents: "outside"},
		{Name: "/absolute.txt", Contents: "absolute"},
		{Name: "nested/../normalized.txt", Contents: "normalized"},
	})

	if err := extractFeatureLayer(bytes.NewReader(layer), dstDir); err != nil {
		t.Fatalf("extract feature layer: %v", err)
	}

	if data, err := os.ReadFile(filepath.Join(dstDir, "ok.txt")); err != nil || string(data) != "ok" {
		t.Fatalf("expected ok.txt to be extracted, got %q, %v", string(data), err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "normalized.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected normalized path with .. segment to be skipped, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(parentDir, "outside.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected parent traversal target to be skipped, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "absolute.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected absolute archive path to be skipped, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(parentDir, "absolute.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected absolute archive path outside destination to be skipped, got %v", err)
	}
	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatalf("read destination entries: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "ok.txt" {
		t.Fatalf("unexpected extracted entries %#v", entries)
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
	entries := make([]featureLayerEntry, 0, len(files))
	for name, contents := range files {
		entries = append(entries, featureLayerEntry{Name: name, Contents: contents})
	}
	return buildFeatureLayerEntries(t, entries)
}

type featureLayerEntry struct {
	Name     string
	Contents string
}

func buildFeatureLayerEntries(t *testing.T, entries []featureLayerEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		if err := tarWriter.WriteHeader(&tar.Header{Name: entry.Name, Mode: 0o755, Size: int64(len(entry.Contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(entry.Contents)); err != nil {
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

func newFeatureRegistryServer(t *testing.T, layer []byte) (*httptest.Server, *map[string]int) {
	t.Helper()
	const token = "test-token"
	digest := "sha256:test-layer"
	requests := map[string]int{}
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch {
		case strings.HasPrefix(r.URL.Path, "/token"):
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
		case strings.HasSuffix(r.URL.Path, "/manifests/1") || strings.HasSuffix(r.URL.Path, "/manifests/sha256:test-manifest"):
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
	return server, &requests
}

func serverURLFromRequest(r *http.Request) string {
	return "http://" + r.Host
}
