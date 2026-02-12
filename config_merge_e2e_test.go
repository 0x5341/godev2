package godev

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerEngine_ConfigStructMerge(t *testing.T) {
	cli := requireDocker(t)
	pre := countDockerResources(t, cli)
	containerID := ""
	baseImage := "alpine:3.19"
	removeBaseImage := false
	t.Cleanup(func() {
		cleanupContainer(t, cli, containerID)
		if removeBaseImage {
			cleanupImage(t, cli, baseImage)
		}
	})

	root := t.TempDir()
	copyTestcaseDir(t, root, "docker-engine-merge")
	configPath := filepath.Join(root, ".devcontainer", "devcontainer.json")

	inspectCtx, cancelInspect := context.WithTimeout(context.Background(), 10*time.Second)
	if _, err := cli.ImageInspect(inspectCtx, baseImage); err != nil {
		removeBaseImage = true
	}
	cancelInspect()

	baseCfg := &DevcontainerConfig{
		Name:  "struct-base",
		Image: baseImage,
		ContainerEnv: map[string]string{
			"BASE":   "1",
			"SHARED": "base",
		},
	}
	overlayCfg := &DevcontainerConfig{
		Name: "struct-overlay",
		ContainerEnv: map[string]string{
			"OVERLAY": "1",
			"SHARED":  "overlay",
		},
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	containerID, err := StartDevcontainer(startCtx, WithConfigPath(configPath), WithConfig(baseCfg), WithMergeConfig(overlayCfg))
	if err != nil {
		t.Fatalf("StartDevcontainer: %v", err)
	}

	inspect, err := cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		t.Fatalf("ContainerInspect: %v", err)
	}
	name := strings.TrimPrefix(inspect.Name, "/")
	if name != "struct-overlay" {
		t.Fatalf("unexpected container name: %s", name)
	}
	envMap := envSliceToMap(inspect.Config.Env)
	if envMap["BASE"] != "1" || envMap["OVERLAY"] != "1" || envMap["SHARED"] != "overlay" {
		t.Fatalf("unexpected env: %#v", envMap)
	}

	if err := StopDevcontainer(context.Background(), containerID, 10*time.Second); err != nil {
		t.Fatalf("StopDevcontainer: %v", err)
	}
	if err := RemoveDevcontainer(context.Background(), containerID); err != nil {
		t.Fatalf("RemoveDevcontainer: %v", err)
	}
	containerID = ""
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

func envSliceToMap(values []string) map[string]string {
	envMap := make(map[string]string, len(values))
	for _, entry := range values {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		envMap[parts[0]] = parts[1]
	}
	return envMap
}
