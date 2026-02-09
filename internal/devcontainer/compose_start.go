package devcontainer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types/container"
	"gopkg.in/yaml.v3"
)

func startComposeDevcontainer(ctx context.Context, configPath string, cfg *DevcontainerConfig, options startOptions) (string, error) {
	if err := validateComposeOptions(options); err != nil {
		return "", err
	}

	workspaceRoot, workspaceFolder, vars, err := resolveComposeWorkspacePaths(configPath, cfg)
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
	composeFiles, err := resolveComposeFiles(configPath, cfg)
	if err != nil {
		return "", err
	}

	projectName := resolveComposeProjectName(cfg, workspaceRoot, vars["devcontainerId"])
	project, err := loadComposeProject(ctx, composeFiles, workspaceRoot, projectName)
	if err != nil {
		return "", err
	}

	labels := mergeLabels(options.Labels, nil)
	labels["devcontainer.config_path"] = configPath

	service, err := findComposeService(project, cfg.Service)
	if err != nil {
		return "", err
	}
	override, err := buildComposeOverride(cfg, envMap, labels, workspaceFolder, service)
	if err != nil {
		return "", err
	}
	overrideFile, err := writeComposeOverride(override)
	if err != nil {
		return "", err
	}
	if overrideFile != "" {
		defer func() {
			_ = os.Remove(overrideFile)
		}()
	}
	if err := composeUp(ctx, workspaceRoot, project.Name, composeFiles, overrideFile, cfg.RunServices); err != nil {
		return "", err
	}
	containerID, err := composePrimaryContainerID(ctx, workspaceRoot, project.Name, composeFiles, overrideFile, cfg.Service)
	if err != nil {
		return "", err
	}
	cli, err := newDockerClient()
	if err != nil {
		return containerID, err
	}
	defer func() {
		_ = cli.Close()
	}()
	lifecycleEnv, err := buildLifecycleEnv(envMap, cfg.RemoteEnv, vars)
	if err != nil {
		return containerID, err
	}
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = cfg.ContainerUser
	}
	hooks := []lifecycleHook{
		{Name: "onCreateCommand", Commands: cfg.OnCreateCommand},
		{Name: "updateContentCommand", Commands: cfg.UpdateContentCommand},
		{Name: "postCreateCommand", Commands: cfg.PostCreateCommand},
		{Name: "postStartCommand", Commands: cfg.PostStartCommand},
		{Name: "postAttachCommand", Commands: cfg.PostAttachCommand},
	}
	runner := containerLifecycleRunner(cli, containerID, workspaceFolder, remoteUser, vars, envMap, envMapToSlice(lifecycleEnv))
	if err := runLifecycleSequence(ctx, hooks, runner); err != nil {
		return containerID, err
	}
	if !options.Detach {
		if err := waitForContainerExit(ctx, containerID); err != nil {
			return containerID, err
		}
	}
	return containerID, nil
}

func validateComposeOptions(options startOptions) error {
	if len(options.ExtraPublish) > 0 {
		return errors.New("compose does not support extra publishes")
	}
	if len(options.ExtraMounts) > 0 {
		return errors.New("compose does not support extra mounts")
	}
	if len(options.RunArgs) > 0 {
		return errors.New("compose does not support runArgs")
	}
	if options.Network != "" {
		return errors.New("compose does not support network override")
	}
	if options.Workdir != "" {
		return errors.New("compose does not support workdir override")
	}
	if options.Resources.CPUQuota != 0 || options.Resources.Memory != "" {
		return errors.New("compose does not support resource limits")
	}
	return nil
}

func resolveComposeProjectName(cfg *DevcontainerConfig, workspaceRoot, devcontainerID string) string {
	if cfg.Name != "" {
		return sanitizeName(cfg.Name)
	}
	base := sanitizeName(filepath.Base(workspaceRoot))
	if base == "" {
		base = "devcontainer"
	}
	return fmt.Sprintf("godev2-%s-%s", base, devcontainerID)
}

func loadComposeProject(ctx context.Context, composeFiles []string, workingDir, projectName string) (*types.Project, error) {
	configFiles := make([]types.ConfigFile, 0, len(composeFiles))
	for _, file := range composeFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		configFiles = append(configFiles, types.ConfigFile{
			Filename: file,
			Content:  content,
		})
	}
	env, err := loadComposeEnvironment(workingDir)
	if err != nil {
		return nil, err
	}
	configDetails := types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configFiles,
		Environment: env,
	}
	project, err := loader.LoadWithContext(ctx, configDetails)
	if err != nil {
		return nil, err
	}
	project.Name = projectName
	return project, nil
}

