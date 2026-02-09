package devcontainer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type registryClient struct {
	httpClient *http.Client
	auth       map[string]registryAuth
}

type registryAuth struct {
	username      string
	password      string
	identityToken string
}

type ociManifest struct {
	MediaType   string            `json:"mediaType"`
	Config      ociDescriptor     `json:"config"`
	Layers      []ociDescriptor   `json:"layers"`
	Annotations map[string]string `json:"annotations"`
	Manifests   []ociDescriptor   `json:"manifests"`
}

type ociDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Platform    map[string]any    `json:"platform"`
	Annotations map[string]string `json:"annotations"`
}

func newRegistryClient() *registryClient {
	return &registryClient{
		httpClient: &http.Client{Timeout: 2 * time.Minute},
		auth:       make(map[string]registryAuth),
	}
}

func (c *registryClient) fetchHTTPFeature(ctx context.Context, url string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("feature download failed: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(data)
	dir, err := extractFeatureArchive(data)
	if err != nil {
		return "", "", err
	}
	return dir, fmt.Sprintf("sha256:%s", hex.EncodeToString(digest[:])), nil
}

func (c *registryClient) fetchOCIFeature(ctx context.Context, registry, repository, reference string) (string, string, error) {
	manifest, digest, err := c.fetchManifest(ctx, registry, repository, reference)
	if err != nil {
		return "", "", err
	}
	if manifest.MediaType == "application/vnd.oci.image.index.v1+json" && len(manifest.Manifests) > 0 {
		manifest, digest, err = c.fetchManifest(ctx, registry, repository, manifest.Manifests[0].Digest)
		if err != nil {
			return "", "", err
		}
	}
	layer, err := selectFeatureLayer(manifest.Layers)
	if err != nil {
		return "", "", err
	}
	blob, err := c.fetchBlob(ctx, registry, repository, layer.Digest)
	if err != nil {
		return "", "", err
	}
	dir, err := extractFeatureArchive(blob)
	if err != nil {
		return "", "", err
	}
	return dir, digest, nil
}

func selectFeatureLayer(layers []ociDescriptor) (ociDescriptor, error) {
	for _, layer := range layers {
		if strings.Contains(layer.MediaType, "devcontainers.layer.v1+tar") {
			return layer, nil
		}
	}
	return ociDescriptor{}, errors.New("feature layer not found in OCI manifest")
}

func (c *registryClient) fetchManifest(ctx context.Context, registry, repository, reference string) (ociManifest, string, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, reference)
	headers := map[string]string{
		"Accept": strings.Join([]string{
			"application/vnd.oci.image.index.v1+json",
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		}, ", "),
	}
	resp, err := c.doRequest(ctx, registry, http.MethodGet, url, headers)
	if err != nil {
		return ociManifest{}, "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return ociManifest{}, "", fmt.Errorf("manifest request failed: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ociManifest{}, "", err
	}
	var manifest ociManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return ociManifest{}, "", err
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		sum := sha256.Sum256(body)
		digest = fmt.Sprintf("sha256:%s", hex.EncodeToString(sum[:]))
	}
	return manifest, digest, nil
}

