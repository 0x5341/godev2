package devcontainer

import (
	"bytes"
	"context"
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
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	featureDir := filepath.Join(configDir, "feature-a")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatalf("mkdir feature: %v", err)
	}
	featureMeta := `{
		"id": "feature-a",
		"version": "1.0.0",
		"name": "Feature A",
		"options": {
			"message": {
				"type": "string",
				"default": "default"
			},
			"flag": {
				"type": "boolean",
				"default": true
			}
		},
		"postCreateCommand": "echo feature >> ${containerWorkspaceFolder}/feature.log"
	}`
	if err := os.WriteFile(filepath.Join(featureDir, "devcontainer-feature.json"), []byte(featureMeta), 0o644); err != nil {
		t.Fatalf("write feature metadata: %v", err)
	}
	install := "#!/bin/sh\nset -e\nprintf 'message=%s flag=%s' \"$MESSAGE\" \"$FLAG\" > /tmp/feature-installed\n"
	if err := os.WriteFile(filepath.Join(featureDir, "install.sh"), []byte(install), 0o755); err != nil {
		t.Fatalf("write install.sh: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"image": "alpine:3.19",
		"features": {
			"./feature-a": {
				"message": "hello",
				"flag": true
			}
		},
		"postCreateCommand": "echo user >> ${containerWorkspaceFolder}/feature.log"
	}`
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
