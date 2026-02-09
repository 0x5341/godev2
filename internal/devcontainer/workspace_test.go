package devcontainer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspacePaths_Defaults(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	writeTestcaseFile(t, configPath, "config", "basic", "devcontainer.json")

	cfg := &DevcontainerConfig{}
	workspaceRoot, workspaceFolder, workspaceMount, vars, err := resolveWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveWorkspacePaths: %v", err)
	}
	if workspaceRoot != root {
		t.Fatalf("expected workspaceRoot %s, got %s", root, workspaceRoot)
	}
	if workspaceFolder != filepath.ToSlash(filepath.Join("/workspaces", filepath.Base(root))) {
		t.Fatalf("unexpected workspaceFolder: %s", workspaceFolder)
	}
	if workspaceMount == "" {
		t.Fatalf("workspaceMount should not be empty")
	}
	if vars["localWorkspaceFolder"] != workspaceRoot {
		t.Fatalf("unexpected vars: %#v", vars)
	}
}
