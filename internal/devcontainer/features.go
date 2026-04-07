package devcontainer

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lauritsk/hatchctl/internal/security"
	"github.com/tailscale/hujson"
)

var (
	featureHTTPTimeout             = 30 * time.Second
	featureArtifactMaxBytes  int64 = 64 << 20
	featureMetadataMaxBytes  int64 = 2 << 20
	featureErrorBodyMaxBytes int64 = 64 << 10
)

type ResolvedFeature struct {
	SourceKind    string
	Source        string
	Path          string
	Version       string
	Resolved      string
	Integrity     string
	Options       map[string]string
	DependsOn     []string
	InstallsAfter []string
	Metadata      MetadataEntry
}

type FeatureResolveOptions struct {
	AllowNetwork   bool
	WriteLockFile  bool
	WriteStateFile bool
	StateDir       string
	LockfilePolicy FeatureLockfilePolicy
}

var githubReleaseBaseURL = "https://github.com"

type featureManifest struct {
	ID                   string                   `json:"id"`
	ContainerEnv         map[string]string        `json:"containerEnv,omitempty"`
	Mounts               []string                 `json:"mounts,omitempty"`
	Init                 *bool                    `json:"init,omitempty"`
	Privileged           *bool                    `json:"privileged,omitempty"`
	CapAdd               []string                 `json:"capAdd,omitempty"`
	SecurityOpt          []string                 `json:"securityOpt,omitempty"`
	Customizations       map[string]any           `json:"customizations,omitempty"`
	OnCreateCommand      LifecycleCommand         `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleCommand         `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleCommand         `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleCommand         `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleCommand         `json:"postAttachCommand,omitempty"`
	InstallsAfter        []string                 `json:"installsAfter,omitempty"`
	DependsOn            map[string]any           `json:"dependsOn,omitempty"`
	Options              map[string]featureOption `json:"options,omitempty"`
}

type featureOption struct {
	Default any `json:"default,omitempty"`
}

type featureSource struct {
	Kind string
	Path string
	OCI  ociReference
}

type ociReference struct {
	Registry   string
	Repository string
	Reference  string
	Insecure   bool
}

