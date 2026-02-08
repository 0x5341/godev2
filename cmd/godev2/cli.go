package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0x5341/godev2/internal/devcontainer"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type StartFunc func(context.Context, startConfig, []devcontainer.StartOption) (string, error)

type startConfig struct {
	ConfigPath   string
	Detach       bool
	TTY          bool
	RemoveOnStop bool
	Timeout      time.Duration
	Workdir      string
	Network      string
	Envs         []string
	Publishes    []string
	Mounts       []string
	Labels       []string
	RunArgs      []string
}

var errUsage = errors.New("usage error")

func run(args []string, start StartFunc, stdout, stderr io.Writer) int {
	cmd := newRootCommand(start)
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

func newRootCommand(start StartFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "godev2",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errUsage
		},
	}
	cmd.AddCommand(newDevcontainerCommand(start))
	return cmd
}

func newDevcontainerCommand(start StartFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devcontainer",
		Short: "Devcontainer commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errUsage
		},
	}
	cmd.AddCommand(newStartCommand(start))
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
