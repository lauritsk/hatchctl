package featurefetch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var githubReleaseBaseURL = "https://github.com"

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

type parsedFeatureSource struct {
	Kind      string
	Source    string
	LocalPath string
	GitHubRef gitHubFeatureReference
	OCIRef    ociReference
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
		return gitHubFeatureReference{}, fmt.Errorf("invalid GitHub feature reference %q; expected owner/repo/feature[@version]", source)
	}
	version := ""
	featurePath := source
	if before, after, ok := strings.Cut(source, "@"); ok {
		if after == "" || strings.Contains(after, "@") {
			return gitHubFeatureReference{}, fmt.Errorf("invalid GitHub feature reference %q; expected owner/repo/feature[@version]", source)
		}
		featurePath = before
		version = after
	}
	parts := strings.Split(featurePath, "/")
	if len(parts) != 3 {
		return gitHubFeatureReference{}, fmt.Errorf("invalid GitHub feature reference %q; expected owner/repo/feature[@version]", source)
	}
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" || strings.Contains(parts[2], ":") {
		return gitHubFeatureReference{}, fmt.Errorf("invalid GitHub feature reference %q; expected owner/repo/feature[@version]", source)
	}
	for _, part := range parts {
		if part == "" {
			return gitHubFeatureReference{}, fmt.Errorf("invalid GitHub feature reference %q; expected owner/repo/feature[@version]", source)
		}
	}
	if !strings.Contains(featurePath, "/") {
		return gitHubFeatureReference{}, fmt.Errorf("invalid GitHub feature reference %q; expected owner/repo/feature[@version]", source)
	}
	return gitHubFeatureReference{owner: parts[0], repo: parts[1], feature: parts[2], tag: version}, nil
}

func IsRemoteFeatureSource(source string) bool {
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || strings.HasPrefix(source, "/") {
		return false
	}
	parsed, err := parseFeatureSource("", source)
	if err != nil {
		return false
	}
	return parsed.Kind != "file-path"
}

func parseFeatureSource(configDir string, source string) (parsedFeatureSource, error) {
	resolved, err := ResolveLocalFeaturePath(configDir, source)
	if err == nil {
		return parsedFeatureSource{Kind: "file-path", Source: source, LocalPath: resolved}, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return parsedFeatureSource{}, err
	}
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || strings.HasPrefix(source, "/") {
		return parsedFeatureSource{}, err
	}
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		if err := validateTarballURL(source); err != nil {
			return parsedFeatureSource{}, err
		}
		return parsedFeatureSource{Kind: "direct-tarball", Source: source}, nil
	}
	if githubRef, err := parseGitHubFeatureReference(source); err == nil {
		return parsedFeatureSource{Kind: "github-release", Source: source, GitHubRef: githubRef}, nil
	}
	ref, err := parseOCIReference(source)
	if err != nil {
		return parsedFeatureSource{}, fmt.Errorf("feature %q not found locally and is not a valid remote feature source: %w", source, err)
	}
	return parsedFeatureSource{Kind: "oci", Source: source, OCIRef: ref}, nil
}

func ResolveLocalFeaturePath(configDir string, source string) (string, error) {
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