type ociManifest struct {
	SchemaVersion int `json:"schemaVersion"`
	Config        struct {
		Digest string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

func ResolveFeatures(ctx context.Context, configPath string, configDir string, cacheDir string, values map[string]any, opts FeatureResolveOptions) ([]ResolvedFeature, error) {
	if len(values) == 0 {
		return nil, nil
	}
	policy, err := ParseFeatureLockfilePolicy(string(opts.LockfilePolicy))
	if err != nil {
		return nil, err
	}
	lockFile, _, err := ReadFeatureLockFile(configPath)
	if err != nil {
		return nil, err
	}
	features := make([]ResolvedFeature, 0, len(values))
	byAlias := map[string]int{}
	for source, raw := range values {
		options, enabled := featureValueOptions(raw)
		if !enabled {
			continue
		}
		if err := validateFeatureLockfilePolicy(source, lockFile[source], policy); err != nil {
			return nil, err
		}
		featurePath, kind, resolvedRef, integrity, version, err := resolveFeaturePath(ctx, configDir, cacheDir, source, lockFile[source], opts.AllowNetwork, policy)
		if err != nil {
			return nil, err
		}
		manifest, err := loadFeatureManifest(featurePath)
		if err != nil {
			return nil, fmt.Errorf("load feature %q: %w", source, err)
		}
		if manifest.ID == "" {
			return nil, fmt.Errorf("load feature %q: missing id in devcontainer-feature.json", source)
		}
		feature := ResolvedFeature{
			SourceKind:    kind,
			Source:        source,
			Path:          featurePath,
			Version:       version,
			Resolved:      resolvedRef,
			Integrity:     integrity,
			Options:       materializeFeatureOptions(manifest, options),
			DependsOn:     sortedKeys(manifest.DependsOn),
			InstallsAfter: slices.Clone(manifest.InstallsAfter),
			Metadata: MetadataEntry{
				ID:                   manifest.ID,
				Init:                 manifest.Init,
				Privileged:           manifest.Privileged,
				CapAdd:               cloneSlice(manifest.CapAdd),
				SecurityOpt:          cloneSlice(manifest.SecurityOpt),
				Mounts:               cloneSlice(manifest.Mounts),
				ContainerEnv:         cloneMap(manifest.ContainerEnv),
				Customizations:       manifest.Customizations,
				OnCreateCommand:      manifest.OnCreateCommand,
				UpdateContentCommand: manifest.UpdateContentCommand,
				PostCreateCommand:    manifest.PostCreateCommand,
				PostStartCommand:     manifest.PostStartCommand,
				PostAttachCommand:    manifest.PostAttachCommand,
			},
		}
		features = append(features, feature)
		idx := len(features) - 1
		byAlias[source] = idx
		byAlias[manifest.ID] = idx
	}
	if opts.WriteLockFile && policy != FeatureLockfilePolicyFrozen {
		if err := WriteFeatureLockFile(configPath, features); err != nil {
			return nil, err
		}
	}
	if opts.WriteStateFile {
		if err := WriteFeatureStateFile(opts.StateDir, features); err != nil {
			return nil, err
		}
	}
	return orderFeatures(features, byAlias)
}

func resolveFeaturePath(ctx context.Context, configDir string, cacheDir string, source string, lock FeatureLockEntry, allowNetwork bool, policy FeatureLockfilePolicy) (string, string, string, string, string, error) {
	resolved, err := resolveLocalFeaturePath(configDir, source)
	if err == nil {
		return resolved, "file-path", source, "", "", nil
	}
	if !isMissingPathError(err) {
		return "", "", "", "", "", err
	}
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		if policy == FeatureLockfilePolicyUpdate {
			if !allowNetwork {
				return "", "", "", "", "", fmt.Errorf("feature %q requires network access in update lockfile mode", source)
			}
			resolved, integrity, err := fetchTarballFeature(ctx, cacheDir, source, lock)
			return resolved, "direct-tarball", source, integrity, source, err
		}
		if !allowNetwork {
			resolved, integrity, err := cachedTarballFeature(cacheDir, source, lock)
			return resolved, "direct-tarball", source, integrity, source, err
		}
		resolved, integrity, err := fetchTarballFeature(ctx, cacheDir, source, lock)
		return resolved, "direct-tarball", source, integrity, source, err
	}
	if githubRef, err := parseGitHubFeatureReference(source); err == nil {
		resolvedSource := githubRef.tarballURL()
		if policy == FeatureLockfilePolicyUpdate {
			if !allowNetwork {
				return "", "", "", "", "", fmt.Errorf("feature %q requires network access in update lockfile mode", source)
			}
			resolved, integrity, err := fetchTarballFeature(ctx, cacheDir, resolvedSource, lock)
			return resolved, "github-release", resolvedSource, integrity, githubRef.version(), err
		}
		if !allowNetwork {
			resolved, integrity, err := cachedTarballFeature(cacheDir, resolvedSource, lock)
			return resolved, "github-release", resolvedSource, integrity, githubRef.version(), err
		}
		resolved, integrity, err := fetchTarballFeature(ctx, cacheDir, resolvedSource, lock)
		return resolved, "github-release", resolvedSource, integrity, githubRef.version(), err
	}
	ref, err := parseOCIReference(source)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("feature %q not found locally and is not a valid remote feature source: %w", source, err)
	}
	if policy == FeatureLockfilePolicyUpdate {
		if !allowNetwork {
			return "", "", "", "", "", fmt.Errorf("feature %q requires network access in update lockfile mode", source)
		}
		resolvedPath, resolvedRef, integrity, version, err := fetchOCIFeature(ctx, cacheDir, source, ref, lock)
		return resolvedPath, "oci", resolvedRef, integrity, version, err
	}
	if !allowNetwork {
		resolvedPath, resolvedRef, integrity, version, err := cachedOCIFeature(cacheDir, source, ref, lock)
		return resolvedPath, "oci", resolvedRef, integrity, version, err
	}
	resolvedPath, resolvedRef, integrity, version, err := fetchOCIFeature(ctx, cacheDir, source, ref, lock)
	return resolvedPath, "oci", resolvedRef, integrity, version, err
}

