package devcontainer

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerCompose_FeaturesImage(t *testing.T) {
	cli := requireDocker(t)
	requireDockerCompose(t)
	pre := countDockerResources(t, cli)
	containerID := ""
	featuresImage := ""
	baseImage := "alpine:3.19"
	removeBaseImage := false

	root := t.TempDir()
	copyTestcaseDir(t, root, "compose", "features-image")
	configPath := filepath.Join(root, ".devcontainer", "devcontainer.json")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	workspaceRoot, _, vars, err := resolveComposeWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveComposeWorkspacePaths: %v", err)
	}
	features, err := resolveFeatures(context.Background(), configPath, workspaceRoot, cfg)
	if err != nil {
		t.Fatalf("resolveFeatures: %v", err)
	}
	if features != nil {
		featuresImage = featuresImageTag(workspaceRoot, vars["devcontainerId"], features.Order)
	}
	composeFiles, err := resolveComposeFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveComposeFiles: %v", err)
	}
	projectName := resolveComposeProjectName(cfg, workspaceRoot, vars["devcontainerId"])

	inspectCtx, cancelInspect := context.WithTimeout(context.Background(), 10*time.Second)
	if _, err := cli.ImageInspect(inspectCtx, baseImage); err != nil {
		removeBaseImage = true
	}
	cancelInspect()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = composeDown(ctx, workspaceRoot, projectName, composeFiles)
		cleanupContainer(t, cli, containerID)
		cleanupImage(t, cli, featuresImage)
		if removeBaseImage {
			cleanupImage(t, cli, baseImage)
		}
	})

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
	logOutput := execContainer(t, cli, containerID, []string{"cat", "/feature.log"})
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	if len(lines) != 2 || lines[0] != "feature" || lines[1] != "user" {
		t.Fatalf("unexpected feature log: %#v", lines)
	}

	downCtx, cancelDown := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelDown()
	if err := composeDown(downCtx, workspaceRoot, projectName, composeFiles); err != nil {
		t.Fatalf("compose down: %v", err)
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

func requireDockerCompose(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		t.Skipf("docker compose unavailable: %v", err)
	}
}

func composeDown(ctx context.Context, projectDir, projectName string, composeFiles []string) error {
	args := composeBaseArgs(projectDir, projectName, composeFiles, "")
	args = append(args, "down", "--volumes", "--remove-orphans")
	_, err := runDockerCompose(ctx, projectDir, args)
	return err
}
