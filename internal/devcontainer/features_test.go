package devcontainer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFeatureOptions(t *testing.T) {
	defs := map[string]FeatureOptionDefinition{
		"version": {
			Type:    "string",
			Default: FeatureOptionValue{String: stringPtr("latest")},
		},
		"flag": {
			Type:    "boolean",
			Default: FeatureOptionValue{Bool: boolPtr(true)},
		},
	}
	user := FeatureOptions{
		"version": {String: stringPtr("1.2.3")},
		"flag":    {Bool: boolPtr(false)},
	}
	resolved, err := resolveFeatureOptions(defs, user)
	if err != nil {
		t.Fatalf("resolveFeatureOptions: %v", err)
	}
	if resolved.Values["version"] != "1.2.3" || resolved.Values["flag"] != "false" {
		t.Fatalf("unexpected resolved values: %#v", resolved.Values)
	}
	if resolved.UserValues["version"] != "1.2.3" || resolved.UserValues["flag"] != "false" {
		t.Fatalf("unexpected user values: %#v", resolved.UserValues)
	}
}

func TestOrderFeatures_DependsOnInstallsAfter(t *testing.T) {
	foo := &ResolvedFeature{
		DependencyKey: "foo-key",
		BaseName:      "foo",
		Tag:           "1",
		Options:       ResolvedFeatureOptions{UserValues: map[string]string{}},
		CanonicalName: "foo@sha",
	}
	bar := &ResolvedFeature{
		DependencyKey: "bar-key",
		BaseName:      "bar",
		Tag:           "1",
		DependsOnKeys: []string{"foo-key"},
		Options:       ResolvedFeatureOptions{UserValues: map[string]string{}},
		CanonicalName: "bar@sha",
	}
	baz := &ResolvedFeature{
		DependencyKey:    "baz-key",
		BaseName:         "baz",
		Tag:              "1",
		InstallsAfterIDs: []string{"foo"},
		Options:          ResolvedFeatureOptions{UserValues: map[string]string{}},
		CanonicalName:    "baz@sha",
	}
	order, err := orderFeatures([]*ResolvedFeature{bar, baz, foo}, nil)
	if err != nil {
		t.Fatalf("orderFeatures: %v", err)
	}
	got := []string{order[0].DependencyKey, order[1].DependencyKey, order[2].DependencyKey}
	expected := []string{"foo-key", "bar-key", "baz-key"}
	for i, value := range expected {
		if got[i] != value {
			t.Fatalf("unexpected order: %#v", got)
		}
	}
	override, err := orderFeatures([]*ResolvedFeature{bar, baz, foo}, []string{"baz"})
	if err != nil {
		t.Fatalf("orderFeatures override: %v", err)
	}
	if override[1].DependencyKey != "baz-key" {
		t.Fatalf("expected baz priority, got %#v", override)
	}
}

func TestResolveFeatures_Local(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".devcontainer")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(configDir, "devcontainer.json")
	config := `{
		"image": "alpine:3.19",
		"features": {
			"./featureA": {}
		}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	featureB := filepath.Join(configDir, "featureB")
	if err := os.MkdirAll(featureB, 0o755); err != nil {
		t.Fatalf("mkdir featureB: %v", err)
	}
	featureBMeta := `{
		"id": "featureB",
		"version": "1.0.0",
		"name": "Feature B"
	}`
	if err := os.WriteFile(filepath.Join(featureB, "devcontainer-feature.json"), []byte(featureBMeta), 0o644); err != nil {
		t.Fatalf("write featureB meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(featureB, "install.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write featureB install: %v", err)
	}
	featureA := filepath.Join(configDir, "featureA")
	if err := os.MkdirAll(featureA, 0o755); err != nil {
		t.Fatalf("mkdir featureA: %v", err)
	}
	featureAMeta := `{
		"id": "featureA",
		"version": "1.0.0",
		"name": "Feature A",
		"dependsOn": {
			"./featureB": {}
		}
	}`
	if err := os.WriteFile(filepath.Join(featureA, "devcontainer-feature.json"), []byte(featureAMeta), 0o644); err != nil {
		t.Fatalf("write featureA meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(featureA, "install.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write featureA install: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	workspaceRoot, _, _, _, err := resolveWorkspacePaths(configPath, cfg)
	if err != nil {
		t.Fatalf("resolveWorkspacePaths: %v", err)
	}
	resolved, err := resolveFeatures(context.Background(), configPath, workspaceRoot, cfg)
	if err != nil {
		t.Fatalf("resolveFeatures: %v", err)
	}
	if resolved == nil || len(resolved.Order) != 2 {
		t.Fatalf("unexpected resolved features: %#v", resolved)
	}
	if resolved.Order[0].Metadata.ID != "featureB" || resolved.Order[1].Metadata.ID != "featureA" {
		t.Fatalf("unexpected feature order: %#v", resolved.Order)
	}
}

func stringPtr(value string) *string {
	return &value
}
