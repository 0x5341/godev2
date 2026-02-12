package godev

import (
	"testing"
	"time"
)

func TestStartOptionHelpers(t *testing.T) {
	options := defaultStartOptions()

	config := &DevcontainerConfig{Name: "base"}
	mergeConfig := &DevcontainerConfig{Name: "overlay"}

	WithConfigPath("devcontainer.json")(&options)
	WithConfig(config)(&options)
	WithMergeConfig(mergeConfig)(&options)
	WithEnv("FOO", "bar")(&options)
	WithExtraPublish("3000:3000")(&options)
	WithExtraMount(Mount{Source: "/tmp", Target: "/work", Type: "bind"})(&options)
	WithRunArg("--privileged")(&options)
	WithRemoveOnStop()(&options)
	WithDetach()(&options)
	WithDetachValue(false)(&options)
	WithTTY()(&options)
	WithTTYValue(false)(&options)
	WithLabel("team", "dev")(&options)
	WithTimeout(5 * time.Second)(&options)
	WithResources(ResourceLimits{CPUQuota: 100, Memory: "128m"})(&options)
	WithWorkdir("/work")(&options)
	WithNetwork("host")(&options)

	if options.ConfigPath != "devcontainer.json" {
		t.Fatalf("unexpected config path: %s", options.ConfigPath)
	}
	if options.Config != config {
		t.Fatalf("unexpected config: %#v", options.Config)
	}
	if len(options.MergeConfigs) != 1 || options.MergeConfigs[0] != mergeConfig {
		t.Fatalf("unexpected merge configs: %#v", options.MergeConfigs)
	}
	if options.Env["FOO"] != "bar" {
		t.Fatalf("unexpected env: %#v", options.Env)
	}
	if len(options.ExtraPublish) != 1 || options.ExtraPublish[0] != "3000:3000" {
		t.Fatalf("unexpected publish: %#v", options.ExtraPublish)
	}
	if len(options.ExtraMounts) != 1 || options.ExtraMounts[0].Target != "/work" {
		t.Fatalf("unexpected mount: %#v", options.ExtraMounts)
	}
	if len(options.RunArgs) != 1 || options.RunArgs[0] != "--privileged" {
		t.Fatalf("unexpected run args: %#v", options.RunArgs)
	}
	if !options.RemoveOnStop {
		t.Fatalf("expected remove-on-stop true")
	}
	if options.Detach {
		t.Fatalf("expected detach false")
	}
	if options.TTY {
		t.Fatalf("expected tty false")
	}
	if options.Labels["team"] != "dev" {
		t.Fatalf("unexpected labels: %#v", options.Labels)
	}
	if options.Timeout != 5*time.Second {
		t.Fatalf("unexpected timeout: %s", options.Timeout)
	}
	if options.Resources.CPUQuota != 100 || options.Resources.Memory != "128m" {
		t.Fatalf("unexpected resources: %#v", options.Resources)
	}
	if options.Workdir != "/work" {
		t.Fatalf("unexpected workdir: %s", options.Workdir)
	}
	if options.Network != "host" {
		t.Fatalf("unexpected network: %s", options.Network)
	}
}
