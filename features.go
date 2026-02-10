package devcontainer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FeatureOptionValue represents a feature option value that may be a string or bool.
type FeatureOptionValue struct {
	String *string // String holds the string value when the option is a string.
	Bool   *bool   // Bool holds the boolean value when the option is a bool.
}

// UnmarshalJSON loads a JSON string or boolean into FeatureOptionValue.
// Impact: It rejects null and sets either String or Bool based on the input type.
// Example:
//
//	var v devcontainer.FeatureOptionValue
//	_ = json.Unmarshal([]byte(`true`), &v)
//
// Similar: StringSlice.UnmarshalJSON loads arrays of strings, while FeatureOptionValue holds a single typed value.
func (v *FeatureOptionValue) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return errors.New("feature option value cannot be null")
	}
	switch data[0] {
	case '"':
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		v.String = &value
		v.Bool = nil
		return nil
	case 't', 'f':
		var value bool
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		v.Bool = &value
		v.String = nil
		return nil
	default:
		return fmt.Errorf("unsupported feature option value: %s", string(data))
	}
}

// StringValue converts a FeatureOptionValue to its string representation.
// Impact: Bool values become "true"/"false", and missing values return an error.
// Example:
//
//	value := true
//	v := devcontainer.FeatureOptionValue{Bool: &value}
//	s, err := v.StringValue()
//
// Similar: Directly reading String/Bool requires manual type checks; StringValue centralizes the conversion.
func (v FeatureOptionValue) StringValue() (string, error) {
	switch {
	case v.String != nil:
		return *v.String, nil
	case v.Bool != nil:
		if *v.Bool {
			return "true", nil
		}
		return "false", nil
	default:
		return "", errors.New("feature option value is missing")
	}
}

func (v FeatureOptionValue) matchesType(expected string) bool {
	switch expected {
	case "string":
		return v.String != nil
	case "boolean":
		return v.Bool != nil
	default:
		return false
	}
}

type FeatureOptions map[string]FeatureOptionValue

type FeatureSet map[string]FeatureOptions

// UnmarshalJSON loads a JSON feature map into FeatureSet.
// Impact: It rejects empty feature IDs or options and expands "feature": "1.0" into a version option.
// Example:
//
//	var fs devcontainer.FeatureSet
//	_ = json.Unmarshal([]byte(`{"ghcr.io/devcontainers/features/git":"1.0"}`), &fs)
//
// Similar: FeatureOptionValue.UnmarshalJSON parses a single option value, while FeatureSet structures full feature entries.
func (fs *FeatureSet) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed := make(FeatureSet, len(raw))
	for key, value := range raw {
		if strings.TrimSpace(key) == "" {
			return errors.New("feature id cannot be empty")
		}
		if len(value) == 0 || string(value) == "null" {
			return fmt.Errorf("feature %s options cannot be null", key)
		}
		switch value[0] {
		case '"':
			var version string
			if err := json.Unmarshal(value, &version); err != nil {
				return err
			}
			parsed[key] = FeatureOptions{"version": {String: &version}}
		case '{':
			opts, err := parseFeatureOptions(value)
			if err != nil {
				return fmt.Errorf("feature %s options: %w", key, err)
			}
			parsed[key] = opts
		default:
			return fmt.Errorf("feature %s options must be string or object", key)
		}
	}
	*fs = parsed
	return nil
}

func parseFeatureOptions(data []byte) (FeatureOptions, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return FeatureOptions{}, nil
	}
	opts := make(FeatureOptions, len(raw))
	for key, value := range raw {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("feature option key cannot be empty")
		}
		var parsed FeatureOptionValue
		if err := json.Unmarshal(value, &parsed); err != nil {
			return nil, fmt.Errorf("option %s: %w", key, err)
		}
		opts[key] = parsed
	}
	return opts, nil
}

// FeatureOptionDefinition describes a feature option declared in metadata.
type FeatureOptionDefinition struct {
	Type        string             `json:"type"`        // Type is the option type, such as string or boolean.
	Default     FeatureOptionValue `json:"default"`     // Default is the default option value.
	Enum        []string           `json:"enum"`        // Enum lists allowed values.
	Proposals   []string           `json:"proposals"`   // Proposals lists suggested values for tooling.
	Description string             `json:"description"` // Description explains the option meaning.
}

