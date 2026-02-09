package devcontainer

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

func TestNormalizePortSpec(t *testing.T) {
	got, err := normalizePortSpec("3000")
	if err != nil {
		t.Fatalf("normalizePortSpec: %v", err)
	}
	if got != "3000:3000" {
		t.Fatalf("unexpected port spec: %s", got)
	}
	got, err = normalizePortSpec("3000/tcp")
	if err != nil {
		t.Fatalf("normalizePortSpec proto: %v", err)
	}
	if got != "3000:3000/tcp" {
		t.Fatalf("unexpected proto spec: %s", got)
	}
	if _, err := normalizePortSpec("invalid"); err == nil {
		t.Fatalf("expected error for invalid port spec")
	}
}

func TestParsePortSpecs(t *testing.T) {
	exposed, bindings, err := parsePortSpecs([]string{"3000:3000"})
	if err != nil {
		t.Fatalf("parsePortSpecs: %v", err)
	}
	if len(exposed) != 1 || len(bindings) != 1 {
		t.Fatalf("unexpected port maps: %#v %#v", exposed, bindings)
	}
	var port nat.Port
	for item := range exposed {
		port = item
	}
	if port.Proto() != "tcp" || port.Port() != "3000" {
		t.Fatalf("unexpected port: %s", port.Port())
	}
	portBindings := bindings[port]
	if len(portBindings) != 1 || portBindings[0].HostPort != "3000" {
		t.Fatalf("unexpected bindings: %#v", portBindings)
	}
}

func TestMountParsingHelpers(t *testing.T) {
	parsed, err := ParseMountSpec("type=bind,source=/tmp,target=/work,readonly,consistency=cached")
	if err != nil {
		t.Fatalf("ParseMountSpec: %v", err)
	}
	if parsed.Type != "bind" || parsed.Source != "/tmp" || parsed.Target != "/work" {
		t.Fatalf("unexpected parsed mount: %#v", parsed)
	}
	if !parsed.ReadOnly || parsed.Consistency != "cached" {
		t.Fatalf("unexpected parsed flags: %#v", parsed)
	}

	rawMount, err := mountFromSpec(MountSpec{Raw: "type=bind,source=/tmp,target=/work"})
	if err != nil {
		t.Fatalf("mountFromSpec raw: %v", err)
	}
	if rawMount.Type != mount.TypeBind || rawMount.Target != "/work" {
		t.Fatalf("unexpected raw mount: %#v", rawMount)
	}

	objectMount, err := mountFromSpec(MountSpec{Type: "volume", Source: "data", Target: "/data"})
	if err != nil {
		t.Fatalf("mountFromSpec object: %v", err)
	}
	if objectMount.Type != mount.TypeVolume || objectMount.Source != "data" {
		t.Fatalf("unexpected object mount: %#v", objectMount)
	}

	if _, err := mountFromSpec(MountSpec{Type: "volume"}); err == nil {
		t.Fatalf("expected error for missing target")
	}

	dockerMount, err := toDockerMount(Mount{Source: "data", Target: "/data"})
	if err != nil {
		t.Fatalf("toDockerMount: %v", err)
	}
	if dockerMount.Type != mount.TypeVolume || dockerMount.Target != "/data" {
		t.Fatalf("unexpected docker mount: %#v", dockerMount)
	}
	if _, err := toDockerMount(Mount{}); err == nil {
		t.Fatalf("expected error for empty target")
	}
}
