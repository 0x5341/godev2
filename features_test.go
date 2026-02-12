package godev

import (
	"context"
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
	copyTestcaseDir(t, root, "features", "deps")
	configPath := filepath.Join(root, ".devcontainer", "devcontainer.json")

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