// FeatureMount describes a mount contributed by a feature.
type FeatureMount struct {
	Type   string `json:"type"`   // Type is the mount type.
	Source string `json:"source"` // Source is the mount source.
	Target string `json:"target"` // Target is the mount destination in the container.
}

// FeatureMetadata represents the devcontainer-feature.json payload.
type FeatureMetadata struct {
	ID                   string                             `json:"id"`                   // ID is the canonical feature identifier.
	Version              string                             `json:"version"`              // Version is the feature version string.
	Name                 string                             `json:"name"`                 // Name is the human-readable feature name.
	Description          string                             `json:"description"`          // Description explains the feature behavior.
	DocumentationURL     string                             `json:"documentationURL"`     // DocumentationURL points to feature docs.
	LicenseURL           string                             `json:"licenseURL"`           // LicenseURL points to the feature license.
	Keywords             []string                           `json:"keywords"`             // Keywords lists search keywords.
	Options              map[string]FeatureOptionDefinition `json:"options"`              // Options declares configurable feature options.
	ContainerEnv         map[string]string                  `json:"containerEnv"`         // ContainerEnv exports environment variables.
	Privileged           bool                               `json:"privileged"`           // Privileged requests privileged container mode.
	Init                 *bool                              `json:"init"`                 // Init controls Docker init usage.
	CapAdd               []string                           `json:"capAdd"`               // CapAdd adds Linux capabilities.
	SecurityOpt          []string                           `json:"securityOpt"`          // SecurityOpt supplies security options.
	Entrypoint           string                             `json:"entrypoint"`           // Entrypoint points to a feature entrypoint script.
	Customizations       map[string]any                     `json:"customizations"`       // Customizations exposes editor/tooling settings.
	DependsOn            FeatureSet                         `json:"dependsOn"`            // DependsOn declares dependent features.
	InstallsAfter        []string                           `json:"installsAfter"`        // InstallsAfter lists features that should be installed first.
	LegacyIds            []string                           `json:"legacyIds"`            // LegacyIds lists legacy feature identifiers.
	Deprecated           bool                               `json:"deprecated"`           // Deprecated marks the feature as deprecated.
	Mounts               []FeatureMount                     `json:"mounts"`               // Mounts declares feature-provided mounts.
	OnCreateCommand      *LifecycleCommands                 `json:"onCreateCommand"`      // OnCreateCommand runs after container create.
	UpdateContentCommand *LifecycleCommands                 `json:"updateContentCommand"` // UpdateContentCommand runs after content update.
	PostCreateCommand    *LifecycleCommands                 `json:"postCreateCommand"`    // PostCreateCommand runs after creation tasks.
	PostStartCommand     *LifecycleCommands                 `json:"postStartCommand"`     // PostStartCommand runs after container start.
	PostAttachCommand    *LifecycleCommands                 `json:"postAttachCommand"`    // PostAttachCommand runs after attach.
}

type FeatureSource string

const (
	FeatureSourceOCI   FeatureSource = "oci"
	FeatureSourceHTTP  FeatureSource = "http"
	FeatureSourceLocal FeatureSource = "local"
)

// FeatureReference describes a parsed feature reference and its source.
type FeatureReference struct {
	ID         string        // ID is the raw feature identifier string.
	Source     FeatureSource // Source indicates OCI, HTTP, or local resolution.
	Registry   string        // Registry is the OCI registry hostname when Source is OCI.
	Repository string        // Repository is the OCI repository name when Source is OCI.
	Reference  string        // Reference is the OCI tag or digest.
	URL        string        // URL is the HTTP URL when Source is HTTP.
	LocalPath  string        // LocalPath is the path when Source is local.
}

// ResolvedFeatureOptions holds resolved option values for a feature.
type ResolvedFeatureOptions struct {
	Values     map[string]string // Values are resolved option values (defaults + overrides).
	UserValues map[string]string // UserValues are the values explicitly provided by users.
}

