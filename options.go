package godev

import "time"

type StartOption func(*startOptions)

// startOptions holds StartDevcontainer configuration derived from StartOption values.
type startOptions struct {
	ConfigPath   string                // ConfigPath overrides the devcontainer.json path.
	Config       *DevcontainerConfig   // Config overrides devcontainer.json loading when set.
	MergeConfigs []*DevcontainerConfig // MergeConfigs are merged onto the base config in order.
	Env          map[string]string     // Env holds extra environment variables.
	ExtraPublish []string              // ExtraPublish adds port publish entries.
	ExtraMounts  []Mount               // ExtraMounts adds extra mount entries.
	RunArgs      []string              // RunArgs adds raw docker run arguments.
	RemoveOnStop bool                  // RemoveOnStop enables AutoRemove on the container.
	Detach       bool                  // Detach controls whether StartDevcontainer waits.
	TTY          bool                  // TTY controls pseudo-TTY allocation.
	Labels       map[string]string     // Labels adds Docker labels.
	Resources    ResourceLimits        // Resources configures CPU and memory limits.
	Network      string                // Network overrides the network mode.
	Timeout      time.Duration         // Timeout limits the overall start duration.
	Workdir      string                // Workdir overrides the container working directory.
}

// Mount describes an extra container mount to apply at start.
type Mount struct {
	Source      string // Source is the mount source path or volume name.
	Target      string // Target is the mount destination inside the container.
	Type        string // Type is the mount type, such as bind or volume.
	ReadOnly    bool   // ReadOnly marks the mount as read-only.
	Consistency string // Consistency sets the Docker mount consistency mode.
}

// ResourceLimits defines CPU and memory limits for the container.
type ResourceLimits struct {
	CPUQuota int64  // CPUQuota is the Docker CPU quota value.
	Memory   string // Memory is the memory limit string (e.g. "1g").
}

func defaultStartOptions() startOptions {
	return startOptions{
		Detach: true,
		TTY:    true,
	}
}

// WithConfigPath sets the devcontainer.json path used by StartDevcontainer.
// Impact: The provided path is used directly instead of searching for the config.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithConfigPath("./.devcontainer/devcontainer.json"))
//
// Similar: FindConfigPath only searches for the file path and does not configure StartDevcontainer.
func WithConfigPath(path string) StartOption {
	return func(o *startOptions) {
		o.ConfigPath = path
	}
}

// WithConfig sets the devcontainer config struct used by StartDevcontainer.
// Impact: The provided config is used instead of loading devcontainer.json.
// Example:
//
//	cfg := &devcontainer.DevcontainerConfig{Name: "example"}
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithConfig(cfg))
//
// Similar: WithConfigPath changes the file path, while WithConfig bypasses file loading.
func WithConfig(cfg *DevcontainerConfig) StartOption {
	return func(o *startOptions) {
		o.Config = cfg
	}
}

// WithMergeConfig adds a config overlay merged onto the base config.
// Impact: Later overlays override earlier values for scalar fields and append to slices.
// Example:
//
//	overlay := &devcontainer.DevcontainerConfig{RunArgs: []string{"--privileged"}}
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithMergeConfig(overlay))
//
// Similar: WithConfig sets the base config, while WithMergeConfig layers changes on top.
func WithMergeConfig(cfg *DevcontainerConfig) StartOption {
	return func(o *startOptions) {
		if cfg == nil {
			return
		}
		o.MergeConfigs = append(o.MergeConfigs, cfg)
	}
}

// WithEnv adds one container environment variable.
// Impact: Values are merged with containerEnv and override keys with the same name.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithEnv("FOO", "bar"))
//
// Similar: WithLabel adds Docker labels rather than environment variables.
func WithEnv(key, value string) StartOption {
	return func(o *startOptions) {
		if o.Env == nil {
			o.Env = make(map[string]string)
		}
		o.Env[key] = value
	}
}

// WithExtraPublish adds an extra port publish mapping.
// Impact: It is applied in addition to forwardPorts and appPort from devcontainer.json.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithExtraPublish("3000:3000"))
//
// Similar: forwardPorts/appPort come from the config file, while WithExtraPublish is a runtime override.
func WithExtraPublish(mapping string) StartOption {
	return func(o *startOptions) {
		o.ExtraPublish = append(o.ExtraPublish, mapping)
	}
}

