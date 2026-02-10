package devcontainer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type LifecycleCommand struct {
	Shell string
	Exec  []string
}

type NamedLifecycleCommand struct {
	Name    string
	Command LifecycleCommand
}

type LifecycleCommands struct {
	Single   *LifecycleCommand
	Parallel []NamedLifecycleCommand
}

// IsZero は LifecycleCommands が未設定かどうかを判定する。
// 影響: 状態確認のみで副作用はなく、単体/並列のどちらも空なら true になる。
// 例:
//
//	if cmds.IsZero() { /* no-op */ }
//
// 類似: nil チェックはポインタの有無だけを見るが、IsZero は空スライスも未設定扱いにする。
func (c *LifecycleCommands) IsZero() bool {
	return c == nil || (c.Single == nil && len(c.Parallel) == 0)
}

// UnmarshalJSON は LifecycleCommands に JSON の文字列/配列/オブジェクト形式のコマンドを取り込む。
// 影響: 空値を拒否し、オブジェクト形式ではキーをソートして並列実行順を安定させる。
// 例:
//
//	var c devcontainer.LifecycleCommands
//	_ = json.Unmarshal([]byte(`{"postCreateCommand":"echo hi"}`), &c)
//
// 類似: FeatureSet の UnmarshalJSON は feature マップを解析するが、LifecycleCommands はコマンド構造に特化する。
func (c *LifecycleCommands) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	switch data[0] {
	case '"', '[':
		command, err := parseLifecycleCommand(data)
		if err != nil {
			return err
		}
		if command.isEmpty() {
			return errors.New("lifecycle command cannot be empty")
		}
		c.Single = &command
		c.Parallel = nil
		return nil
	case '{':
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		if len(raw) == 0 {
			return errors.New("lifecycle command object cannot be empty")
		}
		names := make([]string, 0, len(raw))
		for name := range raw {
			names = append(names, name)
		}
		sort.Strings(names)
		commands := make([]NamedLifecycleCommand, 0, len(names))
		for _, name := range names {
			if strings.TrimSpace(name) == "" {
				return errors.New("lifecycle command name cannot be empty")
			}
			command, err := parseLifecycleCommand(raw[name])
			if err != nil {
				return fmt.Errorf("lifecycle command %s: %w", name, err)
			}
			if command.isEmpty() {
				return fmt.Errorf("lifecycle command %s is empty", name)
			}
			commands = append(commands, NamedLifecycleCommand{Name: name, Command: command})
		}
		c.Single = nil
		c.Parallel = commands
		return nil
	default:
		return fmt.Errorf("invalid lifecycle command: %s", string(data))
	}
}

func parseLifecycleCommand(data []byte) (LifecycleCommand, error) {
	if len(data) == 0 || string(data) == "null" {
		return LifecycleCommand{}, nil
	}
	switch data[0] {
	case '"':
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return LifecycleCommand{}, err
		}
		if strings.TrimSpace(value) == "" {
			return LifecycleCommand{}, errors.New("lifecycle command cannot be empty")
		}
		return LifecycleCommand{Shell: value}, nil
	case '[':
		var values []string
		if err := json.Unmarshal(data, &values); err != nil {
			return LifecycleCommand{}, err
		}
		if len(values) == 0 {
			return LifecycleCommand{}, errors.New("lifecycle command array cannot be empty")
		}
		return LifecycleCommand{Exec: values}, nil
	default:
		return LifecycleCommand{}, fmt.Errorf("invalid lifecycle command: %s", string(data))
	}
}

func (c LifecycleCommand) isEmpty() bool {
	return c.Shell == "" && len(c.Exec) == 0
}

type lifecycleHook struct {
	Name     string
	Commands *LifecycleCommands
}

type lifecycleRunner func(ctx context.Context, name string, command LifecycleCommand) error

func runLifecycleSequence(ctx context.Context, hooks []lifecycleHook, runner lifecycleRunner) error {
	for _, hook := range hooks {
		if hook.Commands == nil || hook.Commands.IsZero() {
			continue
		}
		if err := runLifecycleCommands(ctx, hook.Name, hook.Commands, runner); err != nil {
			return err
		}
	}
	return nil
}

