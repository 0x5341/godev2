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

// UnmarshalJSON は StringSlice に JSON の文字列または文字列配列を読み込む。
// 影響: devcontainer.json の項目で "value" と ["value1","value2"] の両方を許可し、型が不正な場合はエラーになる。
// 例:
//
//	var s devcontainer.StringSlice
//	_ = json.Unmarshal([]byte(`["a","b"]`), &s)
//
// 類似: PortList の UnmarshalJSON は数値も受け付けて文字列化するが、StringSlice は文字列のみ。
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

// UnmarshalJSON は PortList に数値/文字列または配列のポート指定を読み込む。
// 影響: 3000 などの数値は "3000" に正規化され、無効な値はエラーになる。
// 例:
//
//	var p devcontainer.PortList
//	_ = json.Unmarshal([]byte(`[3000,"9229"]`), &p)
//
// 類似: StringSlice の UnmarshalJSON は数値を受け付けず、ポート正規化もしない。
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

// UnmarshalJSON は MountSpec に JSON の文字列またはオブジェクト形式のマウント指定を読み込む。
// 影響: "type" と "target" が欠けるとエラーになり、文字列形式は Raw に保持される。
// 例:
//
//	var m devcontainer.MountSpec
//	_ = json.Unmarshal([]byte(`{"type":"bind","source":"/tmp","target":"/work"}`), &m)
//
// 類似: ParseMountSpec は CLI の --mount 文字列を Mount に変換する点が異なる。
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

// LoadConfig は devcontainer.json を読み込み、コメントを除去して DevcontainerConfig にデコードする。
// 影響: ファイル I/O を行い、無効な JSON や仕様違反はエラーになる。
// 例:
//
//	cfg, err := devcontainer.LoadConfig("./.devcontainer/devcontainer.json")
//
// 類似: FindConfigPath はパス探索のみ行い、読み込みやデコードはしない。
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

// FindConfigPath は baseDir から devcontainer.json を探索して最初に見つかったパスを返す。
// 影響: ファイルシステムの存在確認を行い、見つからない場合はエラーになる。
// 例:
//
//	path, err := devcontainer.FindConfigPath(".")
//
// 類似: LoadConfig はパスが分かっている前提で読み込みとデコードを行う。
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