// ResolvedFeature contains metadata and resolved paths for a feature.
type ResolvedFeature struct {
	Reference         FeatureReference       // Reference is the parsed feature reference.
	Metadata          FeatureMetadata        // Metadata is the parsed feature metadata file.
	FeatureDir        string                 // FeatureDir is the resolved feature directory.
	ImageDir          string                 // ImageDir is the optional image build directory.
	Options           ResolvedFeatureOptions // Options holds resolved option values.
	DependencyKey     string                 // DependencyKey is the unique key for dependency resolution.
	DependsOnKeys     []string               // DependsOnKeys are dependency keys for required features.
	InstallsAfterIDs  []string               // InstallsAfterIDs are normalized IDs to install after.
	InstallsAfterKeys []string               // InstallsAfterKeys are resolved keys to install after.
	BaseName          string                 // BaseName is the normalized feature name.
	Tag               string                 // Tag is the OCI tag when resolved from OCI.
	CanonicalName     string                 // CanonicalName is the canonical identifier with digest.
}

// ResolvedFeatures aggregates resolved features and their merged config.
type ResolvedFeatures struct {
	Order        []*ResolvedFeature // Order is the installation order for features.
	ContainerEnv map[string]string  // ContainerEnv is the merged container environment.
	Mounts       []MountSpec        // Mounts are the merged mount specs.
	Privileged   bool               // Privileged indicates whether privileged mode is required.
	Init         *bool              // Init reflects merged init settings.
	CapAdd       []string           // CapAdd is the merged capability list.
	SecurityOpt  []string           // SecurityOpt is the merged security options list.
}

// featureResolver tracks state while resolving feature references.
type featureResolver struct {
	configDir       string                      // configDir is the directory of the devcontainer config.
	devcontainerDir string                      // devcontainerDir is the workspace .devcontainer directory.
	resolving       map[string]struct{}         // resolving tracks in-progress dependency keys.
	resolved        map[string]*ResolvedFeature // resolved caches resolved features by key.
	features        []*ResolvedFeature          // features is the list of resolved features.
	registry        *registryClient             // registry provides feature registry access.
}

func resolveFeatures(ctx context.Context, configPath, workspaceRoot string, cfg *DevcontainerConfig) (*ResolvedFeatures, error) {
	if len(cfg.Features) == 0 {
		return nil, nil
	}
	devcontainerDir := filepath.Join(workspaceRoot, ".devcontainer")
	configDir := filepath.Dir(configPath)
	resolver := &featureResolver{
		configDir:       configDir,
		devcontainerDir: devcontainerDir,
		resolving:       make(map[string]struct{}),
		resolved:        make(map[string]*ResolvedFeature),
		registry:        newRegistryClient(),
	}
	ids := make([]string, 0, len(cfg.Features))
	for id := range cfg.Features {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		options := cfg.Features[id]
		if _, err := resolver.resolveRequest(ctx, id, options); err != nil {
			return nil, err
		}
	}
	ordered, err := orderFeatures(resolver.features, cfg.OverrideFeatureInstallOrder)
	if err != nil {
		return nil, err
	}
	featureConfig := aggregateFeatureConfig(ordered)
	return &ResolvedFeatures{
		Order:        ordered,
		ContainerEnv: featureConfig.containerEnv,
		Mounts:       featureConfig.mounts,
		Privileged:   featureConfig.privileged,
		Init:         featureConfig.init,
		CapAdd:       featureConfig.capAdd,
		SecurityOpt:  featureConfig.securityOpt,
	}, nil
}

func (r *featureResolver) resolveRequest(ctx context.Context, id string, options FeatureOptions) (*ResolvedFeature, error) {
	reference, err := parseFeatureReference(id)
	if err != nil {
		return nil, err
	}
	reqKey, err := featureRequestKey(reference, options)
	if err != nil {
		return nil, err
	}
	if _, ok := r.resolving[reqKey]; ok {
		return nil, fmt.Errorf("feature dependency cycle detected at %s", id)
	}
	if existing, ok := r.resolved[reqKey]; ok {
		return existing, nil
	}
	r.resolving[reqKey] = struct{}{}
	defer delete(r.resolving, reqKey)

	resolved, err := r.fetchAndParse(ctx, reference, options)
	if err != nil {
		return nil, err
	}
	if existing, ok := r.resolved[resolved.DependencyKey]; ok {
		return existing, nil
	}
	r.resolved[resolved.DependencyKey] = resolved
	r.features = append(r.features, resolved)

	for depID, depOptions := range resolved.Metadata.DependsOn {
		dep, err := r.resolveRequest(ctx, depID, depOptions)
		if err != nil {
			return nil, err
		}
		resolved.DependsOnKeys = append(resolved.DependsOnKeys, dep.DependencyKey)
	}
	resolved.InstallsAfterIDs = normalizeIDs(resolved.Metadata.InstallsAfter)
	return resolved, nil
}

