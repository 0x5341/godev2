package devcontainer

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-units"
)

func StartDevcontainer(ctx context.Context, opts ...StartOption) (string, error) {
	options := defaultStartOptions()
	for _, opt := range opts {
		opt(&options)
	}

	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}

	configPath, err := resolveConfigPath(options.ConfigPath)
	if err != nil {
		return "", err
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return "", err
	}
	if err := validateConfig(cfg); err != nil {
		return "", err
	}
	if isComposeConfig(cfg) {
		return startComposeDevcontainer(ctx, configPath, cfg, options)
	}

	workspaceRoot, workspaceFolder, workspaceMount, vars, err := resolveWorkspacePaths(configPath, cfg)
	if err != nil {
		return "", err
	}

	envMap, err := mergeEnvMaps(cfg.ContainerEnv, options.Env, vars)
	if err != nil {
		return "", err
	}
	if err := runLifecycleCommands(ctx, "initializeCommand", cfg.InitializeCommand, hostLifecycleRunner(workspaceRoot, vars, envMap)); err != nil {
		return "", err
	}

	cli, err := newDockerClient()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = cli.Close()
	}()

	imageRef, err := ensureImage(ctx, cli, cfg, configPath, workspaceRoot, vars["devcontainerId"])
	if err != nil {
		return "", err
	}

	runArgOptions, err := parseRunArgs(append(cfg.RunArgs, options.RunArgs...))
	if err != nil {
		return "", err
	}

	portSpecs, err := collectPortSpecs(cfg.ForwardPorts, cfg.AppPort, options.ExtraPublish)
	if err != nil {
		return "", err
	}
	exposedPorts, portBindings, err := parsePortSpecs(portSpecs)
	if err != nil {
		return "", err
	}

	mounts, err := buildMounts(workspaceMount, cfg.Mounts, options.ExtraMounts, vars)
	if err != nil {
		return "", err
	}

	labels := mergeLabels(options.Labels, runArgOptions.Labels)
	labels["devcontainer.config_path"] = configPath

	workingDir := workspaceFolder
	if options.Workdir != "" {
		workingDir = options.Workdir
	}

	containerConfig := &container.Config{
		Image:        imageRef,
		Env:          envMapToSlice(envMap),
		ExposedPorts: exposedPorts,
		WorkingDir:   workingDir,
		Tty:          options.TTY,
		User:         cfg.ContainerUser,
		Labels:       labels,
	}

	if runArgOptions.User != "" {
		containerConfig.User = runArgOptions.User
	}

	overrideCommand := true
	if cfg.OverrideCommand != nil {
		overrideCommand = *cfg.OverrideCommand
	}
	if overrideCommand {
		containerConfig.Cmd = []string{"/bin/sh", "-c", "while sleep 1000; do :; done"}
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Mounts:       mounts,
		AutoRemove:   options.RemoveOnStop,
		Privileged:   cfg.Privileged || runArgOptions.Privileged,
		CapAdd:       append([]string{}, cfg.CapAdd...),
		SecurityOpt:  append([]string{}, cfg.SecurityOpt...),
	}

	if runArgOptions.Init {
		hostConfig.Init = &runArgOptions.Init
	} else if cfg.Init != nil {
		hostConfig.Init = cfg.Init
	}

	if len(runArgOptions.CapAdd) > 0 {
		hostConfig.CapAdd = append(hostConfig.CapAdd, runArgOptions.CapAdd...)
	}
	if len(runArgOptions.SecurityOpt) > 0 {
		hostConfig.SecurityOpt = append(hostConfig.SecurityOpt, runArgOptions.SecurityOpt...)
	}

	if options.Network != "" {
		hostConfig.NetworkMode = container.NetworkMode(options.Network)
	} else if runArgOptions.Network != "" {
		hostConfig.NetworkMode = container.NetworkMode(runArgOptions.Network)
	}

	if options.Resources.CPUQuota != 0 {
		hostConfig.CPUQuota = options.Resources.CPUQuota
	}
	if options.Resources.Memory != "" {
		bytes, err := units.RAMInBytes(options.Resources.Memory)
		if err != nil {
			return "", err
		}
		hostConfig.Memory = bytes
	}

	containerName := resolveContainerName(cfg.Name, workspaceRoot, vars["devcontainerId"])
	created, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", err
	}

	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return created.ID, err
	}

	lifecycleEnv, err := buildLifecycleEnv(envMap, cfg.RemoteEnv, vars)
	if err != nil {
		return created.ID, err
	}
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		if runArgOptions.User != "" {
			remoteUser = runArgOptions.User
		} else {
			remoteUser = cfg.ContainerUser
		}
	}
	hooks := []lifecycleHook{
		{Name: "onCreateCommand", Commands: cfg.OnCreateCommand},
		{Name: "updateContentCommand", Commands: cfg.UpdateContentCommand},
		{Name: "postCreateCommand", Commands: cfg.PostCreateCommand},
		{Name: "postStartCommand", Commands: cfg.PostStartCommand},
		{Name: "postAttachCommand", Commands: cfg.PostAttachCommand},
	}
	runner := containerLifecycleRunner(cli, created.ID, workspaceFolder, remoteUser, vars, envMap, envMapToSlice(lifecycleEnv))
	if err := runLifecycleSequence(ctx, hooks, runner); err != nil {
		return created.ID, err
	}

	if !options.Detach {
		statusCh, errCh := cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				return created.ID, err
			}
		case status := <-statusCh:
			if status.StatusCode != 0 {
				return created.ID, fmt.Errorf("container exited with status %d", status.StatusCode)
			}
		}
	}

	return created.ID, nil
}

