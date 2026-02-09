package devcontainer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveComposeWorkspacePaths_Defaults(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"docker-compose.yml","service":"app"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	composePath := filepath.Join(configDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:3.19\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	cfg := &DevcontainerConfig{
		DockerComposeFile: StringSlice{"docker-compose.yml"},
		Service:           "app",
	}

	workspaceRoot, workspaceFolder, vars, err := resolveComposeWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveComposeWorkspacePaths: %v", err)
	}
	if workspaceRoot != root {
		t.Fatalf("expected workspaceRoot %s, got %s", root, workspaceRoot)
	}
	if workspaceFolder != "/" {
		t.Fatalf("expected workspaceFolder '/', got %s", workspaceFolder)
	}
	if vars["containerWorkspaceFolder"] != "/" {
		t.Fatalf("unexpected vars: %#v", vars)
	}
}

func TestResolveComposeWorkspacePaths_ConfigInRoot(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"compose.yml","service":"app","workspaceFolder":"/workspace"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	composePath := filepath.Join(root, "compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:3.19\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	cfg := &DevcontainerConfig{
		DockerComposeFile: StringSlice{"compose.yml"},
		Service:           "app",
		WorkspaceFolder:   "/workspace",
	}

	workspaceRoot, workspaceFolder, vars, err := resolveComposeWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveComposeWorkspacePaths: %v", err)
	}
	if workspaceRoot != root {
		t.Fatalf("expected workspaceRoot %s, got %s", root, workspaceRoot)
	}
	if workspaceFolder != "/workspace" {
		t.Fatalf("expected workspaceFolder '/workspace', got %s", workspaceFolder)
	}
	if vars["containerWorkspaceFolder"] != "/workspace" {
		t.Fatalf("unexpected vars: %#v", vars)
	}
}

func TestResolveComposeFiles(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":["compose.yml","compose.override.yml"],"service":"app"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	first := filepath.Join(configDir, "compose.yml")
	second := filepath.Join(configDir, "compose.override.yml")
	if err := os.WriteFile(first, []byte("services:\n  app:\n    image: alpine:3.19\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	if err := os.WriteFile(second, []byte("services:\n  app:\n    environment:\n      FOO: bar\n"), 0o644); err != nil {
		t.Fatalf("write compose override: %v", err)
	}

	cfg := &DevcontainerConfig{
		DockerComposeFile: StringSlice{"compose.yml", "compose.override.yml"},
		Service:           "app",
	}
	files, err := resolveComposeFiles(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveComposeFiles: %v", err)
	}
	if len(files) != 2 || files[0] != first || files[1] != second {
		t.Fatalf("unexpected compose files: %#v", files)
	}
}

func TestResolveComposeFiles_Errors(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"dockerComposeFile":"compose.yml","service":"app"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	tests := []struct {
		name  string
		file  string
		setup func(string) error
	}{
		{
			name: "empty entry",
			file: "",
		},
		{
			name: "missing file",
			file: "missing.yml",
		},
		{
			name: "directory path",
			file: "compose-dir",
			setup: func(path string) error {
				return os.MkdirAll(path, 0o755)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				if err := tt.setup(filepath.Join(configDir, tt.file)); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			cfg := &DevcontainerConfig{
				DockerComposeFile: StringSlice{tt.file},
				Service:           "app",
			}
			if _, err := resolveComposeFiles(configPath, cfg); err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}
}

func TestValidateConfig_ComposeRequiresService(t *testing.T) {
	cfg := &DevcontainerConfig{
		DockerComposeFile: StringSlice{"compose.yml"},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected error for missing service")
	}
}

func TestValidateConfig_ComposeRequiresComposeFile(t *testing.T) {
	cfg := &DevcontainerConfig{
		Service: "app",
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected error for missing dockerComposeFile")
	}
}

func TestValidateConfig_ComposeRejectsImage(t *testing.T) {
	cfg := &DevcontainerConfig{
		DockerComposeFile: StringSlice{"compose.yml"},
		Service:           "app",
		Image:             "alpine:3.19",
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected error for image with compose")
	}
}

func TestValidateConfig_ComposeRejectsBuild(t *testing.T) {
	cfg := &DevcontainerConfig{
		DockerComposeFile: StringSlice{"compose.yml"},
		Service:           "app",
		Build:             &DevcontainerBuild{Dockerfile: "Dockerfile"},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected error for build with compose")
	}
}