func (r *featureResolver) fetchAndParse(ctx context.Context, reference FeatureReference, options FeatureOptions) (*ResolvedFeature, error) {
	var (
		featureDir  string
		digest      string
		canonicalID string
		tag         string
		baseName    string
		err         error
	)
	switch reference.Source {
	case FeatureSourceLocal:
		featureDir, err = resolveLocalFeaturePath(reference.LocalPath, r.configDir, r.devcontainerDir)
		if err != nil {
			return nil, err
		}
		digest = localFeatureDigest(featureDir)
		baseName = normalizeFeatureID(reference.LocalPath)
		canonicalID = baseName
	case FeatureSourceHTTP:
		featureDir, digest, err = r.registry.fetchHTTPFeature(ctx, reference.URL)
		if err != nil {
			return nil, err
		}
		baseName = normalizeFeatureID(reference.URL)
		canonicalID = fmt.Sprintf("%s@%s", baseName, digest)
	case FeatureSourceOCI:
		featureDir, digest, err = r.registry.fetchOCIFeature(ctx, reference.Registry, reference.Repository, reference.Reference)
		if err != nil {
			return nil, err
		}
		tag = reference.Reference
		baseName = fmt.Sprintf("%s/%s", strings.ToLower(reference.Registry), strings.ToLower(reference.Repository))
		canonicalID = fmt.Sprintf("%s@%s", baseName, digest)
	default:
		return nil, fmt.Errorf("unsupported feature source: %s", reference.Source)
	}

	metadata, err := readFeatureMetadata(featureDir)
	if err != nil {
		return nil, err
	}
	if err := validateFeatureMetadata(metadata); err != nil {
		return nil, err
	}
	if reference.Source == FeatureSourceLocal {
		if err := validateFeatureDirName(metadata.ID, featureDir); err != nil {
			return nil, err
		}
	}
	resolvedOptions, err := resolveFeatureOptions(metadata.Options, options)
	if err != nil {
		return nil, err
	}
	dependencyKey := featureEqualityKey(reference.Source, digest, resolvedOptions.Values)
	return &ResolvedFeature{
		Reference:     reference,
		Metadata:      metadata,
		FeatureDir:    featureDir,
		Options:       resolvedOptions,
		DependencyKey: dependencyKey,
		BaseName:      baseName,
		Tag:           tag,
		CanonicalName: canonicalID,
	}, nil
}

func readFeatureMetadata(featureDir string) (FeatureMetadata, error) {
	path := filepath.Join(featureDir, "devcontainer-feature.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return FeatureMetadata{}, err
	}
	var metadata FeatureMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return FeatureMetadata{}, err
	}
	return metadata, nil
}

func validateFeatureMetadata(metadata FeatureMetadata) error {
	if metadata.ID == "" || metadata.Version == "" || metadata.Name == "" {
		return errors.New("devcontainer-feature.json requires id, version, and name")
	}
	return nil
}

func validateFeatureDirName(id, featureDir string) error {
	expected := normalizeFeatureID(id)
	actual := normalizeFeatureID(filepath.Base(featureDir))
	if expected != actual {
		return fmt.Errorf("feature directory name %s does not match id %s", filepath.Base(featureDir), id)
	}
	return nil
}

