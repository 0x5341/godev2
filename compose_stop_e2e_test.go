package devcontainer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerCompose_StopRemove_Dispatcher(t *testing.T) {
	cli := requireDocker(t)
	requireDockerCompose(t)
	pre := countDockerResources(t, cli)
	containerID := ""
	baseImage := "alpine:3.19"
	removeBaseImage := false

	root := t.TempDir()
	copyTestcaseDir(t, root, "compose", "stop-remove")
	configPath := filepath.Join(root, ".devcontainer", "devcontainer.json")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	workspaceRoot, _, vars, err := resolveComposeWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveComposeWorkspacePaths: %v", err)
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

	dbContainerID, err := composePrimaryContainerID(context.Background(), workspaceRoot, projectName, composeFiles, "", "db")
	if err != nil {
		t.Fatalf("composePrimaryContainerID: %v", err)
	}
	dbInspect, err := cli.ContainerInspect(context.Background(), dbContainerID)
	if err != nil {
		t.Fatalf("ContainerInspect db: %v", err)
	}
	if dbInspect.State == nil || !dbInspect.State.Running {
		t.Fatalf("db container is not running")
	}

	if err := StopDevcontainer(context.Background(), containerID, 10*time.Second); err != nil {
		t.Fatalf("StopDevcontainer: %v", err)
	}
	appInspect, err := cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		t.Fatalf("ContainerInspect app: %v", err)
	}
	if appInspect.State == nil || appInspect.State.Running {
		t.Fatalf("app container still running")
	}
	dbInspect, err = cli.ContainerInspect(context.Background(), dbContainerID)
	if err != nil {
		t.Fatalf("ContainerInspect db: %v", err)
	}
	if dbInspect.State == nil || dbInspect.State.Running {
		t.Fatalf("db container still running")
	}

	if err := RemoveDevcontainer(context.Background(), containerID); err != nil {
		t.Fatalf("RemoveDevcontainer: %v", err)
	}
	containerID = ""
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCheck()
	args := composeBaseArgs(workspaceRoot, projectName, composeFiles, "")
	args = append(args, "ps", "-q")
	output, err := runDockerCompose(checkCtx, workspaceRoot, args)
	if err != nil {
		t.Fatalf("compose ps: %v", err)
	}
	if strings.TrimSpace(output) != "" {
		t.Fatalf("compose containers still exist: %s", strings.TrimSpace(output))
	}
	if removeBaseImage {
		cleanupImage(t, cli, baseImage)
		baseImage = ""
		removeBaseImage = false
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
