package main

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	devcontainer "github.com/0x5341/godev"
)

func TestStartCommand_ParsesFlagsAndCallsStart(t *testing.T) {
	var got startConfig
	called := false
	startFn := func(ctx context.Context, cfg startConfig, _ []devcontainer.StartOption) (string, error) {
		called = true
		got = cfg
		return "container-123", nil
	}
	stopFn := func(ctx context.Context, _ stopConfig) error {
		return nil
	}
	downFn := func(ctx context.Context, _ downConfig) error {
		return nil
	}

	cmd := newRootCommand(startFn, stopFn, downFn)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"devcontainer",
		"start",
		"--config", "devcontainer.json",
		"--env", "FOO=bar",
		"--env", "BAZ=qux",
		"--publish", "3000:3000",
		"--mount", "type=bind,source=/tmp,target=/work",
		"--label", "team=dev",
		"--run-arg", "--cap-add=SYS_PTRACE",
		"--rm",
		"--detach=false",
		"--tty=false",
		"--timeout", "2s",
		"--workdir", "/work",
		"--network", "host",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Fatal("start function was not called")
	}
	if got.ConfigPath != "devcontainer.json" {
		t.Fatalf("expected config path, got %q", got.ConfigPath)
	}
	if got.Detach {
		t.Fatalf("expected detach false")
	}
	if got.TTY {
		t.Fatalf("expected tty false")
	}
	if !got.RemoveOnStop {
		t.Fatalf("expected remove-on-stop true")
	}
	if got.Timeout != 2*time.Second {
		t.Fatalf("expected timeout 2s, got %s", got.Timeout)
	}
	if got.Workdir != "/work" {
		t.Fatalf("expected workdir /work, got %q", got.Workdir)
	}
	if got.Network != "host" {
		t.Fatalf("expected network host, got %q", got.Network)
	}
	if !reflect.DeepEqual(got.Envs, []string{"FOO=bar", "BAZ=qux"}) {
		t.Fatalf("unexpected envs: %#v", got.Envs)
	}
	if !reflect.DeepEqual(got.Publishes, []string{"3000:3000"}) {
		t.Fatalf("unexpected publishes: %#v", got.Publishes)
	}
	if !reflect.DeepEqual(got.Mounts, []string{"type=bind,source=/tmp,target=/work"}) {
		t.Fatalf("unexpected mounts: %#v", got.Mounts)
	}
	if !reflect.DeepEqual(got.Labels, []string{"team=dev"}) {
		t.Fatalf("unexpected labels: %#v", got.Labels)
	}
	if !reflect.DeepEqual(got.RunArgs, []string{"--cap-add=SYS_PTRACE"}) {
		t.Fatalf("unexpected run args: %#v", got.RunArgs)
	}
	if stdout.String() != "container-123\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestStartCommand_InvalidEnv(t *testing.T) {
	called := false
	startFn := func(ctx context.Context, cfg startConfig, _ []devcontainer.StartOption) (string, error) {
		called = true
		return "", nil
	}
	stopFn := func(ctx context.Context, _ stopConfig) error {
		return nil
	}
	downFn := func(ctx context.Context, _ downConfig) error {
		return nil
	}

	cmd := newRootCommand(startFn, stopFn, downFn)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"devcontainer", "start", "--env", "INVALID"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid env")
	}
	if called {
		t.Fatal("start should not have been called")
	}
}

func TestStopCommand_ParsesFlagsAndCallsStop(t *testing.T) {
	var got stopConfig
	called := false
	startFn := func(ctx context.Context, cfg startConfig, _ []devcontainer.StartOption) (string, error) {
		return "", nil
	}
	stopFn := func(ctx context.Context, cfg stopConfig) error {
		called = true
		got = cfg
		return nil
	}
	downFn := func(ctx context.Context, _ downConfig) error {
		return nil
	}

	cmd := newRootCommand(startFn, stopFn, downFn)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"devcontainer", "stop", "--timeout", "3s", "container-123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Fatal("stop function was not called")
	}
	if got.ContainerID != "container-123" {
		t.Fatalf("expected container ID, got %q", got.ContainerID)
	}
	if got.Timeout != 3*time.Second {
		t.Fatalf("expected timeout 3s, got %s", got.Timeout)
	}
}

func TestDownCommand_CallsDown(t *testing.T) {
	var got downConfig
	called := false
	startFn := func(ctx context.Context, cfg startConfig, _ []devcontainer.StartOption) (string, error) {
		return "", nil
	}
	stopFn := func(ctx context.Context, _ stopConfig) error {
		return nil
	}
	downFn := func(ctx context.Context, cfg downConfig) error {
		called = true
		got = cfg
		return nil
	}

	cmd := newRootCommand(startFn, stopFn, downFn)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"devcontainer", "down", "container-123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Fatal("down function was not called")
	}
	if got.ContainerID != "container-123" {
		t.Fatalf("expected container ID, got %q", got.ContainerID)
	}
}
