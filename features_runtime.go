package godev

import (
	"context"
	"fmt"
)

var lifecycleOrder = []string{
	"onCreateCommand",
	"updateContentCommand",
	"postCreateCommand",
	"postStartCommand",
	"postAttachCommand",
}

func runFeatureEntrypoints(ctx context.Context, features []*ResolvedFeature, vars map[string]string, runner lifecycleRunner) error {
	if len(features) == 0 {
		return nil
	}
	for _, feature := range features {
		entrypoint, err := featureEntrypointPath(feature, vars)
		if err != nil {
			return err
		}
		if entrypoint == "" {
			continue
		}
		command := LifecycleCommand{Shell: entrypoint}
		name := fmt.Sprintf("featureEntrypoint:%s", feature.Metadata.ID)
		if err := runner(ctx, name, command); err != nil {
			return err
		}
	}
	return nil
}

func runLifecycleWithFeatures(ctx context.Context, features *ResolvedFeatures, userHooks map[string]*LifecycleCommands, runner lifecycleRunner) error {
	if len(userHooks) == 0 && (features == nil || len(features.Order) == 0) {
		return nil
	}
	for _, hook := range lifecycleOrder {
		if features != nil {
			for _, feature := range features.Order {
				if err := runFeatureLifecycleCommand(ctx, hook, feature, runner); err != nil {
					return err
				}
			}
		}
		if commands := userHooks[hook]; commands != nil {
			if err := runLifecycleCommands(ctx, hook, commands, runner); err != nil {
				return err
			}
		}
	}
	return nil
}

func runFeatureLifecycleCommand(ctx context.Context, hook string, feature *ResolvedFeature, runner lifecycleRunner) error {
	commands := featureLifecycleCommands(hook, feature)
	if commands == nil || commands.IsZero() {
		return nil
	}
	return runLifecycleCommands(ctx, hook, commands, runner)
}

func featureLifecycleCommands(hook string, feature *ResolvedFeature) *LifecycleCommands {
	switch hook {
	case "onCreateCommand":
		return feature.Metadata.OnCreateCommand
	case "updateContentCommand":
		return feature.Metadata.UpdateContentCommand
	case "postCreateCommand":
		return feature.Metadata.PostCreateCommand
	case "postStartCommand":
		return feature.Metadata.PostStartCommand
	case "postAttachCommand":
		return feature.Metadata.PostAttachCommand
	default:
		return nil
	}
}
