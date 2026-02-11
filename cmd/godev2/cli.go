package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0x5341/godev"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type StartFunc func(context.Context, startConfig, []devcontainer.StartOption) (string, error)
type StopFunc func(context.Context, stopConfig) error
type DownFunc func(context.Context, downConfig) error

// startConfig holds CLI flag values for devcontainer start.
type startConfig struct {
	ConfigPath   string        // ConfigPath is the devcontainer.json path override.
	Detach       bool          // Detach controls whether to run in the background.
	TTY          bool          // TTY controls whether to allocate a TTY.
	RemoveOnStop bool          // RemoveOnStop removes the container when it stops.
	Timeout      time.Duration // Timeout sets the start operation deadline.
	Workdir      string        // Workdir overrides the container working directory.
	Network      string        // Network overrides the container network mode.
	Envs         []string      // Envs holds extra KEY=VALUE environment variables.
	Publishes    []string      // Publishes holds extra port publish mappings.
	Mounts       []string      // Mounts holds extra Docker --mount specs.
	Labels       []string      // Labels holds extra Docker labels.
	RunArgs      []string      // RunArgs holds extra docker run arguments.
}

// stopConfig holds CLI flag values for devcontainer stop.
type stopConfig struct {
	ContainerID string        // ContainerID is the target container.
	Timeout     time.Duration // Timeout sets the stop grace period.
}

// downConfig holds CLI flag values for devcontainer down.
type downConfig struct {
	ContainerID string // ContainerID is the target container.
}

var errUsage = errors.New("usage error")

func run(args []string, start StartFunc, stop StopFunc, down DownFunc, stdout, stderr io.Writer) int {
	cmd := newRootCommand(start, stop, down)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return 0
		}
		if errors.Is(err, errUsage) || isUnknownCommandError(err) {
			_ = cmd.Usage()
			return 2
		}
		if isFlagParseError(err) {
			if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
				return 1
			}
			_ = cmd.Usage()
			return 2
		}
		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}

func newRootCommand(start StartFunc, stop StopFunc, down DownFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "godev",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errUsage
		},
	}
	cmd.AddCommand(newDevcontainerCommand(start, stop, down))
	return cmd
}

func newDevcontainerCommand(start StartFunc, stop StopFunc, down DownFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devcontainer",
		Short: "Devcontainer commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errUsage
		},
	}
	cmd.AddCommand(newStartCommand(start))
	cmd.AddCommand(newStopCommand(stop))
	cmd.AddCommand(newDownCommand(down))
	return cmd
}

func newStartCommand(start StartFunc) *cobra.Command {
	cfg := startConfig{
		Detach: true,
		TTY:    true,
	}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a devcontainer",
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := buildStartOptions(cfg)
			if err != nil {
				return err
			}
			containerID, err := start(cmd.Context(), cfg, options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), containerID)
			return err
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&cfg.ConfigPath, "config", "", "Path to devcontainer.json")
	flags.BoolVar(&cfg.Detach, "detach", true, "Run container in background")
	flags.BoolVar(&cfg.TTY, "tty", true, "Allocate a TTY")
	flags.BoolVar(&cfg.RemoveOnStop, "rm", false, "Remove container when it stops")
	flags.DurationVar(&cfg.Timeout, "timeout", 0, "Timeout for starting container")
	flags.StringVar(&cfg.Workdir, "workdir", "", "Override container working directory")
	flags.StringVar(&cfg.Network, "network", "", "Override container network")
	flags.StringArrayVar(&cfg.Envs, "env", nil, "Extra env var (KEY=VALUE)")
	flags.StringArrayVar(&cfg.Publishes, "publish", nil, "Extra port publish (e.g. 3000:3000)")
	flags.StringArrayVar(&cfg.Mounts, "mount", nil, "Extra mount (Docker --mount syntax)")
	flags.StringArrayVar(&cfg.Labels, "label", nil, "Extra label (KEY=VALUE)")
	flags.StringArrayVar(&cfg.RunArgs, "run-arg", nil, "Extra docker run argument")
	return cmd
}

