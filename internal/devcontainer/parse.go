package devcontainer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

func stripJSONComments(input []byte) ([]byte, error) {
	var out bytes.Buffer
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				out.WriteByte(ch)
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(input) && input[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == '/' && i+1 < len(input) {
			if input[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if input[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		}
		out.WriteByte(ch)
	}

	if inBlockComment {
		return nil, errors.New("unterminated block comment")
	}
	return out.Bytes(), nil
}

func resolveWorkspacePaths(configPath string, cfg *DevcontainerConfig) (string, string, string, map[string]string, error) {
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return "", "", "", nil, err
	}
	configDir := filepath.Dir(absConfig)
	workspaceRoot := configDir
	if filepath.Base(configDir) == ".devcontainer" {
		workspaceRoot = filepath.Dir(configDir)
	}
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		return "", "", "", nil, err
	}

	workspaceFolder := cfg.WorkspaceFolder
	if workspaceFolder == "" {
		workspaceFolder = path.Join("/workspaces", filepath.Base(workspaceRoot))
	}

	workspaceMount := cfg.WorkspaceMount
	if workspaceMount == "" {
		workspaceMount = fmt.Sprintf("source=%s,target=%s,type=bind", workspaceRoot, workspaceFolder)
	}

	devcontainerID := devcontainerID(workspaceRoot, absConfig)
	vars := map[string]string{
		"localWorkspaceFolder":             workspaceRoot,
		"localWorkspaceFolderBasename":     filepath.Base(workspaceRoot),
		"containerWorkspaceFolder":         workspaceFolder,
		"containerWorkspaceFolderBasename": path.Base(workspaceFolder),
		"devcontainerId":                   devcontainerID,
	}
	return workspaceRoot, workspaceFolder, workspaceMount, vars, nil
}

func devcontainerID(workspaceRoot, configPath string) string {
	sum := sha256.Sum256([]byte(workspaceRoot + "::" + configPath))
	return hex.EncodeToString(sum[:8])
}

var variablePattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func expandVariables(input string, vars map[string]string, containerEnv map[string]string) (string, error) {
	matches := variablePattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		out.WriteString(input[last:match[0]])
		token := input[match[2]:match[3]]
		value, err := resolveVariable(token, vars, containerEnv)
		if err != nil {
			return "", err
		}
		out.WriteString(value)
		last = match[1]
	}
	out.WriteString(input[last:])
	return out.String(), nil
}

func resolveVariable(token string, vars map[string]string, containerEnv map[string]string) (string, error) {
	if strings.HasPrefix(token, "localEnv:") {
		return resolveEnvVariable(strings.TrimPrefix(token, "localEnv:"))
	}
	if strings.HasPrefix(token, "containerEnv:") {
		key := strings.TrimPrefix(token, "containerEnv:")
		if value, ok := containerEnv[key]; ok {
			return value, nil
		}
		return resolveEnvVariable(key)
	}
	if value, ok := vars[token]; ok {
		return value, nil
	}
	if value, ok := containerEnv[token]; ok {
		return value, nil
	}
	if env := os.Getenv(token); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("unsupported variable: %s", token)
}

func resolveEnvVariable(token string) (string, error) {
	parts := strings.SplitN(token, ":", 2)
	env := os.Getenv(parts[0])
	if env == "" && len(parts) == 2 {
		return parts[1], nil
	}
	return env, nil
}

func mergeEnvMaps(base, overlay map[string]string, vars map[string]string) (map[string]string, error) {
	merged := make(map[string]string)
	for key, value := range base {
		expanded, err := expandVariables(value, vars, merged)
		if err != nil {
			return nil, err
		}
		merged[key] = expanded
	}
	for key, value := range overlay {
		expanded, err := expandVariables(value, vars, merged)
		if err != nil {
			return nil, err
		}
		merged[key] = expanded
	}
	return merged, nil
}

func envMapToSlice(envMap map[string]string) []string {
	if len(envMap) == 0 {
		return nil
	}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, fmt.Sprintf("%s=%s", key, envMap[key]))
	}
	return env
}

func collectPortSpecs(configPorts, appPorts PortList, extra []string) ([]string, error) {
	specs := make([]string, 0, len(configPorts)+len(appPorts)+len(extra))
	for _, item := range append(append([]string{}, configPorts...), appPorts...) {
		normalized, err := normalizePortSpec(item)
		if err != nil {
			return nil, err
		}
		specs = append(specs, normalized)
	}
	for _, item := range extra {
		normalized, err := normalizePortSpec(item)
		if err != nil {
			return nil, err
		}
		specs = append(specs, normalized)
	}
	return specs, nil
}

func normalizePortSpec(spec string) (string, error) {
	if spec == "" {
		return "", errors.New("empty port spec")
	}
	if strings.Contains(spec, ":") {
		parts := strings.SplitN(spec, ":", 2)
		if parts[0] != "" {
			if _, err := strconv.Atoi(parts[0]); err != nil {
				return "", fmt.Errorf("unsupported host in port spec: %s", spec)
			}
		}
		return spec, nil
	}
	proto := ""
	port := spec
	if strings.Contains(spec, "/") {
		sub := strings.SplitN(spec, "/", 2)
		port = sub[0]
		proto = sub[1]
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("invalid port spec: %s", spec)
	}
	if proto == "" {
		return fmt.Sprintf("%s:%s", port, port), nil
	}
	return fmt.Sprintf("%s:%s/%s", port, port, proto), nil
}

