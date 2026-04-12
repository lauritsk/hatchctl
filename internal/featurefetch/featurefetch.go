package featurefetch

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lauritsk/hatchctl/internal/security"
	storefs "github.com/lauritsk/hatchctl/internal/store/fs"
)

var (
	featureHTTPTimeout                 = 90 * time.Second
	featureArtifactMaxBytes      int64 = 64 << 20
	featureMetadataMaxBytes      int64 = 2 << 20
	featureErrorBodyMaxBytes     int64 = 64 << 10
	featureExtractedMaxBytes     int64 = 256 << 20
	featureExtractedMaxFiles           = 4096
	featureExtractedMaxFileBytes int64 = 64 << 20
)

type ResolveOptions struct {
	AllowNetwork bool
	HTTPTimeout  time.Duration
	VerifyImage  func(context.Context, string) security.VerificationResult
}

type ResolvedSource struct {
	Path         string
	Kind         string
	Resolved     string
	Integrity    string
	Version      string
	Verification security.VerificationResult
}

func ResolveSource(ctx context.Context, configDir string, cacheDir string, source string, lock storefs.FeatureLockEntry, lockfilePolicy string, opts ResolveOptions) (ResolvedSource, error) {
	httpTimeout := effectiveFeatureHTTPTimeout(opts.HTTPTimeout)
	parsed, err := parseFeatureSource(configDir, source)
	if err != nil {
		return ResolvedSource{}, err
	}
	switch parsed.Kind {
	case "file-path":
		return ResolvedSource{Path: parsed.LocalPath, Kind: "file-path", Resolved: source}, nil
	case "direct-tarball":
		return resolveTarballSource(ctx, cacheDir, source, parsed.Source, "direct-tarball", parsed.Source, lock, lockfilePolicy, opts, httpTimeout)
	case "github-release":
		resolvedSource := parsed.GitHubRef.tarballURL()
		return resolveTarballSource(ctx, cacheDir, source, resolvedSource, "github-release", parsed.GitHubRef.version(), lock, lockfilePolicy, opts, httpTimeout)
	case "oci":
		return resolveOCISource(ctx, cacheDir, source, parsed.OCIRef, lock, lockfilePolicy, opts, httpTimeout)
	default:
		return ResolvedSource{}, fmt.Errorf("unsupported feature source kind %q", parsed.Kind)
	}
}

func resolveTarballSource(ctx context.Context, cacheDir string, source string, resolvedSource string, kind string, version string, lock storefs.FeatureLockEntry, lockfilePolicy string, opts ResolveOptions, httpTimeout time.Duration) (ResolvedSource, error) {
	fetchRemote, err := shouldFetchRemoteSource(source, lockfilePolicy, opts.AllowNetwork)
	if err != nil {
		return ResolvedSource{}, err
	}
	if fetchRemote {
		resolvedPath, integrity, err := fetchTarballFeature(ctx, cacheDir, resolvedSource, lock, httpTimeout)
		return ResolvedSource{Path: resolvedPath, Kind: kind, Resolved: resolvedSource, Integrity: integrity, Version: version}, err
	}
	resolvedPath, integrity, err := cachedTarballFeature(cacheDir, resolvedSource, lock)
	return ResolvedSource{Path: resolvedPath, Kind: kind, Resolved: resolvedSource, Integrity: integrity, Version: version}, err
}