func resolveFeatureOptions(defs map[string]FeatureOptionDefinition, user FeatureOptions) (ResolvedFeatureOptions, error) {
	resolved := ResolvedFeatureOptions{
		Values:     map[string]string{},
		UserValues: map[string]string{},
	}
	if len(user) > 0 && len(defs) == 0 {
		return resolved, errors.New("feature does not declare any options")
	}
	for key := range user {
		if _, ok := defs[key]; !ok && len(defs) > 0 {
			return resolved, fmt.Errorf("unsupported feature option: %s", key)
		}
	}
	for name, def := range defs {
		if def.Type == "" {
			return resolved, fmt.Errorf("feature option %s missing type", name)
		}
		if !def.Default.matchesType(def.Type) {
			return resolved, fmt.Errorf("feature option %s default does not match type %s", name, def.Type)
		}
		if value, ok := user[name]; ok {
			if !value.matchesType(def.Type) {
				return resolved, fmt.Errorf("feature option %s expects %s", name, def.Type)
			}
			stringValue, err := value.StringValue()
			if err != nil {
				return resolved, err
			}
			resolved.Values[name] = stringValue
			resolved.UserValues[name] = stringValue
			continue
		}
		defaultValue, err := def.Default.StringValue()
		if err != nil {
			return resolved, err
		}
		resolved.Values[name] = defaultValue
	}
	return resolved, nil
}

func normalizeFeatureID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func normalizeIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		normalized = append(normalized, normalizeFeatureID(id))
	}
	return normalized
}

var optionNamePattern = regexp.MustCompile(`[^\w_]`)
var leadingOptionPattern = regexp.MustCompile(`^[\d_]+`)

func normalizeOptionEnvName(name string) string {
	clean := optionNamePattern.ReplaceAllString(name, "_")
	clean = leadingOptionPattern.ReplaceAllString(clean, "_")
	return strings.ToUpper(clean)
}

func featureEqualityKey(source FeatureSource, digest string, options map[string]string) string {
	hash := hashFeatureOptions(options)
	switch source {
	case FeatureSourceLocal:
		return fmt.Sprintf("local:%s:%s", digest, hash)
	case FeatureSourceHTTP:
		return fmt.Sprintf("http:%s:%s", digest, hash)
	default:
		return fmt.Sprintf("oci:%s:%s", digest, hash)
	}
}

func featureRequestKey(ref FeatureReference, options FeatureOptions) (string, error) {
	values := make(map[string]string, len(options))
	for key, value := range options {
		stringValue, err := value.StringValue()
		if err != nil {
			return "", err
		}
		values[key] = stringValue
	}
	return fmt.Sprintf("%s:%s:%s", ref.Source, normalizeFeatureID(ref.ID), hashFeatureOptions(values)), nil
}

func hashFeatureOptions(options map[string]string) string {
	if len(options) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hasher := sha256.New()
	for _, key := range keys {
		_, _ = hasher.Write([]byte(key))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(options[key]))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func localFeatureDigest(path string) string {
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(sum[:]))
}

// featureConfig aggregates configuration contributed by resolved features.
type featureConfig struct {
	containerEnv map[string]string // containerEnv merges container env variables.
	mounts       []MountSpec       // mounts merges feature-provided mounts.
	privileged   bool              // privileged indicates privileged mode is required.
	init         *bool             // init holds merged init preference.
	capAdd       []string          // capAdd is the merged capability list.
	securityOpt  []string          // securityOpt is the merged security options list.
}

func aggregateFeatureConfig(features []*ResolvedFeature) featureConfig {
	cfg := featureConfig{
		containerEnv: make(map[string]string),
	}
	for _, feature := range features {
		for key, value := range feature.Metadata.ContainerEnv {
			cfg.containerEnv[key] = value
		}
		for _, mount := range feature.Metadata.Mounts {
			cfg.mounts = append(cfg.mounts, MountSpec{
				Type:   mount.Type,
				Source: mount.Source,
				Target: mount.Target,
			})
		}
		if feature.Metadata.Privileged {
			cfg.privileged = true
		}
		if feature.Metadata.Init != nil && *feature.Metadata.Init {
			cfg.init = feature.Metadata.Init
		}
		cfg.capAdd = appendUnique(cfg.capAdd, feature.Metadata.CapAdd...)
		cfg.securityOpt = appendUnique(cfg.securityOpt, feature.Metadata.SecurityOpt...)
	}
	return cfg
}

func appendUnique(items []string, values ...string) []string {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		items = append(items, value)
		seen[value] = struct{}{}
	}
	return items
}

