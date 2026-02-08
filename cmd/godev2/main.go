package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/0x5341/godev2/internal/devcontainer"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "devcontainer":
		devcontainerMain(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func devcontainerMain(args []string) {
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "start":
		devcontainerStart(args[1:])
	default:
		usage()
		os.Exit(2)
	}
}

func devcontainerStart(args []string) {
	fs := flag.NewFlagSet("devcontainer start", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to devcontainer.json")
	detach := fs.Bool("detach", true, "Run container in background")
	tty := fs.Bool("tty", true, "Allocate a TTY")
	removeOnStop := fs.Bool("rm", false, "Remove container when it stops")
	timeout := fs.Duration("timeout", 0, "Timeout for starting container")
	workdir := fs.String("workdir", "", "Override container working directory")
	network := fs.String("network", "", "Override container network")

	var envs multiFlag
	var publishes multiFlag
	var mounts multiFlag
	var labels multiFlag
	var runArgs multiFlag

	fs.Var(&envs, "env", "Extra env var (KEY=VALUE)")
	fs.Var(&publishes, "publish", "Extra port publish (e.g. 3000:3000)")
	fs.Var(&mounts, "mount", "Extra mount (Docker --mount syntax)")
	fs.Var(&labels, "label", "Extra label (KEY=VALUE)")
	fs.Var(&runArgs, "run-arg", "Extra docker run argument")

	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	var options []devcontainer.StartOption
	if *configPath != "" {
		options = append(options, devcontainer.WithConfigPath(*configPath))
	}
	for _, env := range envs {
		key, value, err := splitKeyValue(env)
		if err != nil {
			exitWithError(err)
		}
		options = append(options, devcontainer.WithEnv(key, value))
	}
	for _, publish := range publishes {
		options = append(options, devcontainer.WithExtraPublish(publish))
	}
	for _, mountSpec := range mounts {
		parsed, err := devcontainer.ParseMountSpec(mountSpec)
		if err != nil {
			exitWithError(err)
		}
		options = append(options, devcontainer.WithExtraMount(parsed))
	}
	for _, label := range labels {
		key, value, err := splitKeyValue(label)
		if err != nil {
			exitWithError(err)
		}
		options = append(options, devcontainer.WithLabel(key, value))
	}
	for _, arg := range runArgs {
		options = append(options, devcontainer.WithRunArg(arg))
	}

	if *removeOnStop {
		options = append(options, devcontainer.WithRemoveOnStop())
	}
	if *detach {
		options = append(options, devcontainer.WithDetach())
	}
	if *tty {
		options = append(options, devcontainer.WithTTY())
	}
	if *timeout > 0 {
		options = append(options, devcontainer.WithTimeout(*timeout))
	}
	if *workdir != "" {
		options = append(options, devcontainer.WithWorkdir(*workdir))
	}
	if *network != "" {
		options = append(options, devcontainer.WithNetwork(*network))
	}

	containerID, err := devcontainer.StartDevcontainer(context.Background(), options...)
	if err != nil {
		exitWithError(err)
	}
	if _, err := fmt.Fprintln(os.Stdout, containerID); err != nil {
		exitWithError(err)
	}
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func splitKeyValue(input string) (string, string, error) {
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", fmt.Errorf("invalid key=value: %s", input)
	}
	return parts[0], parts[1], nil
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: godev2 devcontainer start [options]")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --config PATH          devcontainer.json path (default: auto-detect)")
	fmt.Fprintln(os.Stderr, "  --env KEY=VALUE        extra containerEnv override")
	fmt.Fprintln(os.Stderr, "  --publish PORTSPEC     extra port mapping")
	fmt.Fprintln(os.Stderr, "  --mount SPEC           extra mount (--mount syntax)")
	fmt.Fprintln(os.Stderr, "  --label KEY=VALUE      extra label")
	fmt.Fprintln(os.Stderr, "  --run-arg ARG          extra docker run arg")
	fmt.Fprintln(os.Stderr, "  --detach               detach (default true)")
	fmt.Fprintln(os.Stderr, "  --tty                  allocate TTY (default true)")
	fmt.Fprintln(os.Stderr, "  --rm                   remove container on stop")
	fmt.Fprintln(os.Stderr, "  --timeout DURATION     timeout for start")
	fmt.Fprintln(os.Stderr, "  --workdir PATH         override container working directory")
	fmt.Fprintln(os.Stderr, "  --network NAME         override container network")
}