func resolveOCISource(ctx context.Context, cacheDir string, source string, ref ociReference, lock storefs.FeatureLockEntry, lockfilePolicy string, opts ResolveOptions, httpTimeout time.Duration) (ResolvedSource, error) {
	fetchRemote, err := shouldFetchRemoteSource(source, lockfilePolicy, opts.AllowNetwork)
	if err != nil {
		return ResolvedSource{}, err
	}
	if fetchRemote {
		resolvedPath, resolvedRef, integrity, version, verification, err := fetchOCIFeature(ctx, cacheDir, source, ref, lock, httpTimeout, opts.VerifyImage)
		return ResolvedSource{Path: resolvedPath, Kind: "oci", Resolved: resolvedRef, Integrity: integrity, Version: version, Verification: verification}, err
	}
	resolvedPath, resolvedRef, integrity, version, err := cachedOCIFeature(cacheDir, source, ref, lock)
	return ResolvedSource{Path: resolvedPath, Kind: "oci", Resolved: resolvedRef, Integrity: integrity, Version: version}, err
}

func shouldFetchRemoteSource(source string, lockfilePolicy string, allowNetwork bool) (bool, error) {
	if lockfilePolicy == "update" {
		if !allowNetwork {
			return false, fmt.Errorf("feature %q requires network access in update lockfile mode", source)
		}
		return true, nil
	}
	return allowNetwork, nil
}

func cachedOCIFeature(cacheDir string, source string, ref ociReference, lock storefs.FeatureLockEntry) (string, string, string, string, error) {
	if lock.Integrity == "" {
		return "", "", "", "", fmt.Errorf("feature %q requires network access or a lockfile integrity", source)
	}
	key := sha256.Sum256([]byte(source))
	featureDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]), SanitizeCacheRef(lock.Integrity))
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

func fetchOCIFeature(ctx context.Context, cacheDir string, source string, ref ociReference, lock storefs.FeatureLockEntry, httpTimeout time.Duration, verifyImage func(context.Context, string) security.VerificationResult) (string, string, string, string, security.VerificationResult, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", "", "", security.VerificationResult{}, err
	}
	key := sha256.Sum256([]byte(source))
	baseDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]))
	manifestRef := ref
	if lock.Integrity != "" {
		manifestRef.Reference = lock.Integrity
	}
	manifest, digest, token, err := fetchOCIManifest(ctx, manifestRef, httpTimeout)
	if err != nil {
		return "", "", "", "", security.VerificationResult{}, err
	}
	if digest == "" {
		digest = manifestRef.Reference
	}
	verification := security.VerificationResult{}
	if verifyImage != nil {
		verification = verifyImage(ctx, ref.Registry+"/"+ref.Repository+"@"+digest)
	}
	featureDir := filepath.Join(baseDir, SanitizeCacheRef(digest))
	if ok, err := hasFeatureManifest(featureDir); err != nil {
		return "", "", "", "", verification, err
	} else if ok {
		return featureDir, ref.Registry + "/" + ref.Repository + "@" + digest, digest, ref.Reference, verification, nil
	}
	if len(manifest.Layers) == 0 {
		return "", "", "", "", verification, fmt.Errorf("OCI feature %q has no layers", source)
	}
	if err := populateFeatureCache(baseDir, featureDir, func(tempDir string) error {
		return fetchOCIBlob(ctx, ref, manifest.Layers[0].Digest, token, tempDir, httpTimeout)
	}); err != nil {
		return "", "", "", "", verification, err
	}
	return featureDir, ref.Registry + "/" + ref.Repository + "@" + digest, digest, ref.Reference, verification, nil
}

func fetchTarballFeature(ctx context.Context, cacheDir string, source string, lock storefs.FeatureLockEntry, httpTimeout time.Duration) (string, string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", err
	}
	key := sha256.Sum256([]byte(source))
	baseDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]))
	body, err := fetchTarballBytes(ctx, source, httpTimeout)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(body)
	integrity := "sha256:" + hex.EncodeToString(sum[:])
	if lock.Integrity != "" && lock.Integrity != integrity {
		return "", "", fmt.Errorf("feature %q integrity mismatch: got %s want %s", source, integrity, lock.Integrity)
	}
	featureDir := filepath.Join(baseDir, SanitizeCacheRef(integrity))
	if ok, err := hasFeatureManifest(featureDir); err != nil {
		return "", "", err
	} else if ok {
		return featureDir, integrity, nil
	}
	if err := populateFeatureCache(baseDir, featureDir, func(tempDir string) error {
		return ExtractFeatureLayer(bytes.NewReader(body), tempDir)
	}); err != nil {
		return "", "", err
	}
	return featureDir, integrity, nil
}