func orderFeatures(features []*ResolvedFeature, override []string) ([]*ResolvedFeature, error) {
	if len(features) == 0 {
		return nil, nil
	}
	baseNameToKeys := make(map[string][]string)
	for _, feature := range features {
		baseNameToKeys[feature.BaseName] = append(baseNameToKeys[feature.BaseName], feature.DependencyKey)
	}
	for _, feature := range features {
		for _, id := range feature.InstallsAfterIDs {
			if keys, ok := baseNameToKeys[id]; ok {
				feature.InstallsAfterKeys = append(feature.InstallsAfterKeys, keys...)
			}
		}
	}
	nodes := make(map[string]*ResolvedFeature, len(features))
	for _, feature := range features {
		nodes[feature.DependencyKey] = feature
	}
	priority := computeOverridePriority(override)
	remaining := make(map[string]struct{}, len(features))
	for _, feature := range features {
		remaining[feature.DependencyKey] = struct{}{}
	}
	var order []*ResolvedFeature
	for len(remaining) > 0 {
		var round []*ResolvedFeature
		for key := range remaining {
			node := nodes[key]
			if canInstall(node, order) {
				round = append(round, node)
			}
		}
		if len(round) == 0 {
			return nil, errors.New("feature dependency cycle detected")
		}
		maxPriority := 0
		for _, node := range round {
			if value, ok := priority[node.BaseName]; ok && value > maxPriority {
				maxPriority = value
			}
		}
		var commit []*ResolvedFeature
		for _, node := range round {
			if priority[node.BaseName] == maxPriority {
				commit = append(commit, node)
			}
		}
		sort.SliceStable(commit, func(i, j int) bool {
			return featureLess(commit[i], commit[j])
		})
		for _, node := range commit {
			order = append(order, node)
			delete(remaining, node.DependencyKey)
		}
	}
	if err := validateOverrideUsage(priority, features); err != nil {
		return nil, err
	}
	return order, nil
}

func computeOverridePriority(ids []string) map[string]int {
	if len(ids) == 0 {
		return map[string]int{}
	}
	priority := make(map[string]int, len(ids))
	total := len(ids)
	for idx, id := range ids {
		normalized := normalizeFeatureID(id)
		if normalized == "" {
			continue
		}
		priority[normalized] = total - idx
	}
	return priority
}

func validateOverrideUsage(priority map[string]int, features []*ResolvedFeature) error {
	if len(priority) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(features))
	for _, feature := range features {
		known[feature.BaseName] = struct{}{}
	}
	for id := range priority {
		if _, ok := known[id]; !ok {
			return fmt.Errorf("overrideFeatureInstallOrder includes unknown feature: %s", id)
		}
	}
	return nil
}

func canInstall(node *ResolvedFeature, installed []*ResolvedFeature) bool {
	installedSet := make(map[string]struct{}, len(installed))
	for _, feature := range installed {
		installedSet[feature.DependencyKey] = struct{}{}
	}
	for _, dep := range node.DependsOnKeys {
		if _, ok := installedSet[dep]; !ok {
			return false
		}
	}
	for _, dep := range node.InstallsAfterKeys {
		if dep == "" {
			continue
		}
		if _, ok := installedSet[dep]; !ok {
			return false
		}
	}
	return true
}

func featureLess(a, b *ResolvedFeature) bool {
	if a.BaseName != b.BaseName {
		return a.BaseName < b.BaseName
	}
	if a.Tag != b.Tag {
		return compareFeatureTag(a.Tag, b.Tag) < 0
	}
	aCount := len(a.Options.UserValues)
	bCount := len(b.Options.UserValues)
	if aCount != bCount {
		return aCount > bCount
	}
	aKeys := sortedKeys(a.Options.UserValues)
	bKeys := sortedKeys(b.Options.UserValues)
	if diff := compareStringSlices(aKeys, bKeys); diff != 0 {
		return diff < 0
	}
	aValues := valuesForKeys(a.Options.UserValues, aKeys)
	bValues := valuesForKeys(b.Options.UserValues, bKeys)
	if diff := compareStringSlices(aValues, bValues); diff != 0 {
		return diff < 0
	}
	return a.CanonicalName < b.CanonicalName
}

func compareFeatureTag(a, b string) int {
	if a == b {
		return 0
	}
	if a == "latest" {
		return 1
	}
	if b == "latest" {
		return -1
	}
	aParts, aOK := parseSemver(a)
	bParts, bOK := parseSemver(b)
	if aOK && bOK {
		for i := 0; i < len(aParts) || i < len(bParts); i++ {
			aVal := 0
			if i < len(aParts) {
				aVal = aParts[i]
			}
			bVal := 0
			if i < len(bParts) {
				bVal = bParts[i]
			}
			if aVal != bVal {
				if aVal < bVal {
					return -1
				}
				return 1
			}
		}
		return 0
	}
	if a < b {
		return -1
	}
	return 1
}