type gitHubFeatureReference struct {
	owner   string
	repo    string
	feature string
	tag     string
}

func (r gitHubFeatureReference) version() string {
	if r.tag != "" {
		return r.tag
	}
	return "latest"
}

func (r gitHubFeatureReference) tarballURL() string {
	base := strings.TrimRight(githubReleaseBaseURL, "/")
	if r.tag != "" {
		return fmt.Sprintf("%s/%s/%s/releases/download/%s/%s.tgz", base, r.owner, r.repo, r.tag, r.feature)
	}
	return fmt.Sprintf("%s/%s/%s/releases/latest/download/%s.tgz", base, r.owner, r.repo, r.feature)
}

func parseGitHubFeatureReference(source string) (gitHubFeatureReference, error) {
	if strings.Contains(source, "://") || strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || strings.HasPrefix(source, "/") {
		return gitHubFeatureReference{}, fmt.Errorf("expected deprecated github shorthand reference")
	}
	version := ""
	featurePath := source
	if before, after, ok := strings.Cut(source, "@"); ok {
		if after == "" || strings.Contains(after, "@") {
			return gitHubFeatureReference{}, fmt.Errorf("invalid github shorthand version")
		}
		featurePath = before
		version = after
	}
	parts := strings.Split(featurePath, "/")
	if len(parts) != 3 {
		return gitHubFeatureReference{}, fmt.Errorf("expected owner/repo/feature")
	}
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" || strings.Contains(parts[2], ":") {
		return gitHubFeatureReference{}, fmt.Errorf("expected deprecated github shorthand reference")
	}
	for _, part := range parts {
		if part == "" {
			return gitHubFeatureReference{}, fmt.Errorf("expected owner/repo/feature")
		}
	}
	if !strings.Contains(featurePath, "/") {
		return gitHubFeatureReference{}, fmt.Errorf("expected owner/repo/feature")
	}
	return gitHubFeatureReference{owner: parts[0], repo: parts[1], feature: parts[2], tag: version}, nil
}

func validateFeatureLockfilePolicy(source string, lock FeatureLockEntry, policy FeatureLockfilePolicy) error {
	if policy != FeatureLockfilePolicyFrozen {
		return nil
	}
	if !isRemoteFeatureSource(source) || lock.Integrity != "" {
		return nil
	}
	return fmt.Errorf("feature %q requires a lockfile integrity in frozen lockfile mode", source)
}

func isRemoteFeatureSource(source string) bool {
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || strings.HasPrefix(source, "/") {
		return false
	}
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return true
	}
	if _, err := parseGitHubFeatureReference(source); err == nil {
		return true
	}
	_, err := parseOCIReference(source)
	return err == nil
}

func cachedOCIFeature(cacheDir string, source string, ref ociReference, lock FeatureLockEntry) (string, string, string, string, error) {
	if lock.Integrity == "" {
		return "", "", "", "", fmt.Errorf("feature %q requires network access or a lockfile integrity", source)
	}
	key := sha256.Sum256([]byte(source))
	featureDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]), sanitizeFeatureCacheRef(lock.Integrity))
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
		if os.IsNotExist(err) {
			return "", "", "", "", fmt.Errorf("feature %q is not cached locally", source)
		}
		return "", "", "", "", err
	}
	version := lock.Version
	if version == "" {
		version = ref.Reference
	}
	return featureDir, ref.Registry + "/" + ref.Repository + "@" + lock.Integrity, lock.Integrity, version, nil
}

func resolveLocalFeaturePath(configDir string, source string) (string, error) {
	pathValue := source
	if !filepath.IsAbs(pathValue) {
		pathValue = filepath.Join(configDir, pathValue)
	}
	resolved, err := filepath.Abs(pathValue)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("feature %q must resolve to a directory", source)
	}
	return resolved, nil
}