func cachedTarballFeature(cacheDir string, source string, lock storefs.FeatureLockEntry) (string, string, error) {
	if lock.Integrity == "" {
		return "", "", fmt.Errorf("feature %q requires network access or a lockfile integrity", source)
	}
	key := sha256.Sum256([]byte(source))
	featureDir := filepath.Join(cacheDir, hex.EncodeToString(key[:]), SanitizeCacheRef(lock.Integrity))
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("feature %q is not cached locally", source)
		}
		return "", "", err
	}
	return featureDir, lock.Integrity, nil
}

func fetchTarballBytes(ctx context.Context, rawURL string, httpTimeout time.Duration) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := featureHTTPClient(httpTimeout, validateTarballRedirect).Do(req)
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

func effectiveFeatureHTTPTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return featureHTTPTimeout
}

func populateFeatureCache(baseDir string, featureDir string, populate func(string) error) error {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	if ok, err := hasFeatureManifest(featureDir); err != nil {
		return err
	} else if ok {
		return nil
	}
	tempDir, err := os.MkdirTemp(baseDir, filepath.Base(featureDir)+".tmp-*")
	if err != nil {
		return err
	}
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.RemoveAll(tempDir)
		}
	}()
	if err := populate(tempDir); err != nil {
		return err
	}
	if ok, err := hasFeatureManifest(tempDir); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("cached feature is missing devcontainer-feature.json")
	}
	if err := os.Rename(tempDir, featureDir); err != nil {
		if ok, statErr := hasFeatureManifest(featureDir); statErr == nil && ok {
			return nil
		}
		if removeErr := os.RemoveAll(featureDir); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if retryErr := os.Rename(tempDir, featureDir); retryErr != nil {
			if ok, statErr := hasFeatureManifest(featureDir); statErr == nil && ok {
				return nil
			}
			return retryErr
		}
	}
	keepTemp = true
	return nil
}

func hasFeatureManifest(featureDir string) (bool, error) {
	if _, err := os.Stat(filepath.Join(featureDir, "devcontainer-feature.json")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func featureHTTPClient(timeout time.Duration, redirectPolicy func(*http.Request, []*http.Request) error) *http.Client {
	return &http.Client{Timeout: effectiveFeatureHTTPTimeout(timeout), CheckRedirect: redirectPolicy}
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

func ExtractFeatureLayer(reader io.Reader, dstDir string) error {
	tarReader, err := newTarStream(reader)
	if err != nil {
		return err
	}
	var extractedFiles int
	var extractedBytes int64
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
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return fmt.Errorf("feature archive contains invalid file size for %s", header.Name)
			}
			if header.Size > featureExtractedMaxFileBytes {
				return fmt.Errorf("feature archive file %s exceeds %d bytes", header.Name, featureExtractedMaxFileBytes)
			}
			extractedFiles++
			if extractedFiles > featureExtractedMaxFiles {
				return fmt.Errorf("feature archive exceeds %d files", featureExtractedMaxFiles)
			}
			extractedBytes += header.Size
			if extractedBytes > featureExtractedMaxBytes {
				return fmt.Errorf("feature archive exceeds %d bytes when extracted", featureExtractedMaxBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0o755)
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

func validateTarballURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if strings.EqualFold(u.Scheme, "https") {
		return nil
	}
	if strings.EqualFold(u.Scheme, "http") && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("feature tarball %q must use https or loopback http", rawURL)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func denyRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

func validateTarballRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	return validateTarballURL(req.URL.String())
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

func SanitizeCacheRef(value string) string {
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "@", "_")
	return value
}
