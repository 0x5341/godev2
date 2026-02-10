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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
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
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", registry, repository))
	if err != nil {
		return "", "", err
	}
	if isLocalRegistry(registry) {
		repo.PlainHTTP = true
	}
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			return c.orasCredential(hostport), nil
		},
	}
	desc, err := repo.Resolve(ctx, reference)
	if err != nil {
		return "", "", err
	}
	manifestDesc := desc
	if isManifestIndex(desc.MediaType) {
		indexBytes, err := content.FetchAll(ctx, repo, desc)
		if err != nil {
			return "", "", err
		}
		var index ocispec.Index
		if err := json.Unmarshal(indexBytes, &index); err != nil {
			return "", "", err
		}
		if len(index.Manifests) == 0 {
			return "", "", errors.New("OCI manifest index has no manifests")
		}
		manifestDesc = index.Manifests[0]
	}
	manifestBytes, err := content.FetchAll(ctx, repo, manifestDesc)
	if err != nil {
		return "", "", err
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return "", "", err
	}
	layer, err := selectFeatureLayer(manifest.Layers)
	if err != nil {
		return "", "", err
	}
	blob, err := content.FetchAll(ctx, repo, layer)
	if err != nil {
		return "", "", err
	}
	dir, err := extractFeatureArchive(blob)
	if err != nil {
		return "", "", err
	}
	return dir, manifestDesc.Digest.String(), nil
}

func selectFeatureLayer(layers []ocispec.Descriptor) (ocispec.Descriptor, error) {
	for _, layer := range layers {
		if strings.Contains(layer.MediaType, "devcontainers.layer.v1+tar") {
			return layer, nil
		}
	}
	return ocispec.Descriptor{}, errors.New("feature layer not found in OCI manifest")
}

func (c *registryClient) lookupAuth(registry string) registryAuth {
	if auth, ok := c.auth[registry]; ok {
		return auth
	}
	auth := loadRegistryAuth(registry)
	c.auth[registry] = auth
	return auth
}

func (c *registryClient) orasCredential(hostport string) auth.Credential {
	authInfo := c.lookupAuth(hostport)
	if authInfo.identityToken != "" {
		return auth.Credential{AccessToken: authInfo.identityToken}
	}
	if authInfo.username != "" || authInfo.password != "" {
		return auth.Credential{Username: authInfo.username, Password: authInfo.password}
	}
	return auth.EmptyCredential
}

func isManifestIndex(mediaType string) bool {
	switch mediaType {
	case ocispec.MediaTypeImageIndex, "application/vnd.docker.distribution.manifest.list.v2+json":
		return true
	default:
		return false
	}
}

func isLocalRegistry(registry string) bool {
	host := registry
	if parsed, _, err := net.SplitHostPort(registry); err == nil {
		host = parsed
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
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