func parseSemver(value string) ([]int, bool) {
	if value == "" {
		return nil, false
	}
	parts := strings.Split(value, ".")
	parsed := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		parsed = append(parsed, number)
	}
	return parsed, true
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func valuesForKeys(values map[string]string, keys []string) []string {
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func compareStringSlices(a, b []string) int {
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		if a[i] == b[i] {
			continue
		}
		if a[i] < b[i] {
			return -1
		}
		return 1
	}
	return 0
}

func parseFeatureReference(id string) (FeatureReference, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return FeatureReference{}, errors.New("feature id cannot be empty")
	}
	normalized := normalizeFeatureID(trimmed)
	if strings.HasPrefix(normalized, "http://") || strings.HasPrefix(normalized, "https://") {
		return FeatureReference{ID: trimmed, Source: FeatureSourceHTTP, URL: trimmed}, nil
	}
	if strings.HasPrefix(trimmed, ".") {
		return FeatureReference{ID: trimmed, Source: FeatureSourceLocal, LocalPath: trimmed}, nil
	}
	registry, repository, reference, err := parseOCIReference(trimmed)
	if err != nil {
		return FeatureReference{}, err
	}
	return FeatureReference{
		ID:         trimmed,
		Source:     FeatureSourceOCI,
		Registry:   registry,
		Repository: repository,
		Reference:  reference,
	}, nil
}

func parseOCIReference(id string) (string, string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid OCI feature reference: %s", id)
	}
	registry := parts[0]
	repoParts := parts[1:]
	repo := strings.Join(repoParts, "/")
	ref := "latest"
	if strings.Contains(repo, "@") {
		items := strings.SplitN(repo, "@", 2)
		repo = items[0]
		ref = items[1]
		return registry, repo, ref, nil
	}
	last := repoParts[len(repoParts)-1]
	if strings.Contains(last, ":") {
		idx := strings.LastIndex(last, ":")
		if idx == -1 {
			return registry, repo, ref, nil
		}
		tag := last[idx+1:]
		if tag == "" {
			return "", "", "", fmt.Errorf("invalid OCI feature tag: %s", id)
		}
		repoParts[len(repoParts)-1] = last[:idx]
		repo = strings.Join(repoParts, "/")
		ref = tag
	}
	return registry, repo, ref, nil
}

func resolveLocalFeaturePath(relativePath, configDir, devcontainerDir string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", errors.New("local feature path must be relative")
	}
	info, err := os.Stat(devcontainerDir)
	if err != nil || !info.IsDir() {
		return "", errors.New("local features require .devcontainer directory")
	}
	abs := filepath.Clean(filepath.Join(configDir, relativePath))
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(devcontainerDir, abs)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("local feature path must be inside %s", devcontainerDir)
	}
	return abs, nil
}

func renderFeatureEnvFile(options map[string]string, extra map[string]string) string {
	lines := make([]string, 0, len(options)+len(extra))
	keys := sortedKeys(options)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", normalizeOptionEnvName(key), quoteEnvValue(options[key])))
	}
	extraKeys := sortedKeys(extra)
	for _, key := range extraKeys {
		lines = append(lines, fmt.Sprintf("%s=%s", key, quoteEnvValue(extra[key])))
	}
	return strings.Join(lines, "\n") + "\n"
}

func quoteEnvValue(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "$", "\\$")
	return fmt.Sprintf("\"%s\"", escaped)
}

func featureEntrypointPath(feature *ResolvedFeature, vars map[string]string) (string, error) {
	if feature.Metadata.Entrypoint == "" {
		return "", nil
	}
	entrypoint := feature.Metadata.Entrypoint
	var err error
	if strings.Contains(entrypoint, "${") {
		entrypoint, err = expandVariables(entrypoint, vars, nil)
		if err != nil {
			return "", err
		}
	}
	if !path.IsAbs(entrypoint) {
		entrypoint = path.Join(feature.ImageDir, entrypoint)
	}
	return entrypoint, nil
}