func findComposeService(project *types.Project, serviceName string) (*types.ServiceConfig, error) {
	for i, service := range project.Services {
		if service.Name == serviceName {
			return &project.Services[i], nil
		}
	}
	return nil, fmt.Errorf("service %s not found in compose project", serviceName)
}

func buildComposeOverride(cfg *DevcontainerConfig, envMap map[string]string, labels map[string]string, workspaceFolder string, service *types.ServiceConfig) ([]byte, error) {
	serviceOverride := make(map[string]any)
	if len(envMap) > 0 {
		serviceOverride["environment"] = envMap
	}
	if len(labels) > 0 {
		serviceOverride["labels"] = labels
	}
	if cfg.ContainerUser != "" {
		serviceOverride["user"] = cfg.ContainerUser
	}
	overrideCommand := false
	if cfg.OverrideCommand != nil {
		overrideCommand = *cfg.OverrideCommand
	}
	if overrideCommand {
		serviceOverride["command"] = []string{"/bin/sh", "-c", "while sleep 1000; do :; done"}
	}
	if workspaceFolder != "" && service.WorkingDir == "" {
		serviceOverride["working_dir"] = workspaceFolder
	}
	if len(serviceOverride) == 0 {
		return nil, nil
	}
	override := map[string]any{
		"services": map[string]any{
			cfg.Service: serviceOverride,
		},
	}
	return yaml.Marshal(override)
}

func writeComposeOverride(content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}
	file, err := os.CreateTemp("", "godev2-compose-override-*.yml")
	if err != nil {
		return "", err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return file.Name(), nil
}

func composeUp(ctx context.Context, projectDir, projectName string, composeFiles []string, overrideFile string, services []string) error {
	args := composeBaseArgs(projectDir, projectName, composeFiles, overrideFile)
	args = append(args, "up", "-d")
	if len(services) > 0 {
		args = append(args, services...)
	}
	_, err := runDockerCompose(ctx, projectDir, args)
	return err
}

func composePrimaryContainerID(ctx context.Context, projectDir, projectName string, composeFiles []string, overrideFile, serviceName string) (string, error) {
	args := composeBaseArgs(projectDir, projectName, composeFiles, overrideFile)
	args = append(args, "ps", "-q", serviceName)
	output, err := runDockerCompose(ctx, projectDir, args)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(output)
	if id == "" {
		return "", fmt.Errorf("primary service container not found: %s", serviceName)
	}
	lines := strings.Split(id, "\n")
	return strings.TrimSpace(lines[0]), nil
}

func composeBaseArgs(projectDir, projectName string, composeFiles []string, overrideFile string) []string {
	args := []string{"compose"}
	for _, file := range composeFiles {
		args = append(args, "-f", file)
	}
	if overrideFile != "" {
		args = append(args, "-f", overrideFile)
	}
	args = append(args, "--project-directory", projectDir, "-p", projectName)
	return args
}

func runDockerCompose(ctx context.Context, projectDir string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = projectDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("docker %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

func waitForContainerExit(ctx context.Context, containerID string) error {
	cli, err := newDockerClient()
	if err != nil {
		return err
	}
	defer func() {
		_ = cli.Close()
	}()
	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return err
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return fmt.Errorf("container exited with status %d", status.StatusCode)
		}
		return nil
	}
}

func loadComposeEnvironment(workingDir string) (map[string]string, error) {
	env := envFromOS()
	dotenvPath := filepath.Join(workingDir, ".env")
	fileEnv, err := parseDotEnvFile(dotenvPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return env, nil
		}
		return nil, err
	}
	for key, value := range fileEnv {
		if _, ok := env[key]; ok {
			continue
		}
		env[key] = value
	}
	return env, nil
}

func envFromOS() map[string]string {
	env := make(map[string]string)
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		env[parts[0]] = parts[1]
	}
	return env
}

func parseDotEnvFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	env := make(map[string]string)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid .env line: %s", raw)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\r")
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		env[key] = value
	}
	return env, nil
}
