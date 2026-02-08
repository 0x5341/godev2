package devcontainer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func requireDocker(t *testing.T) *client.Client {
	t.Helper()
	cli, err := newDockerClient()
	if err != nil {
		t.Skipf("docker client unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true}); err != nil {
		_ = cli.Close()
		t.Skipf("docker daemon unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = cli.Close()
	})
	return cli
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file location")
	}
	dir := filepath.Dir(file)
	return filepath.Dir(filepath.Dir(dir))
}

func testcasePath(t *testing.T, parts ...string) string {
	t.Helper()
	segments := append([]string{repoRoot(t), "testcase"}, parts...)
	path := filepath.Join(segments...)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("testcase path not found: %s: %v", path, err)
	}
	return path
}

func cleanupContainer(t *testing.T, cli *client.Client, containerID string) {
	t.Helper()
	if containerID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	}); err != nil {
		t.Logf("cleanup container: %v", err)
	}
}

func cleanupImage(t *testing.T, cli *client.Client, imageRef string) {
	t.Helper()
	if imageRef == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := cli.ImageRemove(ctx, imageRef, client.ImageRemoveOptions{
		Force:         true,
		PruneChildren: true,
	}); err != nil {
		t.Logf("cleanup image: %v", err)
	}
}

func TestDockerEngine_StartStopRemove(t *testing.T) {
	cli := requireDocker(t)
	configPath := testcasePath(t, "docker-engine-image", ".devcontainer", "devcontainer.json")

	inspectCtx, cancelInspect := context.WithTimeout(context.Background(), 10*time.Second)
	removeBaseImage := false
	if _, err := cli.ImageInspect(inspectCtx, "alpine:3.19"); err != nil {
		removeBaseImage = true
	}
	cancelInspect()
	if removeBaseImage {
		t.Cleanup(func() {
			cleanupImage(t, cli, "alpine:3.19")
		})
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	containerID, err := StartDevcontainer(startCtx, WithConfigPath(configPath))
	if err != nil {
		t.Fatalf("StartDevcontainer: %v", err)
	}
	t.Cleanup(func() {
		cleanupContainer(t, cli, containerID)
	})

	inspect, err := cli.ContainerInspect(context.Background(), containerID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatalf("ContainerInspect: %v", err)
	}
	if inspect.Container.State == nil || !inspect.Container.State.Running {
		t.Fatalf("container is not running")
	}

	if err := StopDevcontainer(context.Background(), containerID, 10*time.Second); err != nil {
		t.Fatalf("StopDevcontainer: %v", err)
	}
	if err := RemoveDevcontainer(context.Background(), containerID); err != nil {
		t.Fatalf("RemoveDevcontainer: %v", err)
	}
}

func TestDockerEngine_BuildImageFromDevcontainer(t *testing.T) {
	cli := requireDocker(t)
	configPath := testcasePath(t, "docker-engine-build", ".devcontainer", "devcontainer.json")

	inspectCtx, cancelInspect := context.WithTimeout(context.Background(), 10*time.Second)
	removeBaseImage := false
	if _, err := cli.ImageInspect(inspectCtx, "alpine:3.19"); err != nil {
		removeBaseImage = true
	}
	cancelInspect()
	if removeBaseImage {
		t.Cleanup(func() {
			cleanupImage(t, cli, "alpine:3.19")
		})
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	workspaceRoot, _, _, vars, err := resolveWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveWorkspacePaths: %v", err)
	}
	expectedTag := imageTagForBuild(workspaceRoot, vars["devcontainerId"])
	t.Cleanup(func() {
		cleanupImage(t, cli, expectedTag)
	})

	buildCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	imageRef, err := BuildImageFromDevcontainer(buildCtx, configPath)
	if err != nil {
		t.Fatalf("BuildImageFromDevcontainer: %v", err)
	}
	if imageRef != expectedTag {
		t.Fatalf("unexpected image tag: %s", imageRef)
	}
	if _, err := cli.ImageInspect(context.Background(), imageRef); err != nil {
		t.Fatalf("ImageInspect: %v", err)
	}
}