// WithExtraMount adds an extra mount to the container configuration.
// Impact: It is appended to workspace and configured mounts and applied to HostConfig at create time.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithExtraMount(devcontainer.Mount{Source: "/tmp", Target: "/work", Type: "bind"}))
//
// Similar: ParseMountSpec only parses CLI mount strings and does not change options.
func WithExtraMount(m Mount) StartOption {
	return func(o *startOptions) {
		o.ExtraMounts = append(o.ExtraMounts, m)
	}
}

// WithRunArg appends one raw docker run argument.
// Impact: Selected flags (such as --cap-add) are parsed and affect privilege and network settings.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithRunArg("--cap-add=SYS_PTRACE"))
//
// Similar: WithResources/WithNetwork use structured input, while WithRunArg uses a raw string.
func WithRunArg(arg string) StartOption {
	return func(o *startOptions) {
		o.RunArgs = append(o.RunArgs, arg)
	}
}

// WithRemoveOnStop enables automatic container removal when it stops.
// Impact: Docker AutoRemove is set to true so the container is removed after stopping.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithRemoveOnStop())
//
// Similar: RemoveDevcontainer explicitly removes an existing container.
func WithRemoveOnStop() StartOption {
	return func(o *startOptions) {
		o.RemoveOnStop = true
	}
}

// WithDetach enables detached container start.
// Impact: StartDevcontainer returns after the container starts and does not wait for exit.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithDetach())
//
// Similar: WithDetachValue allows explicit true/false and waits on exit when false.
func WithDetach() StartOption {
	return func(o *startOptions) {
		o.Detach = true
	}
}

// WithDetachValue sets the detached execution flag explicitly.
// Impact: When false, StartDevcontainer waits for the container to exit.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithDetachValue(false))
//
// Similar: WithDetach is a convenience that always sets true.
func WithDetachValue(detach bool) StartOption {
	return func(o *startOptions) {
		o.Detach = detach
	}
}

// WithTTY enables TTY allocation.
// Impact: The container is created with Tty=true, which changes stdio handling.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithTTY())
//
// Similar: WithTTYValue allows explicit true/false and disables TTY when false.
func WithTTY() StartOption {
	return func(o *startOptions) {
		o.TTY = true
	}
}

// WithTTYValue sets the TTY allocation flag explicitly.
// Impact: When false, Tty is disabled on the container.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithTTYValue(false))
//
// Similar: WithTTY is a convenience that always sets true.
func WithTTYValue(tty bool) StartOption {
	return func(o *startOptions) {
		o.TTY = tty
	}
}

// WithLabel adds one Docker label.
// Impact: Labels are merged and keys with the same name are overwritten.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithLabel("team", "dev"))
//
// Similar: WithEnv adds environment variables, not labels.
func WithLabel(key, value string) StartOption {
	return func(o *startOptions) {
		if o.Labels == nil {
			o.Labels = make(map[string]string)
		}
		o.Labels[key] = value
	}
}

// WithTimeout sets the overall StartDevcontainer timeout.
// Impact: When the timeout is exceeded, the context is canceled and startup is aborted.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithTimeout(2*time.Minute))
//
// Similar: context.WithTimeout can be used by callers, but WithTimeout keeps timing in options.
func WithTimeout(timeout time.Duration) StartOption {
	return func(o *startOptions) {
		o.Timeout = timeout
	}
}

// WithResources sets CPU and memory limits.
// Impact: Docker HostConfig CPUQuota/Memory are set, enabling resource limits.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithResources(devcontainer.ResourceLimits{Memory: "1g"}))
//
// Similar: WithRunArg can pass --memory, but WithResources uses structured input.
func WithResources(resources ResourceLimits) StartOption {
	return func(o *startOptions) {
		o.Resources = resources
	}
}

// WithWorkdir overrides the container working directory.
// Impact: It takes precedence over workspaceFolder from devcontainer.json.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithWorkdir("/work"))
//
// Similar: workspaceFolder is static in the config file, while WithWorkdir is runtime.
func WithWorkdir(path string) StartOption {
	return func(o *startOptions) {
		o.Workdir = path
	}
}

// WithNetwork sets the Docker network mode to use.
// Impact: HostConfig.NetworkMode is set, overriding the default network resolution.
// Example:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithNetwork("host"))
//
// Similar: WithRunArg can pass --network, but WithNetwork is an explicit API.
func WithNetwork(network string) StartOption {
	return func(o *startOptions) {
		o.Network = network
	}
}