func startWithConfig(ctx context.Context, cfg startConfig, options []devcontainer.StartOption) (string, error) {
	return devcontainer.StartDevcontainer(ctx, options...)
}

func stopWithConfig(ctx context.Context, cfg stopConfig) error {
	return devcontainer.StopDevcontainer(ctx, cfg.ContainerID, cfg.Timeout)
}

func downWithConfig(ctx context.Context, cfg downConfig) error {
	return devcontainer.RemoveDevcontainer(ctx, cfg.ContainerID)
}

func buildStartOptions(cfg startConfig) ([]devcontainer.StartOption, error) {
	options := make([]devcontainer.StartOption, 0, 8)
	if cfg.ConfigPath != "" {
		options = append(options, devcontainer.WithConfigPath(cfg.ConfigPath))
	}
	for _, env := range cfg.Envs {
		key, value, err := splitKeyValue(env)
		if err != nil {
			return nil, err
		}
		options = append(options, devcontainer.WithEnv(key, value))
	}
	for _, publish := range cfg.Publishes {
		options = append(options, devcontainer.WithExtraPublish(publish))
	}
	for _, mountSpec := range cfg.Mounts {
		parsed, err := devcontainer.ParseMountSpec(mountSpec)
		if err != nil {
			return nil, err
		}
		options = append(options, devcontainer.WithExtraMount(parsed))
	}
	for _, label := range cfg.Labels {
		key, value, err := splitKeyValue(label)
		if err != nil {
			return nil, err
		}
		options = append(options, devcontainer.WithLabel(key, value))
	}
	for _, arg := range cfg.RunArgs {
		options = append(options, devcontainer.WithRunArg(arg))
	}
	if cfg.RemoveOnStop {
		options = append(options, devcontainer.WithRemoveOnStop())
	}
	if !cfg.Detach {
		options = append(options, devcontainer.WithDetachValue(false))
	}
	if !cfg.TTY {
		options = append(options, devcontainer.WithTTYValue(false))
	}
	if cfg.Timeout > 0 {
		options = append(options, devcontainer.WithTimeout(cfg.Timeout))
	}
	if cfg.Workdir != "" {
		options = append(options, devcontainer.WithWorkdir(cfg.Workdir))
	}
	if cfg.Network != "" {
		options = append(options, devcontainer.WithNetwork(cfg.Network))
	}
	return options, nil
}

func newStopCommand(stop StopFunc) *cobra.Command {
	cfg := stopConfig{}
	cmd := &cobra.Command{
		Use:   "stop <container-id>",
		Short: "Stop a devcontainer",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errUsage
			}
			cfg.ContainerID = args[0]
			return stop(cmd.Context(), cfg)
		},
	}
	flags := cmd.Flags()
	flags.DurationVar(&cfg.Timeout, "timeout", 0, "Timeout for stopping container")
	return cmd
}

func newDownCommand(down DownFunc) *cobra.Command {
	cfg := downConfig{}
	cmd := &cobra.Command{
		Use:   "down <container-id>",
		Short: "Remove a devcontainer",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errUsage
			}
			cfg.ContainerID = args[0]
			return down(cmd.Context(), cfg)
		},
	}
	return cmd
}

func splitKeyValue(input string) (string, string, error) {
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", fmt.Errorf("invalid key=value: %s", input)
	}
	return parts[0], parts[1], nil
}

func isUnknownCommandError(err error) bool {
	return strings.HasPrefix(err.Error(), "unknown command")
}

func isFlagParseError(err error) bool {
	message := err.Error()
	return strings.HasPrefix(message, "unknown flag") ||
		strings.HasPrefix(message, "unknown shorthand flag") ||
		strings.HasPrefix(message, "flag needs an argument") ||
		strings.HasPrefix(message, "invalid argument")
}