func isMissingPathError(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func parseOCIReference(source string) (ociReference, error) {
	if strings.Contains(source, "://") {
		return ociReference{}, fmt.Errorf("unsupported feature source %q", source)
	}
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], ".") && !strings.Contains(parts[0], ":") && parts[0] != "localhost" {
		return ociReference{}, fmt.Errorf("expected registry/repository reference")
	}
	registry := strings.ToLower(parts[0])
	remainder := parts[1]
	reference := "latest"
	if idx := strings.LastIndex(remainder, "@"); idx >= 0 {
		reference = remainder[idx+1:]
		remainder = remainder[:idx]
	} else if idx := strings.LastIndex(remainder, ":"); idx > strings.LastIndex(remainder, "/") {
		reference = remainder[idx+1:]
		remainder = remainder[:idx]
	}
	if remainder == "" || reference == "" {
		return ociReference{}, fmt.Errorf("invalid OCI feature reference %q", source)
	}
	return ociReference{
		Registry:   registry,
		Repository: strings.ToLower(remainder),
		Reference:  reference,
		Insecure:   registry == "localhost" || strings.HasPrefix(registry, "localhost:") || strings.HasPrefix(registry, "127.0.0.1:"),
	}, nil
}

func fetchOCIFeature(ctx context.Context, cacheDir string, source string, ref ociReference, lock FeatureLockEntry) (string, string, string, string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", "", "", err
	}
	key := sha256.Sum256([]byte(source))
	baseDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]))
	manifestRef := ref
	if lock.Integrity != "" {
		manifestRef.Reference = lock.Integrity
	}
	manifest, digest, token, err := fetchOCIManifest(ctx, manifestRef)
	if err != nil {
		return "", "", "", "", err
	}
	if digest == "" {
		digest = manifestRef.Reference
	}
	if err := security.VerifyImage(ctx, ref.Registry+"/"+ref.Repository+"@"+digest); err != nil {
		return "", "", "", "", err
	}
	featureDir := filepath.Join(baseDir, sanitizeFeatureCacheRef(digest))
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err == nil {
		return featureDir, ref.Registry + "/" + ref.Repository + "@" + digest, digest, ref.Reference, nil
	}
	if len(manifest.Layers) == 0 {
		return "", "", "", "", fmt.Errorf("OCI feature %q has no layers", source)
	}
	if err := os.RemoveAll(featureDir); err != nil {
		return "", "", "", "", err
	}
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		return "", "", "", "", err
	}
	defer func() {
		if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
			_ = os.RemoveAll(featureDir)
		}
	}()
	if err := fetchOCIBlob(ctx, ref, manifest.Layers[0].Digest, token, featureDir); err != nil {
		return "", "", "", "", err
	}
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
		return "", "", "", "", fmt.Errorf("OCI feature %q did not contain devcontainer-feature.json", source)
	}
	return featureDir, ref.Registry + "/" + ref.Repository + "@" + digest, digest, ref.Reference, nil
}

func fetchTarballFeature(ctx context.Context, cacheDir string, source string, lock FeatureLockEntry) (string, string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", err
	}
	key := sha256.Sum256([]byte(source))
	baseDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]))
	body, err := fetchTarballBytes(ctx, source)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(body)
	integrity := "sha256:" + hex.EncodeToString(sum[:])
	if lock.Integrity != "" && lock.Integrity != integrity {
		return "", "", fmt.Errorf("feature %q integrity mismatch: got %s want %s", source, integrity, lock.Integrity)
	}
	featureDir := filepath.Join(baseDir, sanitizeFeatureCacheRef(integrity))
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err == nil {
		return featureDir, integrity, nil
	}
	if err := os.RemoveAll(featureDir); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		return "", "", err
	}
	defer func() {
		if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
			_ = os.RemoveAll(featureDir)
		}
	}()
	if err := extractFeatureLayer(bytes.NewReader(body), featureDir); err != nil {
		return "", "", err
	}
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
		return "", "", fmt.Errorf("tarball feature %q did not contain devcontainer-feature.json", source)
	}
	return featureDir, integrity, nil
}

func cachedTarballFeature(cacheDir string, source string, lock FeatureLockEntry) (string, string, error) {
	if lock.Integrity == "" {
		return "", "", fmt.Errorf("feature %q requires network access or a lockfile integrity", source)
	}
	key := sha256.Sum256([]byte(source))
	featureDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]), sanitizeFeatureCacheRef(lock.Integrity))
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("feature %q is not cached locally", source)
		}
		return "", "", err
	}
	return featureDir, lock.Integrity, nil
}