func StopDevcontainer(ctx context.Context, containerID string, timeout time.Duration) error {
	cli, err := newDockerClient()
	if err != nil {
		return err
	}
	defer func() {
		_ = cli.Close()
	}()

	if timeout <= 0 {
		return cli.ContainerStop(ctx, containerID, container.StopOptions{})
	}
	timeoutSeconds := int(timeout.Seconds())
	return cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeoutSeconds})
}

func RemoveDevcontainer(ctx context.Context, containerID string) error {
	cli, err := newDockerClient()
	if err != nil {
		return err
	}
	defer func() {
		_ = cli.Close()
	}()

	return cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true, RemoveVolumes: true})
}

func BuildImageFromDevcontainer(ctx context.Context, configPath string) (string, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return "", err
	}
	if err := validateConfig(cfg); err != nil {
		return "", err
	}
	if isComposeConfig(cfg) {
		return "", errors.New("docker compose build is not supported")
	}
	workspaceRoot, _, _, vars, err := resolveWorkspacePaths(configPath, cfg)
	if err != nil {
		return "", err
	}
	cli, err := newDockerClient()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = cli.Close()
	}()
	return buildImage(ctx, cli, cfg, configPath, workspaceRoot, vars["devcontainerId"])
}

func buildMounts(workspaceMount string, configMounts []MountSpec, extraMounts []Mount, vars map[string]string) ([]mount.Mount, error) {
	expandedWorkspace, err := expandVariables(workspaceMount, vars, nil)
	if err != nil {
		return nil, err
	}
	workspaceParsed, err := parseMountString(expandedWorkspace)
	if err != nil {
		return nil, err
	}
	mounts := []mount.Mount{workspaceParsed}

	for _, spec := range configMounts {
		if spec.Raw != "" {
			expanded, err := expandVariables(spec.Raw, vars, nil)
			if err != nil {
				return nil, err
			}
			parsed, err := parseMountString(expanded)
			if err != nil {
				return nil, err
			}
			mounts = append(mounts, parsed)
			continue
		}
		parsed, err := mountFromSpec(spec)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, parsed)
	}

	for _, extra := range extraMounts {
		parsed, err := toDockerMount(extra)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, parsed)
	}
	return mounts, nil
}

