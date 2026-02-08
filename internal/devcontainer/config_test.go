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
