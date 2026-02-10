package devcontainer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/mount"
)

func TestExpandVariables(t *testing.T) {
	root := t.TempDir()
	vars := map[string]string{
		"localWorkspaceFolder":             root,
		"localWorkspaceFolderBasename":     filepath.Base(root),
		"containerWorkspaceFolder":         "/workspaces/test",
		"containerWorkspaceFolderBasename": "test",
		"devcontainerId":                   "deadbeef",
	}
	if err := os.Setenv("TEST_ENV", "value"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Unsetenv("TEST_ENV"); err != nil {
			t.Errorf("unset env: %v", err)
		}
	})

	input := "source=${localWorkspaceFolder},target=${containerWorkspaceFolder},env=${localEnv:TEST_ENV}"
	got, err := expandVariables(input, vars, nil)
	if err != nil {
		t.Fatalf("expandVariables: %v", err)
	}
	expected := "source=" + root + ",target=/workspaces/test,env=value"
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestParseMountString(t *testing.T) {
	spec := "type=bind,source=/tmp,target=/work,readonly,consistency=cached"
	parsed, err := parseMountString(spec)
	if err != nil {
		t.Fatalf("parseMountString: %v", err)
	}
	if parsed.Type != mount.TypeBind || parsed.Source != "/tmp" || parsed.Target != "/work" {
		t.Fatalf("unexpected mount: %#v", parsed)
	}
	if !parsed.ReadOnly || parsed.Consistency != mount.ConsistencyCached {
		t.Fatalf("unexpected mount flags: %#v", parsed)
	}
}

func TestParseRunArgs(t *testing.T) {
	opts, err := parseRunArgs([]string{
		"--cap-add=SYS_PTRACE",
		"--security-opt=seccomp=unconfined",
		"--privileged",
		"--init",
		"--user=1000",
		"--label=foo=bar",
		"--network=host",
	})
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if len(opts.CapAdd) != 1 || opts.CapAdd[0] != "SYS_PTRACE" {
		t.Fatalf("unexpected CapAdd: %#v", opts.CapAdd)
	}
	if len(opts.SecurityOpt) != 1 || opts.SecurityOpt[0] != "seccomp=unconfined" {
		t.Fatalf("unexpected SecurityOpt: %#v", opts.SecurityOpt)
	}
	if !opts.Privileged || !opts.Init || opts.User != "1000" || opts.Network != "host" {
		t.Fatalf("unexpected runArg options: %#v", opts)
	}
	if opts.Labels["foo"] != "bar" {
		t.Fatalf("unexpected labels: %#v", opts.Labels)
	}
}
