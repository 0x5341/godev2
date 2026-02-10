package devcontainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type DevcontainerConfig struct {
	Name                        string             `json:"name"`
	Image                       string             `json:"image"`
	Build                       *DevcontainerBuild `json:"build"`
	DockerComposeFile           StringSlice        `json:"dockerComposeFile"`
	Service                     string             `json:"service"`
	RunServices                 []string           `json:"runServices"`
	ShutdownAction              string             `json:"shutdownAction"`
	ForwardPorts                PortList           `json:"forwardPorts"`
	AppPort                     PortList           `json:"appPort"`
	ContainerEnv                map[string]string  `json:"containerEnv"`
	Mounts                      []MountSpec        `json:"mounts"`
	WorkspaceMount              string             `json:"workspaceMount"`
	WorkspaceFolder             string             `json:"workspaceFolder"`
	RunArgs                     []string           `json:"runArgs"`
	Privileged                  bool               `json:"privileged"`
	CapAdd                      []string           `json:"capAdd"`
	SecurityOpt                 []string           `json:"securityOpt"`
	Init                        *bool              `json:"init"`
	ContainerUser               string             `json:"containerUser"`
	RemoteUser                  string             `json:"remoteUser"`
	RemoteEnv                   map[string]string  `json:"remoteEnv"`
	Features                    FeatureSet         `json:"features"`
	OverrideFeatureInstallOrder []string           `json:"overrideFeatureInstallOrder"`
	OverrideCommand             *bool              `json:"overrideCommand"`
	InitializeCommand           *LifecycleCommands `json:"initializeCommand"`
	OnCreateCommand             *LifecycleCommands `json:"onCreateCommand"`
	UpdateContentCommand        *LifecycleCommands `json:"updateContentCommand"`
	PostCreateCommand           *LifecycleCommands `json:"postCreateCommand"`
	PostStartCommand            *LifecycleCommands `json:"postStartCommand"`
	PostAttachCommand           *LifecycleCommands `json:"postAttachCommand"`
}

type DevcontainerBuild struct {
	Dockerfile string            `json:"dockerfile"`
	Context    string            `json:"context"`
	Args       map[string]string `json:"args"`
	Target     string            `json:"target"`
	CacheFrom  StringSlice       `json:"cacheFrom"`
	Options    []string          `json:"options"`
}

type StringSlice []string

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

type MountSpec struct {
	Raw    string
	Type   string
	Source string
	Target string
}

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
