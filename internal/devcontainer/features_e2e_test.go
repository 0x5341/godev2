package devcontainer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

type dockerResourceCounts struct {
	containers int
	images     int
	volumes    int
}

func TestDockerEngine_FeaturesLocal(t *testing.T) {
	cli := requireDocker(t)
	pre := countDockerResources(t, cli)
	containerID := ""
	featuresImage := ""
	baseImage := "alpine:3.19"
	removeBaseImage := false
	t.Cleanup(func() {
		cleanupContainer(t, cli, containerID)
		cleanupImage(t, cli, featuresImage)
		if removeBaseImage {
			cleanupImage(t, cli, baseImage)
		}
	})

	root := t.TempDir()
	copyTestcaseDir(t, root, "features", "local")
	configPath := filepath.Join(root, ".devcontainer", "devcontainer.json")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	workspaceRoot, _, _, vars, err := resolveWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveWorkspacePaths: %v", err)
	}
	features, err := resolveFeatures(context.Background(), configPath, workspaceRoot, cfg)
	if err != nil {
		t.Fatalf("resolveFeatures: %v", err)
	}
	if features != nil {
		featuresImage = featuresImageTag(workspaceRoot, vars["devcontainerId"], features.Order)
	}

	inspectCtx, cancelInspect := context.WithTimeout(context.Background(), 10*time.Second)
	if _, err := cli.ImageInspect(inspectCtx, baseImage); err != nil {
		removeBaseImage = true
	}
	cancelInspect()

	startCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	containerID, err = StartDevcontainer(startCtx, WithConfigPath(configPath))
	if err != nil {
		t.Fatalf("StartDevcontainer: %v", err)
	}

	output := execContainer(t, cli, containerID, []string{"cat", "/tmp/feature-installed"})
	if strings.TrimSpace(output) != "message=hello flag=true" {
		t.Fatalf("unexpected feature output: %s", output)
	}
	logPath := filepath.Join(root, "feature.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read feature log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 || lines[0] != "feature" || lines[1] != "user" {
		t.Fatalf("unexpected feature log: %#v", lines)
	}

	if err := StopDevcontainer(context.Background(), containerID, 10*time.Second); err != nil {
		t.Fatalf("StopDevcontainer: %v", err)
	}
	if err := RemoveDevcontainer(context.Background(), containerID); err != nil {
		t.Fatalf("RemoveDevcontainer: %v", err)
	}
	containerID = ""
	cleanupImage(t, cli, featuresImage)
	featuresImage = ""
	if removeBaseImage {
		cleanupImage(t, cli, baseImage)
	}
	post := countDockerResources(t, cli)
	if post.containers > pre.containers {
		t.Fatalf("container count increased: %d -> %d", pre.containers, post.containers)
	}
	if post.images > pre.images {
		t.Fatalf("image count increased: %d -> %d", pre.images, post.images)
	}
	if post.volumes > pre.volumes {
		t.Fatalf("volume count increased: %d -> %d", pre.volumes, post.volumes)
	}
}

func TestDockerEngine_FeaturesOCI(t *testing.T) {
	cli := requireDocker(t)
	pre := countDockerResources(t, cli)
	containerID := ""
	registryID := ""
	featuresImage := ""
	baseImage := "alpine:3.19"
	registryImage := "registry:2"
	removeBaseImage := false
	removeRegistryImage := false
	t.Cleanup(func() {
		cleanupContainer(t, cli, containerID)
		cleanupContainer(t, cli, registryID)
		cleanupImage(t, cli, featuresImage)
		if removeBaseImage {
			cleanupImage(t, cli, baseImage)
		}
		if removeRegistryImage {
			cleanupImage(t, cli, registryImage)
		}
	})

	root := t.TempDir()
	copyTestcaseDir(t, root, "features", "oci")
	removeRegistryImage = ensureImageAvailable(t, cli, registryImage)
	registryID, registryAddr := startRegistry(t, cli)
	waitForRegistry(t, registryAddr)

	featureDir := filepath.Join(root, "feature-oci")
	archive := archiveFeatureDir(t, featureDir)
	repo := "devcontainers/test-feature"
	tag := "1.0.0"
	pushCtx, cancelPush := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelPush()
	if err := pushFeatureToRegistry(pushCtx, registryAddr, repo, tag, archive); err != nil {
		t.Fatalf("push feature: %v", err)
	}

	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	template := string(readTestcaseFile(t, "features", "oci", "devcontainer.json.tmpl"))
	featureID := fmt.Sprintf("%s/%s:%s", registryAddr, repo, tag)
	config := strings.ReplaceAll(template, "__FEATURE_ID__", featureID)
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	workspaceRoot, _, _, vars, err := resolveWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveWorkspacePaths: %v", err)
	}
	features, err := resolveFeatures(context.Background(), configPath, workspaceRoot, cfg)
	if err != nil {
		t.Fatalf("resolveFeatures: %v", err)
	}
	if features != nil {
		featuresImage = featuresImageTag(workspaceRoot, vars["devcontainerId"], features.Order)
	}

	inspectCtx, cancelInspect := context.WithTimeout(context.Background(), 10*time.Second)
	if _, err := cli.ImageInspect(inspectCtx, baseImage); err != nil {
		removeBaseImage = true
	}
	cancelInspect()

	startCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	containerID, err = StartDevcontainer(startCtx, WithConfigPath(configPath))
	if err != nil {
		t.Fatalf("StartDevcontainer: %v", err)
	}

	output := execContainer(t, cli, containerID, []string{"cat", "/tmp/feature-oci-installed"})
	if strings.TrimSpace(output) != "oci" {
		t.Fatalf("unexpected OCI feature output: %s", output)
	}

	if err := StopDevcontainer(context.Background(), containerID, 10*time.Second); err != nil {
		t.Fatalf("StopDevcontainer: %v", err)
	}
	if err := RemoveDevcontainer(context.Background(), containerID); err != nil {
		t.Fatalf("RemoveDevcontainer: %v", err)
	}
	containerID = ""
	cleanupImage(t, cli, featuresImage)
	featuresImage = ""
	cleanupContainer(t, cli, registryID)
	registryID = ""
	if removeBaseImage {
		cleanupImage(t, cli, baseImage)
	}
	if removeRegistryImage {
		cleanupImage(t, cli, registryImage)
	}
	post := countDockerResources(t, cli)
	if post.containers > pre.containers {
		t.Fatalf("container count increased: %d -> %d", pre.containers, post.containers)
	}
	if post.images > pre.images {
		t.Fatalf("image count increased: %d -> %d", pre.images, post.images)
	}
	if post.volumes > pre.volumes {
		t.Fatalf("volume count increased: %d -> %d", pre.volumes, post.volumes)
	}
}