func fetchTarballBytes(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := featureHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := readAllLimited(resp.Body, featureErrorBodyMaxBytes, "feature error response")
		return nil, fmt.Errorf("feature download failed: %s", strings.TrimSpace(string(body)))
	}
	return readHTTPResponseBody(resp, featureArtifactMaxBytes, "feature tarball")
}

func fetchOCIManifest(ctx context.Context, ref ociReference) (ociManifest, string, string, error) {
	url := registryURL(ref, path.Join("v2", ref.Repository, "manifests", ref.Reference))
	accept := strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", ")
	body, headers, token, err := registryGET(ctx, url, accept, "")
	if err != nil {
		return ociManifest{}, "", "", err
	}
	var manifest ociManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return ociManifest{}, "", "", err
	}
	return manifest, headers.Get("Docker-Content-Digest"), token, nil
}

func fetchOCIBlob(ctx context.Context, ref ociReference, digest string, token string, dstDir string) error {
	url := registryURL(ref, path.Join("v2", ref.Repository, "blobs", digest))
	body, _, _, err := registryGET(ctx, url, "application/octet-stream", token)
	if err != nil {
		return err
	}
	return extractFeatureLayer(bytes.NewReader(body), dstDir)
}

func registryURL(ref ociReference, resource string) string {
	scheme := "https"
	if ref.Insecure {
		scheme = "http"
	}
	return scheme + "://" + ref.Registry + "/" + strings.TrimPrefix(resource, "/")
}

func registryGET(ctx context.Context, rawURL string, accept string, existingToken string) ([]byte, http.Header, string, error) {
	client := featureHTTPClient()
	request := func(token string) ([]byte, http.Header, int, http.Header, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, nil, 0, nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, 0, nil, err
		}
		defer resp.Body.Close()
		limit := featureMetadataMaxBytes
		label := "registry response"
		if accept == "application/octet-stream" {
			limit = featureArtifactMaxBytes
			label = "registry blob"
		}
		body, err := readHTTPResponseBody(resp, limit, label)
		return body, resp.Header.Clone(), resp.StatusCode, resp.Header.Clone(), err
	}
	body, headers, status, authHeaders, err := request(existingToken)
	if err != nil {
		return nil, nil, existingToken, err
	}
	if status == http.StatusUnauthorized {
		token, err := fetchRegistryBearerToken(ctx, authHeaders.Get("Www-Authenticate"))
		if err != nil {
			return nil, nil, existingToken, err
		}
		body, headers, status, _, err = request(token)
		if err != nil {
			return nil, nil, token, err
		}
		if status >= 300 {
			return nil, nil, token, fmt.Errorf("registry request failed: %s", strings.TrimSpace(string(body)))
		}
		return body, headers, token, nil
	}
	if status >= 300 {
		return nil, nil, existingToken, fmt.Errorf("registry request failed: %s", strings.TrimSpace(string(body)))
	}
	return body, headers, existingToken, nil
}

func fetchRegistryBearerToken(ctx context.Context, challenge string) (string, error) {
	challenge = strings.TrimSpace(challenge)
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("unsupported registry auth challenge %q", challenge)
	}
	params := map[string]string{}
	for _, part := range strings.Split(challenge[len("Bearer "):], ",") {
		part = strings.TrimSpace(part)
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		params[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry auth challenge missing realm")
	}
	u, err := url.Parse(realm)
	if err != nil {
		return "", err
	}
	query := u.Query()
	for _, key := range []string{"service", "scope"} {
		if value := params[key]; value != "" {
			query.Set(key, value)
		}
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := featureHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := readAllLimited(resp.Body, featureErrorBodyMaxBytes, "registry token response")
		return "", fmt.Errorf("registry token request failed: %s", strings.TrimSpace(string(body)))
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", fmt.Errorf("registry token response missing token")
}

func featureHTTPClient() *http.Client {
	return &http.Client{Timeout: featureHTTPTimeout}
}

func readHTTPResponseBody(resp *http.Response, limit int64, label string) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return readAllLimited(resp.Body, limit, label)
}

func readAllLimited(reader io.Reader, limit int64, label string) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return body, nil
}

