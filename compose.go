package devcontainer

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

func isComposeConfig(cfg *DevcontainerConfig) bool {
	return len(cfg.DockerComposeFile) > 0 || cfg.Service != ""
}

func validateConfig(cfg *DevcontainerConfig) error {
	if isComposeConfig(cfg) {
		if len(cfg.DockerComposeFile) == 0 {
			return errors.New("dockerComposeFile is required when using docker compose")
		}
		if cfg.Service == "" {
			return errors.New("service is required when using docker compose")
		}
		if cfg.Image != "" || cfg.Build != nil {
			return errors.New("dockerComposeFile cannot be combined with image or build")
		}
		return nil
	}
	if cfg.Image == "" && cfg.Build == nil {
		return errors.New("devcontainer.json must specify image or build")
	}
	return nil
}

func resolveComposeWorkspacePaths(configPath string, cfg *DevcontainerConfig) (string, string, map[string]string, error) {
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return "", "", nil, err
	}
	configDir := filepath.Dir(absConfig)
	workspaceRoot := configDir
	if filepath.Base(configDir) == ".devcontainer" {
		workspaceRoot = filepath.Dir(configDir)
	}
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		return "", "", nil, err
	}
	workspaceFolder := cfg.WorkspaceFolder
	if workspaceFolder == "" {
		workspaceFolder = "/"
	}
	devcontainerID := devcontainerID(workspaceRoot, absConfig)
	vars := map[string]string{
		"localWorkspaceFolder":             workspaceRoot,
		"localWorkspaceFolderBasename":     filepath.Base(workspaceRoot),
		"containerWorkspaceFolder":         workspaceFolder,
		"containerWorkspaceFolderBasename": path.Base(workspaceFolder),
		"devcontainerId":                   devcontainerID,
	}
	return workspaceRoot, workspaceFolder, vars, nil
}

func resolveComposeFiles(configPath string, cfg *DevcontainerConfig) ([]string, error) {
	if len(cfg.DockerComposeFile) == 0 {
		return nil, errors.New("dockerComposeFile is required when using docker compose")
	}
	configDir := filepath.Dir(configPath)
	files := make([]string, 0, len(cfg.DockerComposeFile))
	for _, file := range cfg.DockerComposeFile {
		if file == "" {
			return nil, errors.New("dockerComposeFile entry cannot be empty")
		}
		abs := filepath.Clean(filepath.Join(configDir, file))
		abs, err := filepath.Abs(abs)
		if err != nil {
			return nil, err
		}
		stat, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("docker compose file not found: %s", file)
		}
		if stat.IsDir() {
			return nil, fmt.Errorf("docker compose file is a directory: %s", file)
		}
		files = append(files, abs)
	}
	return files, nil
}
