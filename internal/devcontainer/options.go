package devcontainer

import "time"

type StartOption func(*startOptions)

type startOptions struct {
	ConfigPath   string
	Env          map[string]string
	ExtraPublish []string
	ExtraMounts  []Mount
	RunArgs      []string
	RemoveOnStop bool
	Detach       bool
	TTY          bool
	Labels       map[string]string
	Resources    ResourceLimits
	Network      string
	Timeout      time.Duration
	Workdir      string
}

type Mount struct {
	Source      string
	Target      string
	Type        string
	ReadOnly    bool
	Consistency string
}

type ResourceLimits struct {
	CPUQuota int64
	Memory   string
}

func defaultStartOptions() startOptions {
	return startOptions{
		Detach: true,
		TTY:    true,
	}
}

func WithConfigPath(path string) StartOption {
	return func(o *startOptions) {
		o.ConfigPath = path
	}
}

func WithEnv(key, value string) StartOption {
	return func(o *startOptions) {
		if o.Env == nil {
			o.Env = make(map[string]string)
		}
		o.Env[key] = value
	}
}

func WithExtraPublish(mapping string) StartOption {
	return func(o *startOptions) {
		o.ExtraPublish = append(o.ExtraPublish, mapping)
	}
}

func WithExtraMount(m Mount) StartOption {
	return func(o *startOptions) {
		o.ExtraMounts = append(o.ExtraMounts, m)
	}
}

func WithRunArg(arg string) StartOption {
	return func(o *startOptions) {
		o.RunArgs = append(o.RunArgs, arg)
	}
}

func WithRemoveOnStop() StartOption {
	return func(o *startOptions) {
		o.RemoveOnStop = true
	}
}

func WithDetach() StartOption {
	return func(o *startOptions) {
		o.Detach = true
	}
}

func WithDetachValue(detach bool) StartOption {
	return func(o *startOptions) {
		o.Detach = detach
	}
}

func WithTTY() StartOption {
	return func(o *startOptions) {
		o.TTY = true
	}
}

func WithTTYValue(tty bool) StartOption {
	return func(o *startOptions) {
		o.TTY = tty
	}
}

func WithLabel(key, value string) StartOption {
	return func(o *startOptions) {
		if o.Labels == nil {
			o.Labels = make(map[string]string)
		}
		o.Labels[key] = value
	}
}

func WithTimeout(timeout time.Duration) StartOption {
	return func(o *startOptions) {
		o.Timeout = timeout
	}
}

func WithResources(resources ResourceLimits) StartOption {
	return func(o *startOptions) {
		o.Resources = resources
	}
}

func WithWorkdir(path string) StartOption {
	return func(o *startOptions) {
		o.Workdir = path
	}
}

func WithNetwork(network string) StartOption {
	return func(o *startOptions) {
		o.Network = network
	}
}
