package devcontainer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"gopkg.in/yaml.v3"
)

type composeOverride struct {
	Services map[string]composeServiceOverride `yaml:"services"`
}

type composeServiceOverride struct {
	Environment map[string]string `yaml:"environment"`
	Labels      map[string]string `yaml:"labels"`
	User        string            `yaml:"user"`
	Command     []string          `yaml:"command"`
	WorkingDir  string            `yaml:"working_dir"`
	Image       string            `yaml:"image"`
	Volumes     []string          `yaml:"volumes"`
	Privileged  *bool             `yaml:"privileged"`
	CapAdd      []string          `yaml:"cap_add"`
	SecurityOpt []string          `yaml:"security_opt"`
	Init        *bool             `yaml:"init"`
}

func TestBuildComposeOverride_PopulatesFields(t *testing.T) {
	cfg := &DevcontainerConfig{
		Service:         "app",
		ContainerUser:   "vscode",
		OverrideCommand: boolPtr(true),
	}
	envMap := map[string]string{"FOO": "bar"}
	labels := map[string]string{"devcontainer.config_path": "/path/to/devcontainer.json"}
	workspaceFolder := "/workspace"
	service := &types.ServiceConfig{Name: "app"}

	override, err := buildComposeOverride(cfg, envMap, labels, workspaceFolder, service, nil, "")
	if err != nil {
		t.Fatalf("buildComposeOverride: %v", err)
	}
	if len(override) == 0 {
		t.Fatal("expected override content")
	}

	var parsed composeOverride
	if err := yaml.Unmarshal(override, &parsed); err != nil {
		t.Fatalf("unmarshal override: %v", err)
	}
	serviceOverride, ok := parsed.Services["app"]
	if !ok {
		t.Fatalf("expected override for service app")
	}
	if serviceOverride.Environment["FOO"] != "bar" {
		t.Fatalf("unexpected environment: %#v", serviceOverride.Environment)
	}
	if serviceOverride.Labels["devcontainer.config_path"] != "/path/to/devcontainer.json" {
		t.Fatalf("unexpected labels: %#v", serviceOverride.Labels)
	}
	if serviceOverride.User != "vscode" {
		t.Fatalf("unexpected user: %s", serviceOverride.User)
	}
	command := []string{"/bin/sh", "-c", "while sleep 1000; do :; done"}
	if !reflect.DeepEqual(serviceOverride.Command, command) {
		t.Fatalf("unexpected command: %#v", serviceOverride.Command)
	}
	if serviceOverride.WorkingDir != workspaceFolder {
		t.Fatalf("unexpected working_dir: %s", serviceOverride.WorkingDir)
	}
}

func TestBuildComposeOverride_NoOverrides(t *testing.T) {
	cfg := &DevcontainerConfig{
		Service: "app",
	}
	service := &types.ServiceConfig{
		Name:       "app",
		WorkingDir: "/already-set",
	}

	override, err := buildComposeOverride(cfg, nil, nil, "/workspace", service, nil, "")
	if err != nil {
		t.Fatalf("buildComposeOverride: %v", err)
	}
	if override != nil {
		t.Fatalf("expected nil override, got %s", string(override))
	}
}

func TestComposeBaseArgs(t *testing.T) {
	projectDir := "/project"
	projectName := "godev2-project"
	composeFiles := []string{"/project/compose.yml", "/project/compose.override.yml"}

	tests := []struct {
		name     string
		override string
		wantArgs []string
	}{
		{
			name:     "with override file",
			override: "/tmp/override.yml",
			wantArgs: []string{"compose", "-f", composeFiles[0], "-f", composeFiles[1], "-f", "/tmp/override.yml", "--project-directory", projectDir, "-p", projectName},
		},
		{
			name:     "without override file",
			override: "",
			wantArgs: []string{"compose", "-f", composeFiles[0], "-f", composeFiles[1], "--project-directory", projectDir, "-p", projectName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := composeBaseArgs(projectDir, projectName, composeFiles, tt.override)
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("unexpected args: %#v", args)
			}
		})
	}
}

func TestBuildComposeOverride_Features(t *testing.T) {
	init := true
	cfg := &DevcontainerConfig{
		Service: "app",
	}
	envMap := map[string]string{"FOO": "bar"}
	labels := map[string]string{"devcontainer.config_path": "/path/to/devcontainer.json"}
	workspaceFolder := "/workspace"
	service := &types.ServiceConfig{Name: "app"}
	features := &ResolvedFeatures{
		Mounts:      []MountSpec{{Type: "volume", Source: "feature-cache", Target: "/cache"}},
		Privileged:  true,
		Init:        &init,
		CapAdd:      []string{"SYS_PTRACE"},
		SecurityOpt: []string{"label:role:ROLE"},
	}

	override, err := buildComposeOverride(cfg, envMap, labels, workspaceFolder, service, features, "feature-image:latest")
	if err != nil {
		t.Fatalf("buildComposeOverride: %v", err)
	}
	if len(override) == 0 {
		t.Fatal("expected override content")
	}

	var parsed composeOverride
	if err := yaml.Unmarshal(override, &parsed); err != nil {
		t.Fatalf("unmarshal override: %v", err)
	}
	serviceOverride, ok := parsed.Services["app"]
	if !ok {
		t.Fatalf("expected override for service app")
	}
	if serviceOverride.Environment["FOO"] != "bar" {
		t.Fatalf("unexpected environment: %#v", serviceOverride.Environment)
	}
	if serviceOverride.Labels["devcontainer.config_path"] != "/path/to/devcontainer.json" {
		t.Fatalf("unexpected labels: %#v", serviceOverride.Labels)
	}
	if serviceOverride.Image != "feature-image:latest" {
		t.Fatalf("unexpected image: %s", serviceOverride.Image)
	}
	if len(serviceOverride.Volumes) != 1 || serviceOverride.Volumes[0] != "feature-cache:/cache" {
		t.Fatalf("unexpected volumes: %#v", serviceOverride.Volumes)
	}
	if serviceOverride.Privileged == nil || !*serviceOverride.Privileged {
		t.Fatalf("expected privileged to be true")
	}
	if len(serviceOverride.CapAdd) != 1 || serviceOverride.CapAdd[0] != "SYS_PTRACE" {
		t.Fatalf("unexpected cap_add: %#v", serviceOverride.CapAdd)
	}
	if len(serviceOverride.SecurityOpt) != 1 || serviceOverride.SecurityOpt[0] != "label:role:ROLE" {
		t.Fatalf("unexpected security_opt: %#v", serviceOverride.SecurityOpt)
	}
	if serviceOverride.Init == nil || !*serviceOverride.Init {
		t.Fatalf("expected init to be true")
	}
}

