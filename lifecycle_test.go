package devcontainer

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestLifecycleCommands_UnmarshalString(t *testing.T) {
	var got LifecycleCommands
	if err := json.Unmarshal([]byte(`"echo hello"`), &got); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if got.Single == nil || got.Single.Shell != "echo hello" {
		t.Fatalf("unexpected single command: %#v", got.Single)
	}
	if len(got.Parallel) != 0 {
		t.Fatalf("expected no parallel commands: %#v", got.Parallel)
	}
}

func TestLifecycleCommands_UnmarshalArray(t *testing.T) {
	var got LifecycleCommands
	if err := json.Unmarshal([]byte(`["echo", "hello"]`), &got); err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	if got.Single == nil || !reflect.DeepEqual(got.Single.Exec, []string{"echo", "hello"}) {
		t.Fatalf("unexpected exec command: %#v", got.Single)
	}
	if len(got.Parallel) != 0 {
		t.Fatalf("expected no parallel commands: %#v", got.Parallel)
	}
}

func TestLifecycleCommands_UnmarshalObject(t *testing.T) {
	var got LifecycleCommands
	if err := json.Unmarshal([]byte(`{"alpha":"echo a","beta":["echo","b"]}`), &got); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if got.Single != nil {
		t.Fatalf("expected no single command: %#v", got.Single)
	}
	if len(got.Parallel) != 2 {
		t.Fatalf("unexpected parallel commands: %#v", got.Parallel)
	}
	if got.Parallel[0].Name != "alpha" || got.Parallel[1].Name != "beta" {
		t.Fatalf("unexpected command order: %#v", got.Parallel)
	}
	if got.Parallel[0].Command.Shell != "echo a" {
		t.Fatalf("unexpected command alpha: %#v", got.Parallel[0].Command)
	}
	if !reflect.DeepEqual(got.Parallel[1].Command.Exec, []string{"echo", "b"}) {
		t.Fatalf("unexpected command beta: %#v", got.Parallel[1].Command)
	}
}

func TestLifecycleCommands_UnmarshalInvalid(t *testing.T) {
	var got LifecycleCommands
	if err := json.Unmarshal([]byte(`123`), &got); err == nil {
		t.Fatal("expected error for invalid lifecycle command")
	}
}

func TestRunLifecycleCommands_Parallel(t *testing.T) {
	commands := &LifecycleCommands{
		Parallel: []NamedLifecycleCommand{
			{Name: "alpha", Command: LifecycleCommand{Shell: "echo alpha"}},
			{Name: "beta", Command: LifecycleCommand{Shell: "echo beta"}},
		},
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	runner := func(ctx context.Context, name string, command LifecycleCommand) error {
		started <- name
		<-release
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- runLifecycleCommands(context.Background(), "postCreateCommand", commands, runner)
	}()
	names := []string{<-started, <-started}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("runLifecycleCommands: %v", err)
	}
	sort.Strings(names)
	expected := []string{"postCreateCommand:alpha", "postCreateCommand:beta"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("unexpected command names: %#v", names)
	}
}

func TestRunLifecycleSequence_StopsOnError(t *testing.T) {
	hooks := []lifecycleHook{
		{Name: "onCreateCommand", Commands: &LifecycleCommands{Single: &LifecycleCommand{Shell: "echo a"}}},
		{Name: "postStartCommand", Commands: &LifecycleCommands{Single: &LifecycleCommand{Shell: "echo b"}}},
	}
	var called []string
	runner := func(ctx context.Context, name string, command LifecycleCommand) error {
		called = append(called, name)
		if name == "onCreateCommand" {
			return errors.New("boom")
		}
		return nil
	}
	if err := runLifecycleSequence(context.Background(), hooks, runner); err == nil {
		t.Fatal("expected error from lifecycle sequence")
	}
	if !reflect.DeepEqual(called, []string{"onCreateCommand"}) {
		t.Fatalf("unexpected call order: %#v", called)
	}
}

func TestRunLifecycleSequence_Order(t *testing.T) {
	hooks := []lifecycleHook{
		{Name: "onCreateCommand", Commands: &LifecycleCommands{Single: &LifecycleCommand{Shell: "echo a"}}},
		{Name: "postStartCommand", Commands: &LifecycleCommands{Single: &LifecycleCommand{Shell: "echo b"}}},
	}
	var called []string
	runner := func(ctx context.Context, name string, command LifecycleCommand) error {
		called = append(called, name)
		return nil
	}
	if err := runLifecycleSequence(context.Background(), hooks, runner); err != nil {
		t.Fatalf("runLifecycleSequence: %v", err)
	}
	if !reflect.DeepEqual(called, []string{"onCreateCommand", "postStartCommand"}) {
		t.Fatalf("unexpected call order: %#v", called)
	}
}