func parsePortSpecs(specs []string) (nat.PortSet, nat.PortMap, error) {
	if len(specs) == 0 {
		return nil, nil, nil
	}
	exposed, bindings, err := nat.ParsePortSpecs(specs)
	if err != nil {
		return nil, nil, err
	}
	return exposed, bindings, nil
}

func parseMountString(spec string) (mount.Mount, error) {
	parts := strings.Split(spec, ",")
	var result mount.Mount
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "readonly" || part == "ro" {
			result.ReadOnly = true
			continue
		}
		if !strings.Contains(part, "=") {
			return mount.Mount{}, fmt.Errorf("invalid mount option: %s", part)
		}
		keyValue := strings.SplitN(part, "=", 2)
		key := strings.ToLower(strings.TrimSpace(keyValue[0]))
		value := strings.TrimSpace(keyValue[1])
		switch key {
		case "type":
			result.Type = mount.Type(value)
		case "source", "src":
			result.Source = value
		case "target", "dst", "destination":
			result.Target = value
		case "consistency":
			result.Consistency = mount.Consistency(value)
		default:
			return mount.Mount{}, fmt.Errorf("unsupported mount option: %s", key)
		}
	}
	if result.Type == "" {
		result.Type = mount.TypeVolume
	}
	if result.Target == "" {
		return mount.Mount{}, errors.New("mount target is required")
	}
	return result, nil
}

func ParseMountSpec(spec string) (Mount, error) {
	parsed, err := parseMountString(spec)
	if err != nil {
		return Mount{}, err
	}
	return Mount{
		Source:      parsed.Source,
		Target:      parsed.Target,
		Type:        string(parsed.Type),
		ReadOnly:    parsed.ReadOnly,
		Consistency: string(parsed.Consistency),
	}, nil
}

func mountFromSpec(spec MountSpec) (mount.Mount, error) {
	if spec.Raw != "" {
		return parseMountString(spec.Raw)
	}
	if spec.Type == "" || spec.Target == "" {
		return mount.Mount{}, errors.New("mount requires type and target")
	}
	return mount.Mount{
		Type:   mount.Type(spec.Type),
		Source: spec.Source,
		Target: spec.Target,
	}, nil
}

func toDockerMount(m Mount) (mount.Mount, error) {
	if m.Target == "" {
		return mount.Mount{}, errors.New("mount target is required")
	}
	mountType := mount.Type(m.Type)
	if mountType == "" {
		mountType = mount.TypeVolume
	}
	return mount.Mount{
		Type:        mountType,
		Source:      m.Source,
		Target:      m.Target,
		ReadOnly:    m.ReadOnly,
		Consistency: mount.Consistency(m.Consistency),
	}, nil
}

type runArgOptions struct {
	CapAdd      []string
	SecurityOpt []string
	Privileged  bool
	Init        bool
	User        string
	Network     string
	Labels      map[string]string
}

func parseRunArgs(args []string) (runArgOptions, error) {
	var opts runArgOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--cap-add="):
			opts.CapAdd = append(opts.CapAdd, strings.TrimPrefix(arg, "--cap-add="))
		case arg == "--cap-add":
			value, err := nextRunArgValue(args, &i, arg)
			if err != nil {
				return runArgOptions{}, err
			}
			opts.CapAdd = append(opts.CapAdd, value)
		case strings.HasPrefix(arg, "--security-opt="):
			opts.SecurityOpt = append(opts.SecurityOpt, strings.TrimPrefix(arg, "--security-opt="))
		case arg == "--security-opt":
			value, err := nextRunArgValue(args, &i, arg)
			if err != nil {
				return runArgOptions{}, err
			}
			opts.SecurityOpt = append(opts.SecurityOpt, value)
		case arg == "--privileged":
			opts.Privileged = true
		case arg == "--init":
			opts.Init = true
		case strings.HasPrefix(arg, "--user="):
			opts.User = strings.TrimPrefix(arg, "--user=")
		case arg == "--user" || arg == "-u":
			value, err := nextRunArgValue(args, &i, arg)
			if err != nil {
				return runArgOptions{}, err
			}
			opts.User = value
		case strings.HasPrefix(arg, "--network="):
			opts.Network = strings.TrimPrefix(arg, "--network=")
		case arg == "--network":
			value, err := nextRunArgValue(args, &i, arg)
			if err != nil {
				return runArgOptions{}, err
			}
			opts.Network = value
		case strings.HasPrefix(arg, "--label="):
			if err := applyRunArgLabel(&opts, strings.TrimPrefix(arg, "--label=")); err != nil {
				return runArgOptions{}, err
			}
		case arg == "--label" || arg == "-l":
			value, err := nextRunArgValue(args, &i, arg)
			if err != nil {
				return runArgOptions{}, err
			}
			if err := applyRunArgLabel(&opts, value); err != nil {
				return runArgOptions{}, err
			}
		default:
			return runArgOptions{}, fmt.Errorf("unsupported runArg: %s", arg)
		}
	}
	return opts, nil
}

func nextRunArgValue(args []string, index *int, flag string) (string, error) {
	if *index+1 >= len(args) {
		return "", fmt.Errorf("missing value for %s", flag)
	}
	*index++
	return args[*index], nil
}

func applyRunArgLabel(opts *runArgOptions, value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || parts[0] == "" {
		return fmt.Errorf("invalid label: %s", value)
	}
	if opts.Labels == nil {
		opts.Labels = make(map[string]string)
	}
	opts.Labels[parts[0]] = parts[1]
	return nil
}