func extractFeatureLayer(reader io.Reader, dstDir string) error {
	tarReader, err := newTarStream(reader)
	if err != nil {
		return err
	}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if !filepath.IsLocal(name) || archivePathHasDotDot(header.Name) {
			continue
		}
		target := filepath.Join(dstDir, name)
		if !strings.HasPrefix(target, dstDir+string(filepath.Separator)) && target != dstDir {
			return fmt.Errorf("feature archive tried to write outside destination")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}
}

func archivePathHasDotDot(name string) bool {
	for _, part := range strings.FieldsFunc(name, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return true
		}
	}
	return false
}

func newTarStream(reader io.Reader) (*tar.Reader, error) {
	buffered := bufio.NewReader(reader)
	peek, err := buffered.Peek(2)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(peek) == 2 && peek[0] == 0x1f && peek[1] == 0x8b {
		gzipReader, err := gzip.NewReader(buffered)
		if err != nil {
			return nil, err
		}
		return tar.NewReader(gzipReader), nil
	}
	return tar.NewReader(buffered), nil
}

func sanitizeFeatureCacheRef(value string) string {
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "@", "_")
	return value
}

func loadFeatureManifest(featureDir string) (featureManifest, error) {
	path := filepath.Join(featureDir, "devcontainer-feature.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return featureManifest{}, err
	}
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return featureManifest{}, fmt.Errorf("parse jsonc %s: %w", path, err)
	}
	var manifest featureManifest
	if err := json.Unmarshal(standardized, &manifest); err != nil {
		return featureManifest{}, err
	}
	return manifest, nil
}

func featureValueOptions(raw any) (map[string]any, bool) {
	switch value := raw.(type) {
	case nil:
		return nil, true
	case bool:
		return nil, value
	case string:
		return map[string]any{"version": value}, true
	case map[string]any:
		return value, true
	default:
		return nil, true
	}
}

func materializeFeatureOptions(manifest featureManifest, overrides map[string]any) map[string]string {
	result := map[string]string{}
	for key, option := range manifest.Options {
		if option.Default != nil {
			result[featureOptionEnvName(key)] = fmt.Sprint(option.Default)
		}
	}
	for key, value := range overrides {
		result[featureOptionEnvName(key)] = fmt.Sprint(value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func featureOptionEnvName(key string) string {
	var b strings.Builder
	for i, r := range key {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_':
			if i == 0 && r >= '0' && r <= '9' {
				b.WriteByte('_')
			}
			if r >= 'a' && r <= 'z' {
				b.WriteRune(r - ('a' - 'A'))
			} else {
				b.WriteRune(r)
			}
		case r >= '0' && r <= '9':
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

func sortedKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func orderFeatures(features []ResolvedFeature, byAlias map[string]int) ([]ResolvedFeature, error) {
	if len(features) <= 1 {
		return features, nil
	}
	incoming := make([]int, len(features))
	edges := make([][]int, len(features))
	for i, feature := range features {
		deps := append([]string(nil), feature.DependsOn...)
		deps = append(deps, feature.InstallsAfter...)
		seen := map[int]struct{}{}
		for _, dep := range deps {
			idx, ok := byAlias[dep]
			if !ok || idx == i {
				if contains(feature.DependsOn, dep) {
					return nil, fmt.Errorf("feature %q dependsOn %q, but only configured features are supported", feature.Metadata.ID, dep)
				}
				continue
			}
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			edges[idx] = append(edges[idx], i)
			incoming[i]++
		}
	}
	ready := make([]int, 0, len(features))
	for i := range features {
		if incoming[i] == 0 {
			ready = append(ready, i)
		}
	}
	result := make([]ResolvedFeature, 0, len(features))
	for len(ready) > 0 {
		sort.Slice(ready, func(i int, j int) bool {
			return features[ready[i]].Metadata.ID < features[ready[j]].Metadata.ID
		})
		current := ready[0]
		ready = ready[1:]
		result = append(result, features[current])
		for _, next := range edges[current] {
			incoming[next]--
			if incoming[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	if len(result) != len(features) {
		return nil, fmt.Errorf("feature dependency cycle detected")
	}
	return result, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
