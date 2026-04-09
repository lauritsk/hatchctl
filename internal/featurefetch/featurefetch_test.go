package featurefetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

func TestIsLoopbackHostAcceptsLoopbackLiterals(t *testing.T) {
	t.Parallel()

	if !isLoopbackHost("localhost") || !isLoopbackHost("127.0.0.1") {
		t.Fatal("expected loopback hosts to be accepted")
	}
}

func TestValidateTarballRedirectRejectsNonLoopbackHTTPTargets(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "http://example.com/feature.tgz", nil)
	if err != nil {
		t.Fatal(err)
	}
	viaReq, err := http.NewRequest(http.MethodGet, "https://example.com/feature.tgz", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateTarballRedirect(req, []*http.Request{viaReq}); err == nil || !strings.Contains(err.Error(), "must use https or loopback http") {
		t.Fatalf("expected redirect validation error, got %v", err)
	}
}

func TestFetchRegistryBearerTokenRejectsUnexpectedRealmHost(t *testing.T) {
	t.Parallel()

	_, err := fetchRegistryBearerToken(context.Background(), "https://registry.example/v2/features/tool/manifests/latest", `Bearer realm="https://evil.example/token",service="registry.test"`, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "unexpected host") {
		t.Fatalf("expected unexpected host error, got %v", err)
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
	t.Parallel()

	dstDir := t.TempDir()
	parentDir := filepath.Dir(dstDir)
	layer := buildFeatureLayerEntries(t, []featureLayerEntry{
		{Name: "ok.txt", Contents: "ok"},
		{Name: "../outside.txt", Contents: "outside"},
		{Name: "/absolute.txt", Contents: "absolute"},
		{Name: "nested/../normalized.txt", Contents: "normalized"},
	})

	if err := ExtractFeatureLayer(bytes.NewReader(layer), dstDir); err != nil {
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

func TestExtractFeatureLayerRejectsArchiveBombs(t *testing.T) {
	previousFiles := featureExtractedMaxFiles
	previousFileBytes := featureExtractedMaxFileBytes
	previousTotalBytes := featureExtractedMaxBytes
	featureExtractedMaxFiles = 1
	featureExtractedMaxFileBytes = 8
	featureExtractedMaxBytes = 8
	t.Cleanup(func() {
		featureExtractedMaxFiles = previousFiles
		featureExtractedMaxFileBytes = previousFileBytes
		featureExtractedMaxBytes = previousTotalBytes
	})

	tooManyFiles := buildFeatureLayerEntries(t, []featureLayerEntry{{Name: "one.txt", Contents: "1"}, {Name: "two.txt", Contents: "2"}})
	if err := ExtractFeatureLayer(bytes.NewReader(tooManyFiles), t.TempDir()); err == nil || !strings.Contains(err.Error(), "exceeds 1 files") {
		t.Fatalf("expected file-count limit error, got %v", err)
	}

	tooLargeFile := buildFeatureLayerEntries(t, []featureLayerEntry{{Name: "big.txt", Contents: "123456789"}})
	if err := ExtractFeatureLayer(bytes.NewReader(tooLargeFile), t.TempDir()); err == nil || !strings.Contains(err.Error(), "exceeds 8 bytes") {
		t.Fatalf("expected file-size limit error, got %v", err)
	}
}

func TestResolveSourceFetchesDeprecatedGitHubShorthandFeature(t *testing.T) {
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

	resolved, err := ResolveSource(context.Background(), t.TempDir(), t.TempDir(), "owner/repo/feature@v1.2.3", storefs.FeatureLockEntry{}, "auto", ResolveOptions{AllowNetwork: true})
	if err != nil {
		t.Fatalf("resolve github shorthand feature: %v", err)
	}
	if resolved.Kind != "github-release" {
		t.Fatalf("unexpected github shorthand source %#v", resolved)
	}
	if _, err := os.Stat(filepath.Join(resolved.Path, "devcontainer-feature.json")); err != nil {
		t.Fatalf("expected extracted feature manifest: %v", err)
	}
	if resolved.Resolved != server.URL+"/owner/repo/releases/download/v1.2.3/feature.tgz" {
		t.Fatalf("unexpected resolved source %q", resolved.Resolved)
	}
}

func TestResolveSourceHonorsContextTimeoutForTarballs(t *testing.T) {
	previousTimeout := featureHTTPTimeout
	featureHTTPTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		featureHTTPTimeout = previousTimeout
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := ResolveSource(ctx, t.TempDir(), t.TempDir(), server.URL+"/feature.tgz", storefs.FeatureLockEntry{}, "auto", ResolveOptions{AllowNetwork: true})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("unexpected timeout error %v", err)
	}
}

func TestResolveSourceRejectsOversizedTarballs(t *testing.T) {
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
	_, err := ResolveSource(context.Background(), t.TempDir(), cacheDir, server.URL+"/feature.tgz", storefs.FeatureLockEntry{}, "auto", ResolveOptions{AllowNetwork: true})
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
