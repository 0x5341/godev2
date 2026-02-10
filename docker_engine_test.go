package devcontainer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

func requireDocker(t *testing.T) *client.Client {
	t.Helper()
	cli, err := newDockerClient()
	if err != nil {
		t.Skipf("docker client unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
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
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from test location")
		}
		dir = parent
	}
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

func readTestcaseFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := testcasePath(t, parts...)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read testcase file: %v", err)
	}
	return content
}

func writeTestcaseFile(t *testing.T, dest string, parts ...string) {
	t.Helper()
	content := readTestcaseFile(t, parts...)
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		t.Fatalf("write testcase file: %v", err)
	}
}

func copyTestcaseDir(t *testing.T, dest string, parts ...string) {
	t.Helper()
	source := testcasePath(t, parts...)
	if err := copyDir(source, dest); err != nil {
		t.Fatalf("copy testcase dir: %v", err)
	}
}

func cleanupContainer(t *testing.T, cli *client.Client, containerID string) {
	t.Helper()
	if containerID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
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
	if _, err := cli.ImageRemove(ctx, imageRef, image.RemoveOptions{
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

	inspect, err := cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		t.Fatalf("ContainerInspect: %v", err)
	}
	if inspect.State == nil || !inspect.State.Running {
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

func TestDockerEngine_LifecycleCommands(t *testing.T) {
	cli := requireDocker(t)
	root := t.TempDir()
	copyTestcaseDir(t, root, "docker-engine-lifecycle")
	configPath := filepath.Join(root, ".devcontainer", "devcontainer.json")

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

	initContent, err := os.ReadFile(filepath.Join(root, "init.log"))
	if err != nil {
		t.Fatalf("read init.log: %v", err)
	}
	if strings.TrimSpace(string(initContent)) != "init" {
		t.Fatalf("unexpected init.log: %s", initContent)
	}

	lifecycleContent, err := os.ReadFile(filepath.Join(root, "lifecycle.log"))
	if err != nil {
		t.Fatalf("read lifecycle.log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(lifecycleContent)), "\n")
	expected := []string{"onCreate", "updateContent", "postCreate", "postStart", "postAttach"}
	if len(got) != len(expected) {
		t.Fatalf("unexpected lifecycle order: %#v", got)
	}
	for i, value := range expected {
		if got[i] != value {
			t.Fatalf("unexpected lifecycle order: %#v", got)
		}
	}
}
