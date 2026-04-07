package devcontainer

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
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

	"github.com/tailscale/hujson"
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
}

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

func ResolveFeatures(configPath string, configDir string, cacheDir string, values map[string]any, opts FeatureResolveOptions) ([]ResolvedFeature, error) {
	if len(values) == 0 {
		return nil, nil
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
		featurePath, kind, resolvedRef, integrity, version, err := resolveFeaturePath(configDir, cacheDir, source, lockFile[source], opts.AllowNetwork)
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
	if opts.WriteLockFile {
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

func resolveFeaturePath(configDir string, cacheDir string, source string, lock FeatureLockEntry, allowNetwork bool) (string, string, string, string, string, error) {
	resolved, err := resolveLocalFeaturePath(configDir, source)
	if err == nil {
		return resolved, "file-path", source, "", "", nil
	}
	if !isMissingPathError(err) {
		return "", "", "", "", "", err
	}
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		if !allowNetwork {
			resolved, integrity, err := cachedTarballFeature(cacheDir, source, lock)
			return resolved, "direct-tarball", source, integrity, source, err
		}
		resolved, integrity, err := fetchTarballFeature(cacheDir, source, lock)
		return resolved, "direct-tarball", source, integrity, source, err
	}
	ref, err := parseOCIReference(source)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("feature %q not found locally and is not a valid remote feature source: %w", source, err)
	}
	if !allowNetwork {
		resolvedPath, resolvedRef, integrity, version, err := cachedOCIFeature(cacheDir, source, ref, lock)
		return resolvedPath, "oci", resolvedRef, integrity, version, err
	}
	resolvedPath, resolvedRef, integrity, version, err := fetchOCIFeature(cacheDir, source, ref, lock)
	return resolvedPath, "oci", resolvedRef, integrity, version, err
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

func fetchOCIFeature(cacheDir string, source string, ref ociReference, lock FeatureLockEntry) (string, string, string, string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", "", "", err
	}
	key := sha256.Sum256([]byte(source))
	baseDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]))
	manifestRef := ref
	if lock.Integrity != "" {
		manifestRef.Reference = lock.Integrity
	}
	manifest, digest, token, err := fetchOCIManifest(manifestRef)
	if err != nil {
		return "", "", "", "", err
	}
	if digest == "" {
		digest = manifestRef.Reference
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
	if err := fetchOCIBlob(ref, manifest.Layers[0].Digest, token, featureDir); err != nil {
		return "", "", "", "", err
	}
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
		return "", "", "", "", fmt.Errorf("OCI feature %q did not contain devcontainer-feature.json", source)
	}
	return featureDir, ref.Registry + "/" + ref.Repository + "@" + digest, digest, ref.Reference, nil
}

func fetchTarballFeature(cacheDir string, source string, lock FeatureLockEntry) (string, string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", err
	}
	key := sha256.Sum256([]byte(source))
	baseDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]))
	body, err := fetchTarballBytes(source)
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

func fetchTarballBytes(rawURL string) ([]byte, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("feature download failed: %s", strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func fetchOCIManifest(ref ociReference) (ociManifest, string, string, error) {
	url := registryURL(ref, path.Join("v2", ref.Repository, "manifests", ref.Reference))
	accept := strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", ")
	body, headers, token, err := registryGET(url, accept, "")
	if err != nil {
		return ociManifest{}, "", "", err
	}
	var manifest ociManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return ociManifest{}, "", "", err
	}
	return manifest, headers.Get("Docker-Content-Digest"), token, nil
}

func fetchOCIBlob(ref ociReference, digest string, token string, dstDir string) error {
	url := registryURL(ref, path.Join("v2", ref.Repository, "blobs", digest))
	body, _, _, err := registryGET(url, "application/octet-stream", token)
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

func registryGET(rawURL string, accept string, existingToken string) ([]byte, http.Header, string, error) {
	client := &http.Client{}
	request := func(token string) ([]byte, http.Header, int, http.Header, error) {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
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
		body, err := io.ReadAll(resp.Body)
		return body, resp.Header.Clone(), resp.StatusCode, resp.Header.Clone(), err
	}
	body, headers, status, authHeaders, err := request(existingToken)
	if err != nil {
		return nil, nil, existingToken, err
	}
	if status == http.StatusUnauthorized {
		token, err := fetchRegistryBearerToken(authHeaders.Get("Www-Authenticate"))
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

func fetchRegistryBearerToken(challenge string) (string, error) {
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
	resp, err := http.Get(u.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
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
		if name == "." || name == string(filepath.Separator) || strings.HasPrefix(name, "..") {
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
