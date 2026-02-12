package devcontainer

import "testing"

func TestMergeConfig_MergesFields(t *testing.T) {
	baseInit := false
	overlayInit := true
	baseOverride := true
	overlayOverride := false

	base := &DevcontainerConfig{
		Name:            "base",
		Image:           "base-image",
		RunArgs:         []string{"--cap-add=SYS_PTRACE"},
		CapAdd:          []string{"SYS_PTRACE"},
		Privileged:      true,
		ForwardPorts:    PortList{"3000"},
		ContainerEnv:    map[string]string{"A": "1"},
		RemoteEnv:       map[string]string{"R": "1"},
		Init:            &baseInit,
		OverrideCommand: &baseOverride,
		SecurityOpt:     []string{"seccomp=unconfined"},
		Build: &DevcontainerBuild{
			Dockerfile: "Dockerfile",
			Args:       map[string]string{"A": "1"},
			CacheFrom:  StringSlice{"base"},
			Options:    []string{"--opt1"},
		},
	}
	overlay := &DevcontainerConfig{
		Name:            "overlay",
		Image:           "overlay-image",
		RunArgs:         []string{"--security-opt=apparmor=unconfined"},
		CapAdd:          []string{"NET_ADMIN"},
		ForwardPorts:    PortList{"4000"},
		ContainerEnv:    map[string]string{"A": "2", "B": "3"},
		Init:            &overlayInit,
		OverrideCommand: &overlayOverride,
		SecurityOpt:     []string{"apparmor=unconfined"},
		Build: &DevcontainerBuild{
			Context:   "./context",
			Args:      map[string]string{"A": "2", "B": "3"},
			CacheFrom: StringSlice{"overlay"},
		},
	}

	merged := MergeConfig(base, overlay)

	if merged.Name != "overlay" {
		t.Fatalf("expected name overlay, got %q", merged.Name)
	}
	if merged.Image != "overlay-image" {
		t.Fatalf("expected image overlay-image, got %q", merged.Image)
	}
	if merged.Build == nil || merged.Build.Dockerfile != "Dockerfile" {
		t.Fatalf("expected Dockerfile from base, got %#v", merged.Build)
	}
	if merged.Build.Context != "./context" {
		t.Fatalf("expected context ./context, got %q", merged.Build.Context)
	}
	if merged.Build.Args["A"] != "2" || merged.Build.Args["B"] != "3" {
		t.Fatalf("unexpected build args: %#v", merged.Build.Args)
	}
	if len(merged.Build.CacheFrom) != 2 || merged.Build.CacheFrom[0] != "base" || merged.Build.CacheFrom[1] != "overlay" {
		t.Fatalf("unexpected cacheFrom: %#v", merged.Build.CacheFrom)
	}
	if len(merged.RunArgs) != 2 || merged.RunArgs[0] != "--cap-add=SYS_PTRACE" || merged.RunArgs[1] != "--security-opt=apparmor=unconfined" {
		t.Fatalf("unexpected runArgs: %#v", merged.RunArgs)
	}
	if merged.ContainerEnv["A"] != "2" || merged.ContainerEnv["B"] != "3" {
		t.Fatalf("unexpected containerEnv: %#v", merged.ContainerEnv)
	}
	if merged.RemoteEnv["R"] != "1" {
		t.Fatalf("unexpected remoteEnv: %#v", merged.RemoteEnv)
	}
	if len(merged.ForwardPorts) != 2 || merged.ForwardPorts[0] != "3000" || merged.ForwardPorts[1] != "4000" {
		t.Fatalf("unexpected forwardPorts: %#v", merged.ForwardPorts)
	}
	if !merged.Privileged {
		t.Fatalf("expected privileged true")
	}
	if merged.Init == nil || !*merged.Init {
		t.Fatalf("expected init true")
	}
	if merged.OverrideCommand == nil || *merged.OverrideCommand {
		t.Fatalf("expected overrideCommand false")
	}
	if len(merged.CapAdd) != 2 || merged.CapAdd[0] != "SYS_PTRACE" || merged.CapAdd[1] != "NET_ADMIN" {
		t.Fatalf("unexpected capAdd: %#v", merged.CapAdd)
	}
	if len(merged.SecurityOpt) != 2 || merged.SecurityOpt[0] != "seccomp=unconfined" || merged.SecurityOpt[1] != "apparmor=unconfined" {
		t.Fatalf("unexpected securityOpt: %#v", merged.SecurityOpt)
	}
	if base.ContainerEnv["A"] != "1" {
		t.Fatalf("expected base config not to be mutated")
	}
}

func TestMergeConfig_MergesFeatures(t *testing.T) {
	base := &DevcontainerConfig{
		Features: FeatureSet{
			"ghcr.io/devcontainers/features/go": {
				"version": stringOption("1.20"),
				"global":  boolOption(true),
			},
		},
	}
	overlay := &DevcontainerConfig{
		Features: FeatureSet{
			"ghcr.io/devcontainers/features/go": {
				"version": stringOption("1.21"),
				"newOpt":  stringOption("value"),
			},
			"ghcr.io/devcontainers/features/git": {
				"version": stringOption("1"),
			},
		},
	}

	merged := MergeConfig(base, overlay)
	opts := merged.Features["ghcr.io/devcontainers/features/go"]
	if opts["version"].String == nil || *opts["version"].String != "1.21" {
		t.Fatalf("unexpected feature version: %#v", opts["version"])
	}
	if opts["global"].Bool == nil || !*opts["global"].Bool {
		t.Fatalf("expected feature option from base")
	}
	if opts["newOpt"].String == nil || *opts["newOpt"].String != "value" {
		t.Fatalf("unexpected merged option: %#v", opts["newOpt"])
	}
	if _, ok := merged.Features["ghcr.io/devcontainers/features/git"]; !ok {
		t.Fatalf("expected new feature to be added")
	}
}

func stringOption(value string) FeatureOptionValue {
	return FeatureOptionValue{String: &value}
}

func boolOption(value bool) FeatureOptionValue {
	return FeatureOptionValue{Bool: &value}
}