func execContainer(t *testing.T, cli *client.Client, containerID string, cmd []string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	execResp, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("ContainerExecCreate: %v", err)
	}
	attach, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{Tty: false})
	if err != nil {
		t.Fatalf("ContainerExecAttach: %v", err)
	}
	defer attach.Close()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil {
		t.Fatalf("ContainerExecAttach read: %v", err)
	}
	inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		t.Fatalf("ContainerExecInspect: %v", err)
	}
	if inspect.ExitCode != 0 {
		t.Fatalf("exec failed: %s", strings.TrimSpace(stderr.String()))
	}
	return stdout.String()
}

func countDockerResources(t *testing.T, cli *client.Client) dockerResourceCounts {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		t.Fatalf("ImageList: %v", err)
	}
	volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("VolumeList: %v", err)
	}
	return dockerResourceCounts{
		containers: len(containers),
		images:     len(images),
		volumes:    len(volumes.Volumes),
	}
}

func ensureImageAvailable(t *testing.T, cli *client.Client, imageRef string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := cli.ImageInspect(ctx, imageRef); err == nil {
		return false
	}
	reader, err := cli.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		t.Fatalf("ImagePull: %v", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatalf("ImagePull read: %v", err)
	}
	return true
}

func startRegistry(t *testing.T, cli *client.Client) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	port := nat.Port("5000/tcp")
	config := &container.Config{
		Image:        "registry:2",
		ExposedPorts: nat.PortSet{port: struct{}{}},
	}
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			port: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}},
		},
	}
	created, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		t.Fatalf("ContainerCreate registry: %v", err)
	}
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("ContainerStart registry: %v", err)
	}
	inspect, err := cli.ContainerInspect(ctx, created.ID)
	if err != nil {
		t.Fatalf("ContainerInspect registry: %v", err)
	}
	bindings := inspect.NetworkSettings.Ports[port]
	if len(bindings) == 0 {
		t.Fatalf("registry port binding missing")
	}
	host := bindings[0].HostIP
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return created.ID, fmt.Sprintf("%s:%s", host, bindings[0].HostPort)
}

func waitForRegistry(t *testing.T, addr string) {
	t.Helper()
	url := fmt.Sprintf("http://%s/v2/", addr)
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 10; i++ {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("registry not ready: %s", addr)
}

func archiveFeatureDir(t *testing.T, dir string) []byte {
	t.Helper()
	base := filepath.Base(dir)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(base, rel))
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, file); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	})
	if err != nil {
		t.Fatalf("archive feature: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func pushFeatureToRegistry(ctx context.Context, registryAddr, repo, tag string, payload []byte) error {
	repository, err := remote.NewRepository(fmt.Sprintf("%s/%s", registryAddr, repo))
	if err != nil {
		return err
	}
	repository.PlainHTTP = true
	repository.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			return auth.EmptyCredential, nil
		},
	}
	layerDesc, err := oras.PushBytes(ctx, repository, "application/vnd.devcontainers.layer.v1+tar", payload)
	if err != nil {
		return err
	}
	manifestDesc, err := oras.PackManifest(ctx, repository, oras.PackManifestVersion1_1, "application/vnd.devcontainers", oras.PackManifestOptions{
		Layers: []ocispec.Descriptor{layerDesc},
	})
	if err != nil {
		return err
	}
	return repository.Tag(ctx, manifestDesc, tag)
}