func resolveConfigPath(path string) (string, error) {
	if path != "" {
		return filepath.Abs(path)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return FindConfigPath(cwd)
}

func ensureImage(ctx context.Context, cli *client.Client, cfg *DevcontainerConfig, configPath, workspaceRoot, devcontainerID string) (string, error) {
	if cfg.Image != "" && cfg.Build != nil {
		return "", errors.New("both image and build are set in devcontainer.json")
	}
	if cfg.Image == "" && cfg.Build == nil {
		return "", errors.New("devcontainer.json must specify image or build")
	}
	if cfg.Image != "" {
		if err := pullImage(ctx, cli, cfg.Image); err != nil {
			return "", err
		}
		return cfg.Image, nil
	}
	return buildImage(ctx, cli, cfg, configPath, workspaceRoot, devcontainerID)
}

func buildImage(ctx context.Context, cli *client.Client, cfg *DevcontainerConfig, configPath, workspaceRoot, devcontainerID string) (string, error) {
	if cfg.Build == nil {
		return "", errors.New("build config is required")
	}
	if len(cfg.Build.Options) > 0 {
		return "", errors.New("build.options is not supported yet")
	}
	contextDir, dockerfileRel, err := resolveBuildPaths(configPath, cfg.Build)
	if err != nil {
		return "", err
	}
	buildContext, err := tarDirectory(contextDir)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = buildContext.Close()
	}()

	tag := imageTagForBuild(workspaceRoot, devcontainerID)
	buildArgs := make(map[string]*string, len(cfg.Build.Args))
	for key, value := range cfg.Build.Args {
		val := value
		buildArgs[key] = &val
	}

	resp, err := cli.ImageBuild(ctx, buildContext, build.ImageBuildOptions{
		Dockerfile: dockerfileRel,
		Tags:       []string{tag},
		Remove:     true,
		BuildArgs:  buildArgs,
		Target:     cfg.Build.Target,
		CacheFrom:  []string(cfg.Build.CacheFrom),
	})
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return "", err
	}
	return tag, nil
}

func tarDirectory(dir string) (io.ReadCloser, error) {
	pipeReader, pipeWriter := io.Pipe()
	tarWriter := tar.NewWriter(pipeWriter)

	go func() {
		defer func() {
			_ = tarWriter.Close()
			_ = pipeWriter.Close()
		}()
		walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == dir {
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
			header.Name = filepath.ToSlash(rel)
			if entry.Type()&os.ModeSymlink != 0 {
				link, err := os.Readlink(path)
				if err != nil {
					return err
				}
				header.Linkname = link
			}
			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tarWriter, file); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			return nil
		})
		if walkErr != nil {
			_ = pipeWriter.CloseWithError(walkErr)
		}
	}()

	return pipeReader, nil
}

func pullImage(ctx context.Context, cli *client.Client, imageRef string) error {
	reader, err := cli.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()
	_, err = io.Copy(io.Discard, reader)
	return err
}

func resolveBuildPaths(configPath string, build *DevcontainerBuild) (string, string, error) {
	configDir := filepath.Dir(configPath)
	contextPath := build.Context
	if contextPath == "" {
		contextPath = "."
	}
	contextDir := filepath.Clean(filepath.Join(configDir, contextPath))
	if build.Dockerfile == "" {
		return "", "", errors.New("build.dockerfile is required")
	}
	dockerfilePath := filepath.Clean(filepath.Join(configDir, build.Dockerfile))
	rel, err := filepath.Rel(contextDir, dockerfilePath)
	if err != nil {
		return "", "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", "", errors.New("dockerfile is outside build context")
	}
	return contextDir, filepath.ToSlash(rel), nil
}

func imageTagForBuild(workspaceRoot, devcontainerID string) string {
	base := sanitizeName(filepath.Base(workspaceRoot))
	if base == "" {
		base = "devcontainer"
	}
	return fmt.Sprintf("godev2-%s-%s:latest", base, devcontainerID)
}

func sanitizeName(name string) string {
	allowed := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '-'
		}
	}
	clean := strings.Map(allowed, name)
	clean = strings.Trim(clean, "-")
	return clean
}

func resolveContainerName(configName, workspaceRoot, devcontainerID string) string {
	if configName != "" {
		return sanitizeName(configName)
	}
	base := sanitizeName(filepath.Base(workspaceRoot))
	if base == "" {
		base = "devcontainer"
	}
	return fmt.Sprintf("godev2-%s-%s", base, devcontainerID)
}

func mergeLabels(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string)
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	if len(merged) == 0 {
		return map[string]string{}
	}
	return merged
}

func newDockerClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}
