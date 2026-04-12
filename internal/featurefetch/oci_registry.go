package featurefetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

func fetchOCIManifest(ctx context.Context, ref ociReference, httpTimeout time.Duration) (ociManifest, string, string, error) {
	url := registryURL(ref, path.Join("v2", ref.Repository, "manifests", ref.Reference))
	accept := strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", ")
	body, headers, token, err := registryGET(ctx, url, accept, "", httpTimeout)
	if err != nil {
		return ociManifest{}, "", "", err
	}
	var manifest ociManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return ociManifest{}, "", "", err
	}
	return manifest, headers.Get("Docker-Content-Digest"), token, nil
}

func fetchOCIBlob(ctx context.Context, ref ociReference, digest string, token string, dstDir string, httpTimeout time.Duration) error {
	url := registryURL(ref, path.Join("v2", ref.Repository, "blobs", digest))
	body, _, _, err := registryGET(ctx, url, "application/octet-stream", token, httpTimeout)
	if err != nil {
		return err
	}
	return ExtractFeatureLayer(bytes.NewReader(body), dstDir)
}

func registryURL(ref ociReference, resource string) string {
	scheme := "https"
	if ref.Insecure {
		scheme = "http"
	}
	return scheme + "://" + ref.Registry + "/" + strings.TrimPrefix(resource, "/")
}

func registryGET(ctx context.Context, rawURL string, accept string, existingToken string, httpTimeout time.Duration) ([]byte, http.Header, string, error) {
	client := featureHTTPClient(httpTimeout, nil)
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
		token, err := fetchRegistryBearerToken(ctx, rawURL, authHeaders.Get("Www-Authenticate"), httpTimeout)
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

func fetchRegistryBearerToken(ctx context.Context, requestURL string, challenge string, httpTimeout time.Duration) (string, error) {
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
	if err := validateRegistryTokenRealm(requestURL, u); err != nil {
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
	resp, err := featureHTTPClient(httpTimeout, denyRedirects).Do(req)
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

func validateRegistryTokenRealm(requestURL string, realm *url.URL) error {
	requestParsed, err := url.Parse(requestURL)
	if err != nil {
		return err
	}
	if !strings.EqualFold(realm.Hostname(), requestParsed.Hostname()) {
		return fmt.Errorf("registry auth challenge points to unexpected host %q", realm.Hostname())
	}
	if strings.EqualFold(realm.Scheme, "https") {
		return nil
	}
	if strings.EqualFold(realm.Scheme, "http") && isLoopbackHost(realm.Hostname()) {
		return nil
	}
	return fmt.Errorf("registry auth challenge %q must use https or loopback http", realm.String())
}