func TestParseDotEnvFile_ParsesValues(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	content := "# comment\nexport FOO=bar\nBAR=\"baz\"\nBAZ='qux'\nEMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	env, err := parseDotEnvFile(path)
	if err != nil {
		t.Fatalf("parseDotEnvFile: %v", err)
	}
	if env["FOO"] != "bar" {
		t.Fatalf("unexpected FOO: %s", env["FOO"])
	}
	if env["BAR"] != "baz" {
		t.Fatalf("unexpected BAR: %s", env["BAR"])
	}
	if env["BAZ"] != "qux" {
		t.Fatalf("unexpected BAZ: %s", env["BAZ"])
	}
	if value, ok := env["EMPTY"]; !ok || value != "" {
		t.Fatalf("unexpected EMPTY: %#v", env["EMPTY"])
	}
}

func TestParseDotEnvFile_InvalidLine(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("INVALID"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if _, err := parseDotEnvFile(path); err == nil {
		t.Fatal("expected error for invalid .env line")
	}
}

func TestLoadComposeEnvironment_RespectsOS(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("FROM_OS=file\nFROM_FILE=bar\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv("FROM_OS", "os")

	env, err := loadComposeEnvironment(root)
	if err != nil {
		t.Fatalf("loadComposeEnvironment: %v", err)
	}
	if env["FROM_OS"] != "os" {
		t.Fatalf("unexpected FROM_OS: %s", env["FROM_OS"])
	}
	if env["FROM_FILE"] != "bar" {
		t.Fatalf("unexpected FROM_FILE: %s", env["FROM_FILE"])
	}
}

func TestLoadComposeEnvironment_NoEnvFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ONLY_OS", "1")

	env, err := loadComposeEnvironment(root)
	if err != nil {
		t.Fatalf("loadComposeEnvironment: %v", err)
	}
	if env["ONLY_OS"] != "1" {
		t.Fatalf("unexpected ONLY_OS: %s", env["ONLY_OS"])
	}
}

func TestValidateComposeOptions(t *testing.T) {
	tests := []struct {
		name    string
		options startOptions
		wantErr bool
	}{
		{
			name:    "defaults ok",
			options: startOptions{},
		},
		{
			name:    "extra publish",
			options: startOptions{ExtraPublish: []string{"3000:3000"}},
			wantErr: true,
		},
		{
			name:    "extra mounts",
			options: startOptions{ExtraMounts: []Mount{{Source: "/tmp", Target: "/data"}}},
			wantErr: true,
		},
		{
			name:    "run args",
			options: startOptions{RunArgs: []string{"--privileged"}},
			wantErr: true,
		},
		{
			name:    "network override",
			options: startOptions{Network: "bridge"},
			wantErr: true,
		},
		{
			name:    "workdir override",
			options: startOptions{Workdir: "/work"},
			wantErr: true,
		},
		{
			name:    "resource limits",
			options: startOptions{Resources: ResourceLimits{CPUQuota: 1000, Memory: "512m"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateComposeOptions(tt.options)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveComposeProjectName(t *testing.T) {
	cfg := &DevcontainerConfig{Name: "My App"}
	if got := resolveComposeProjectName(cfg, "/workspaces/demo", "deadbeef"); got != "My-App" {
		t.Fatalf("unexpected project name: %s", got)
	}

	cfg = &DevcontainerConfig{}
	if got := resolveComposeProjectName(cfg, "/workspaces/demo", "deadbeef"); got != "godev2-demo-deadbeef" {
		t.Fatalf("unexpected project name: %s", got)
	}
}

func TestLoadComposeProject_WithProjectName(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:3.19\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	project, err := loadComposeProject(context.Background(), []string{composePath}, root, "custom")
	if err != nil {
		t.Fatalf("loadComposeProject: %v", err)
	}
	if project.Name != "custom" {
		t.Fatalf("unexpected project name: %s", project.Name)
	}
}

func TestFindComposeService_NotFound(t *testing.T) {
	project := &types.Project{
		Services: []types.ServiceConfig{{Name: "app"}},
	}
	if _, err := findComposeService(project, "db"); err == nil {
		t.Fatalf("expected error for missing service")
	} else if !strings.Contains(err.Error(), "service db not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
