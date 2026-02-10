package devcontainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DevcontainerConfig represents the decoded devcontainer.json configuration.
type DevcontainerConfig struct {
	Name                        string             `json:"name"`                        // Name is an optional container name override.
	Image                       string             `json:"image"`                       // Image is the base image reference when not building.
	Build                       *DevcontainerBuild `json:"build"`                       // Build describes Docker build settings for the devcontainer.
	DockerComposeFile           StringSlice        `json:"dockerComposeFile"`           // DockerComposeFile lists compose files for Docker Compose mode.
	Service                     string             `json:"service"`                     // Service selects the primary compose service.
	RunServices                 []string           `json:"runServices"`                 // RunServices lists additional compose services to start.
	ShutdownAction              string             `json:"shutdownAction"`              // ShutdownAction controls container shutdown behavior.
	ForwardPorts                PortList           `json:"forwardPorts"`                // ForwardPorts lists ports to forward from the container.
	AppPort                     PortList           `json:"appPort"`                     // AppPort lists application ports for devcontainer tooling.
	ContainerEnv                map[string]string  `json:"containerEnv"`                // ContainerEnv defines environment variables set in the container.
	Mounts                      []MountSpec        `json:"mounts"`                      // Mounts defines additional mounts for the container.
	WorkspaceMount              string             `json:"workspaceMount"`              // WorkspaceMount overrides the workspace mount spec.
	WorkspaceFolder             string             `json:"workspaceFolder"`             // WorkspaceFolder sets the workspace path inside the container.
	RunArgs                     []string           `json:"runArgs"`                     // RunArgs lists extra docker run arguments.
	Privileged                  bool               `json:"privileged"`                  // Privileged requests privileged container mode.
	CapAdd                      []string           `json:"capAdd"`                      // CapAdd adds Linux capabilities.
	SecurityOpt                 []string           `json:"securityOpt"`                 // SecurityOpt supplies security options to Docker.
	Init                        *bool              `json:"init"`                        // Init controls Docker init usage.
	ContainerUser               string             `json:"containerUser"`               // ContainerUser sets the user for the container process.
	RemoteUser                  string             `json:"remoteUser"`                  // RemoteUser sets the default user for lifecycle commands.
	RemoteEnv                   map[string]string  `json:"remoteEnv"`                   // RemoteEnv defines environment variables for remote commands.
	Features                    FeatureSet         `json:"features"`                    // Features declares requested devcontainer features.
	OverrideFeatureInstallOrder []string           `json:"overrideFeatureInstallOrder"` // OverrideFeatureInstallOrder forces feature install order.
	OverrideCommand             *bool              `json:"overrideCommand"`             // OverrideCommand controls entrypoint override behavior.
	InitializeCommand           *LifecycleCommands `json:"initializeCommand"`           // InitializeCommand runs on the host before container create.
	OnCreateCommand             *LifecycleCommands `json:"onCreateCommand"`             // OnCreateCommand runs after the container is created.
	UpdateContentCommand        *LifecycleCommands `json:"updateContentCommand"`        // UpdateContentCommand runs after content updates.
	PostCreateCommand           *LifecycleCommands `json:"postCreateCommand"`           // PostCreateCommand runs after creation tasks.
	PostStartCommand            *LifecycleCommands `json:"postStartCommand"`            // PostStartCommand runs after the container starts.
	PostAttachCommand           *LifecycleCommands `json:"postAttachCommand"`           // PostAttachCommand runs after attaching to the container.
}

// DevcontainerBuild describes Docker build settings from devcontainer.json.
type DevcontainerBuild struct {
	Dockerfile string            `json:"dockerfile"` // Dockerfile is the path to the Dockerfile.
	Context    string            `json:"context"`    // Context is the build context directory.
	Args       map[string]string `json:"args"`       // Args holds Docker build arguments.
	Target     string            `json:"target"`     // Target selects a specific build stage.
	CacheFrom  StringSlice       `json:"cacheFrom"`  // CacheFrom lists cache sources.
	Options    []string          `json:"options"`    // Options carries additional build options.
}

