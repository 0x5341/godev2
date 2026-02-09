package devcontainer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfigPath(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	expected := filepath.Join(configDir, "devcontainer.json")
	if err := os.WriteFile(expected, []byte(`{"image":"alpine:3.19"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := FindConfigPath(root)
	if err != nil {
		t.Fatalf("FindConfigPath: %v", err)
	}
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}

	if err := os.Remove(expected); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove: %v", err)
	}
	alt := filepath.Join(root, "devcontainer.json")
	if err := os.WriteFile(alt, []byte(`{"image":"alpine:3.19"}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	got, err = FindConfigPath(root)
	if err != nil {
		t.Fatalf("FindConfigPath alt: %v", err)
	}
	if got != alt {
		t.Fatalf("expected %s, got %s", alt, got)
	}
}

func TestLoadConfig_ParsesPortsAndMounts(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"image": "alpine:3.19",
		"forwardPorts": [3000, "3001:3002"],
		"appPort": "4000",
		"containerEnv": {"FOO": "bar"},
		"mounts": [
			"type=bind,source=${localWorkspaceFolder},target=/work",
			{"type":"volume","source":"data","target":"/data"}
		]
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.ForwardPorts) != 2 || cfg.ForwardPorts[0] != "3000" || cfg.ForwardPorts[1] != "3001:3002" {
		t.Fatalf("unexpected forwardPorts: %#v", cfg.ForwardPorts)
	}
	if len(cfg.AppPort) != 1 || cfg.AppPort[0] != "4000" {
		t.Fatalf("unexpected appPort: %#v", cfg.AppPort)
	}
	if len(cfg.Mounts) != 2 {
		t.Fatalf("unexpected mounts: %#v", cfg.Mounts)
	}
	if cfg.Mounts[0].Raw == "" || cfg.Mounts[1].Type != "volume" || cfg.Mounts[1].Target != "/data" {
		t.Fatalf("unexpected mount spec: %#v", cfg.Mounts)
	}
}

func TestLoadConfig_ParsesComposeFields(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"dockerComposeFile": ["compose.yml", "compose.override.yml"],
		"service": "app",
		"runServices": ["app", "db"],
		"workspaceFolder": "/workspace"
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.DockerComposeFile) != 2 || cfg.DockerComposeFile[0] != "compose.yml" || cfg.DockerComposeFile[1] != "compose.override.yml" {
		t.Fatalf("unexpected dockerComposeFile: %#v", cfg.DockerComposeFile)
	}
	if cfg.Service != "app" {
		t.Fatalf("unexpected service: %q", cfg.Service)
	}
	if len(cfg.RunServices) != 2 || cfg.RunServices[0] != "app" || cfg.RunServices[1] != "db" {
		t.Fatalf("unexpected runServices: %#v", cfg.RunServices)
	}
	if cfg.WorkspaceFolder != "/workspace" {
		t.Fatalf("unexpected workspaceFolder: %q", cfg.WorkspaceFolder)
	}
}

func TestLoadConfig_ParsesLifecycleCommands(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"image": "alpine:3.19",
		"initializeCommand": "echo init",
		"onCreateCommand": ["echo", "create"],
		"postCreateCommand": {
			"alpha": "echo a",
			"beta": ["echo", "b"]
		}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.InitializeCommand == nil || cfg.InitializeCommand.Single == nil || cfg.InitializeCommand.Single.Shell != "echo init" {
		t.Fatalf("unexpected initializeCommand: %#v", cfg.InitializeCommand)
	}
	if cfg.OnCreateCommand == nil || cfg.OnCreateCommand.Single == nil || cfg.OnCreateCommand.Single.Exec[0] != "echo" {
		t.Fatalf("unexpected onCreateCommand: %#v", cfg.OnCreateCommand)
	}
	if cfg.PostCreateCommand == nil || len(cfg.PostCreateCommand.Parallel) != 2 {
		t.Fatalf("unexpected postCreateCommand: %#v", cfg.PostCreateCommand)
	}
}

func TestLoadConfig_ParsesFeatures(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"image": "alpine:3.19",
		"features": {
			"ghcr.io/user/repo/go": "1.18",
			"./localFeature": {
				"flag": true
			}
		},
		"overrideFeatureInstallOrder": ["ghcr.io/user/repo/go"]
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Features == nil || len(cfg.Features) != 2 {
		t.Fatalf("unexpected features: %#v", cfg.Features)
	}
	options, ok := cfg.Features["ghcr.io/user/repo/go"]
	if !ok {
		t.Fatalf("missing feature entry")
	}
	version := options["version"]
	if version.String == nil || *version.String != "1.18" {
		t.Fatalf("unexpected version: %#v", version)
	}
	if len(cfg.OverrideFeatureInstallOrder) != 1 || cfg.OverrideFeatureInstallOrder[0] != "ghcr.io/user/repo/go" {
		t.Fatalf("unexpected override order: %#v", cfg.OverrideFeatureInstallOrder)
	}
}

func TestLoadConfig_RejectsInvalidFeatureOption(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"image": "alpine:3.19",
		"features": {
			"ghcr.io/user/repo/go": {
				"flag": 123
			}
		}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadConfig(configPath); err == nil {
		t.Fatalf("expected error for invalid feature option")
	}
}