func (c *registryClient) fetchBlob(ctx context.Context, registry, repository, digest string) ([]byte, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, digest)
	resp, err := c.doRequest(ctx, registry, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("blob request failed: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c *registryClient) doRequest(ctx context.Context, registry, method, url string, headers map[string]string) (*http.Response, error) {
	auth := c.lookupAuth(registry)
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if auth.identityToken != "" {
		req.Header.Set("Authorization", "Bearer "+auth.identityToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	challenge := resp.Header.Get("Www-Authenticate")
	if challenge == "" {
		return resp, nil
	}
	token, err := c.fetchBearerToken(ctx, challenge, auth)
	if err != nil {
		return resp, nil
	}
	_ = resp.Body.Close()
	retry, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		retry.Header.Set(key, value)
	}
	retry.Header.Set("Authorization", "Bearer "+token)
	return c.httpClient.Do(retry)
}

func (c *registryClient) fetchBearerToken(ctx context.Context, challenge string, auth registryAuth) (string, error) {
	if !strings.HasPrefix(challenge, "Bearer ") {
		return "", errors.New("unsupported auth challenge")
	}
	parts := parseAuthHeader(strings.TrimPrefix(challenge, "Bearer "))
	realm := parts["realm"]
	if realm == "" {
		return "", errors.New("auth challenge missing realm")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm, nil)
	if err != nil {
		return "", err
	}
	query := req.URL.Query()
	if service := parts["service"]; service != "" {
		query.Set("service", service)
	}
	if scope := parts["scope"]; scope != "" {
		query.Set("scope", scope)
	}
	req.URL.RawQuery = query.Encode()
	if auth.username != "" || auth.password != "" {
		req.SetBasicAuth(auth.username, auth.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth token request failed: %s", resp.Status)
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Token == "" {
		return "", errors.New("auth token missing")
	}
	return payload.Token, nil
}

func parseAuthHeader(value string) map[string]string {
	parts := strings.Split(value, ",")
	result := make(map[string]string, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		val := strings.Trim(kv[1], "\"")
		result[key] = val
	}
	return result
}

func (c *registryClient) lookupAuth(registry string) registryAuth {
	if auth, ok := c.auth[registry]; ok {
		return auth
	}
	auth := loadRegistryAuth(registry)
	c.auth[registry] = auth
	return auth
}

func loadRegistryAuth(registry string) registryAuth {
	path := dockerConfigPath()
	if path == "" {
		return registryAuth{}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return registryAuth{}
	}
	var cfg struct {
		Auths map[string]struct {
			Auth          string `json:"auth"`
			IdentityToken string `json:"identitytoken"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return registryAuth{}
	}
	candidates := []string{registry, "https://" + registry, "http://" + registry}
	for _, key := range candidates {
		if entry, ok := cfg.Auths[key]; ok {
			auth := registryAuth{identityToken: entry.IdentityToken}
			if entry.Auth != "" {
				decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
				if err == nil {
					parts := strings.SplitN(string(decoded), ":", 2)
					if len(parts) == 2 {
						auth.username = parts[0]
						auth.password = parts[1]
					}
				}
			}
			return auth
		}
	}
	return registryAuth{}
}

func dockerConfigPath() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker", "config.json")
}

func extractFeatureArchive(data []byte) (string, error) {
	root, err := os.MkdirTemp("", "godev2-feature-*")
	if err != nil {
		return "", err
	}
	reader := bytes.NewReader(data)
	var tarReader *tar.Reader
	if gz, err := gzip.NewReader(reader); err == nil {
		defer func() {
			_ = gz.Close()
		}()
		tarReader = tar.NewReader(gz)
	} else {
		if _, err := reader.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		tarReader = tar.NewReader(reader)
	}
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Name == "" {
			continue
		}
		target, err := safeExtractPath(root, header.Name)
		if err != nil {
			return "", err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, header.FileInfo().Mode()); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				_ = file.Close()
				return "", err
			}
			if err := file.Close(); err != nil {
				return "", err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return "", err
			}
		default:
			continue
		}
	}
	return findFeatureRoot(root)
}

func safeExtractPath(root, name string) (string, error) {
	cleaned := filepath.Clean(name)
	target := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", errors.New("tar entry escapes destination")
	}
	return target, nil
}

func findFeatureRoot(root string) (string, error) {
	var candidate string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() != "devcontainer-feature.json" {
			return nil
		}
		if candidate != "" {
			return errors.New("multiple devcontainer-feature.json files found")
		}
		candidate = filepath.Dir(path)
		return nil
	})
	if err != nil {
		return "", err
	}
	if candidate == "" {
		return "", errors.New("devcontainer-feature.json not found in archive")
	}
	if _, err := os.Stat(filepath.Join(candidate, "install.sh")); err != nil {
		return "", errors.New("install.sh not found in feature")
	}
	return candidate, nil
}