type StringSlice []string

// UnmarshalJSON loads a JSON string or string array into StringSlice.
// Impact: It accepts both "value" and ["value1","value2"] in devcontainer.json and errors on invalid types.
// Example:
//
//	var s devcontainer.StringSlice
//	_ = json.Unmarshal([]byte(`["a","b"]`), &s)
//
// Similar: PortList.UnmarshalJSON accepts numeric ports, while StringSlice only accepts strings.
func (s *StringSlice) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	switch data[0] {
	case '[':
		var values []string
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
		*s = values
		return nil
	case '"':
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*s = []string{value}
		return nil
	default:
		return fmt.Errorf("invalid string list: %s", string(data))
	}
}

type PortList []string

// UnmarshalJSON loads numeric or string port values (or arrays) into PortList.
// Impact: Numeric ports like 3000 are normalized to "3000", and invalid values return an error.
// Example:
//
//	var p devcontainer.PortList
//	_ = json.Unmarshal([]byte(`[3000,"9229"]`), &p)
//
// Similar: StringSlice.UnmarshalJSON does not accept numeric values or normalize ports.
func (p *PortList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	switch data[0] {
	case '[':
		var raw []json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		var values []string
		for _, item := range raw {
			value, err := parsePortValue(item)
			if err != nil {
				return err
			}
			values = append(values, value)
		}
		*p = values
		return nil
	default:
		value, err := parsePortValue(data)
		if err != nil {
			return err
		}
		*p = []string{value}
		return nil
	}
}

func parsePortValue(data []byte) (string, error) {
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		return fmt.Sprintf("%d", number), nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		return text, nil
	}
	return "", fmt.Errorf("invalid port value: %s", string(data))
}

// MountSpec represents a mount entry from devcontainer.json.
type MountSpec struct {
	Raw    string // Raw holds the original string-form mount value, if provided.
	Type   string // Type is the mount type, such as bind or volume.
	Source string // Source is the mount source path or volume name.
	Target string // Target is the mount destination inside the container.
}

// UnmarshalJSON loads a JSON string or object-form mount into MountSpec.
// Impact: Missing "type" or "target" returns an error, and string-form mounts are stored in Raw.
// Example:
//
//	var m devcontainer.MountSpec
//	_ = json.Unmarshal([]byte(`{"type":"bind","source":"/tmp","target":"/work"}`), &m)
//
// Similar: ParseMountSpec converts CLI --mount strings into Mount values.
func (m *MountSpec) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		m.Raw = raw
		return nil
	}
	var decoded struct {
		Type   string `json:"type"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Type == "" || decoded.Target == "" {
		return errors.New("mount requires type and target")
	}
	m.Type = decoded.Type
	m.Source = decoded.Source
	m.Target = decoded.Target
	return nil
}

// LoadConfig reads devcontainer.json, strips comments, and decodes it into DevcontainerConfig.
// Impact: It performs file I/O and returns errors for invalid JSON or spec violations.
// Example:
//
//	cfg, err := devcontainer.LoadConfig("./.devcontainer/devcontainer.json")
//
// Similar: FindConfigPath only searches for the file path without decoding it.
func LoadConfig(path string) (*DevcontainerConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	clean, err := stripJSONComments(content)
	if err != nil {
		return nil, err
	}
	var cfg DevcontainerConfig
	if err := json.Unmarshal(clean, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// FindConfigPath searches baseDir for devcontainer.json and returns the first match.
// Impact: It checks filesystem paths and returns an error when no config is found.
// Example:
//
//	path, err := devcontainer.FindConfigPath(".")
//
// Similar: LoadConfig assumes the path is known and performs decoding.
func FindConfigPath(baseDir string) (string, error) {
	candidates := []string{
		filepath.Join(baseDir, ".devcontainer", "devcontainer.json"),
		filepath.Join(baseDir, "devcontainer.json"),
	}
	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("devcontainer.json not found in %s", baseDir)
}