func runLifecycleCommands(ctx context.Context, hookName string, commands *LifecycleCommands, runner lifecycleRunner) error {
	if commands == nil || commands.IsZero() {
		return nil
	}
	if commands.Single != nil {
		return runner(ctx, hookName, *commands.Single)
	}
	return runParallelLifecycleCommands(ctx, hookName, commands.Parallel, runner)
}

func runParallelLifecycleCommands(ctx context.Context, hookName string, commands []NamedLifecycleCommand, runner lifecycleRunner) error {
	if len(commands) == 0 {
		return nil
	}
	errs := make(chan error, len(commands))
	var wg sync.WaitGroup
	for _, command := range commands {
		command := command
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("%s:%s", hookName, command.Name)
			errs <- runner(ctx, name, command.Command)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func hostLifecycleRunner(workdir string, vars, containerEnv map[string]string) lifecycleRunner {
	return func(ctx context.Context, name string, command LifecycleCommand) error {
		expanded, err := expandLifecycleCommand(command, vars, containerEnv)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		args, err := lifecycleCommandArgs(expanded)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = workdir
		cmd.Env = os.Environ()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return formatLifecycleError(name, args, stdout.String(), stderr.String(), err)
		}
		return nil
	}
}

func containerLifecycleRunner(cli *client.Client, containerID, workdir, user string, vars, containerEnv map[string]string, env []string) lifecycleRunner {
	return func(ctx context.Context, name string, command LifecycleCommand) error {
		expanded, err := expandLifecycleCommand(command, vars, containerEnv)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		args, err := lifecycleCommandArgs(expanded)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		execConfig := container.ExecOptions{
			Cmd:          args,
			Env:          env,
			WorkingDir:   workdir,
			User:         user,
			AttachStdout: true,
			AttachStderr: true,
		}
		execResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		resp, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{Tty: false})
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		defer func() {
			resp.Close()
		}()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if inspect.ExitCode != 0 {
			return formatLifecycleExitError(name, args, stdout.String(), stderr.String(), inspect.ExitCode)
		}
		return nil
	}
}

func expandLifecycleCommand(command LifecycleCommand, vars, containerEnv map[string]string) (LifecycleCommand, error) {
	if command.Shell != "" {
		expanded, err := expandVariables(command.Shell, vars, containerEnv)
		if err != nil {
			return LifecycleCommand{}, err
		}
		return LifecycleCommand{Shell: expanded}, nil
	}
	if len(command.Exec) == 0 {
		return LifecycleCommand{}, errors.New("lifecycle command is empty")
	}
	expanded := make([]string, len(command.Exec))
	for i, value := range command.Exec {
		item, err := expandVariables(value, vars, containerEnv)
		if err != nil {
			return LifecycleCommand{}, err
		}
		expanded[i] = item
	}
	return LifecycleCommand{Exec: expanded}, nil
}

func lifecycleCommandArgs(command LifecycleCommand) ([]string, error) {
	if command.Shell != "" {
		return []string{"/bin/sh", "-c", command.Shell}, nil
	}
	if len(command.Exec) == 0 {
		return nil, errors.New("lifecycle command is empty")
	}
	args := make([]string, len(command.Exec))
	copy(args, command.Exec)
	return args, nil
}

func formatLifecycleError(name string, args []string, stdout, stderr string, err error) error {
	output := strings.TrimSpace(strings.Join([]string{stdout, stderr}, "\n"))
	if output != "" {
		return fmt.Errorf("%s failed (%s): %s", name, strings.Join(args, " "), output)
	}
	return fmt.Errorf("%s failed (%s): %w", name, strings.Join(args, " "), err)
}

func formatLifecycleExitError(name string, args []string, stdout, stderr string, exitCode int) error {
	output := strings.TrimSpace(strings.Join([]string{stdout, stderr}, "\n"))
	if output != "" {
		return fmt.Errorf("%s failed (%s): exit code %d: %s", name, strings.Join(args, " "), exitCode, output)
	}
	return fmt.Errorf("%s failed (%s): exit code %d", name, strings.Join(args, " "), exitCode)
}

func buildLifecycleEnv(containerEnv, remoteEnv, vars map[string]string) (map[string]string, error) {
	merged := make(map[string]string, len(containerEnv)+len(remoteEnv))
	for key, value := range containerEnv {
		merged[key] = value
	}
	for key, value := range remoteEnv {
		expanded, err := expandVariables(value, vars, merged)
		if err != nil {
			return nil, err
		}
		merged[key] = expanded
	}
	return merged, nil
}
