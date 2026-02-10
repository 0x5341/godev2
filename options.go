package devcontainer

import "time"

type StartOption func(*startOptions)

type startOptions struct {
	ConfigPath   string
	Env          map[string]string
	ExtraPublish []string
	ExtraMounts  []Mount
	RunArgs      []string
	RemoveOnStop bool
	Detach       bool
	TTY          bool
	Labels       map[string]string
	Resources    ResourceLimits
	Network      string
	Timeout      time.Duration
	Workdir      string
}

type Mount struct {
	Source      string
	Target      string
	Type        string
	ReadOnly    bool
	Consistency string
}

type ResourceLimits struct {
	CPUQuota int64
	Memory   string
}

func defaultStartOptions() startOptions {
	return startOptions{
		Detach: true,
		TTY:    true,
	}
}

// WithConfigPath は StartDevcontainer で使用する devcontainer.json のパスを指定する。
// 影響: 探索ではなく指定パスが優先され、読み込み対象が固定される。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithConfigPath("./.devcontainer/devcontainer.json"))
//
// 類似: FindConfigPath はパス探索のみで StartDevcontainer の設定は行わない。
func WithConfigPath(path string) StartOption {
	return func(o *startOptions) {
		o.ConfigPath = path
	}
}

// WithEnv はコンテナの環境変数を 1 件追加する。
// 影響: devcontainer.json の containerEnv とマージされ、同名キーは上書きされる。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithEnv("FOO", "bar"))
//
// 類似: WithLabel は Docker ラベルを追加する点が異なる。
func WithEnv(key, value string) StartOption {
	return func(o *startOptions) {
		if o.Env == nil {
			o.Env = make(map[string]string)
		}
		o.Env[key] = value
	}
}

// WithExtraPublish は追加のポート公開設定を追加する。
// 影響: devcontainer.json の forwardPorts/appPort に加えて公開される。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithExtraPublish("3000:3000"))
//
// 類似: forwardPorts/appPort は設定ファイル由来であり、WithExtraPublish は実行時上書き。
func WithExtraPublish(mapping string) StartOption {
	return func(o *startOptions) {
		o.ExtraPublish = append(o.ExtraPublish, mapping)
	}
}

// WithExtraMount は追加のマウントを指定する。
// 影響: ワークスペースや設定済みマウントに追加され、コンテナ作成時の HostConfig に反映される。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithExtraMount(devcontainer.Mount{Source: "/tmp", Target: "/work", Type: "bind"}))
//
// 類似: ParseMountSpec は CLI 文字列を Mount に変換するだけで、オプションには追加しない。
func WithExtraMount(m Mount) StartOption {
	return func(o *startOptions) {
		o.ExtraMounts = append(o.ExtraMounts, m)
	}
}

// WithRunArg は Docker run 相当の追加引数を 1 件追加する。
// 影響: 一部の引数(--cap-add 等)は解析され、特権やネットワーク設定に影響する。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithRunArg("--cap-add=SYS_PTRACE"))
//
// 類似: WithResources/WithNetwork は構造化された設定であり、WithRunArg は文字列で指定する。
func WithRunArg(arg string) StartOption {
	return func(o *startOptions) {
		o.RunArgs = append(o.RunArgs, arg)
	}
}

// WithRemoveOnStop はコンテナ停止時に自動削除する設定を有効化する。
// 影響: Docker の AutoRemove が true になり、停止後にコンテナが残らない。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithRemoveOnStop())
//
// 類似: RemoveDevcontainer は既存コンテナを明示的に削除する点が異なる。
func WithRemoveOnStop() StartOption {
	return func(o *startOptions) {
		o.RemoveOnStop = true
	}
}

// WithDetach はコンテナをデタッチで起動するよう設定する。
// 影響: StartDevcontainer は起動完了後すぐに戻り、コンテナ終了を待たない。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithDetach())
//
// 類似: WithDetachValue は true/false を明示でき、false なら起動後の待機を行う。
func WithDetach() StartOption {
	return func(o *startOptions) {
		o.Detach = true
	}
}

// WithDetachValue はデタッチ実行の真偽値を明示的に設定する。
// 影響: false の場合、StartDevcontainer はコンテナ終了まで待機する。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithDetachValue(false))
//
// 類似: WithDetach は常に true を設定する簡易版。
func WithDetachValue(detach bool) StartOption {
	return func(o *startOptions) {
		o.Detach = detach
	}
}

// WithTTY は TTY を割り当てる設定を有効化する。
// 影響: コンテナ作成時の Tty が true になり、標準入出力の扱いが変わる。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithTTY())
//
// 類似: WithTTYValue は true/false を明示でき、false なら TTY 無しで起動する。
func WithTTY() StartOption {
	return func(o *startOptions) {
		o.TTY = true
	}
}

// WithTTYValue は TTY 割り当ての真偽値を明示的に設定する。
// 影響: false の場合、Tty が無効化される。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithTTYValue(false))
//
// 類似: WithTTY は常に true を設定する簡易版。
func WithTTYValue(tty bool) StartOption {
	return func(o *startOptions) {
		o.TTY = tty
	}
}

// WithLabel は Docker ラベルを 1 件追加する。
// 影響: 既存ラベルとマージされ、同名キーは上書きされる。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithLabel("team", "dev"))
//
// 類似: WithEnv は環境変数であり、ラベルとは用途が異なる。
func WithLabel(key, value string) StartOption {
	return func(o *startOptions) {
		if o.Labels == nil {
			o.Labels = make(map[string]string)
		}
		o.Labels[key] = value
	}
}

// WithTimeout は StartDevcontainer 全体のタイムアウトを設定する。
// 影響: タイムアウトを超えると Context がキャンセルされ、起動処理が中断される。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithTimeout(2*time.Minute))
//
// 類似: 呼び出し元で context.WithTimeout を使う方法でも同様だが、WithTimeout はオプションで一元設定する。
func WithTimeout(timeout time.Duration) StartOption {
	return func(o *startOptions) {
		o.Timeout = timeout
	}
}

// WithResources は CPU/メモリ制限を設定する。
// 影響: Docker HostConfig の CPUQuota/Memory が設定され、リソース制限が有効になる。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithResources(devcontainer.ResourceLimits{Memory: "1g"}))
//
// 類似: WithRunArg でも --memory などは渡せるが、WithResources は構造化された入力。
func WithResources(resources ResourceLimits) StartOption {
	return func(o *startOptions) {
		o.Resources = resources
	}
}

// WithWorkdir はコンテナの作業ディレクトリを上書きする。
// 影響: devcontainer.json の workspaceFolder より優先される。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithWorkdir("/work"))
//
// 類似: 設定ファイルの workspaceFolder は静的だが、WithWorkdir は実行時指定。
func WithWorkdir(path string) StartOption {
	return func(o *startOptions) {
		o.Workdir = path
	}
}

// WithNetwork は使用する Docker ネットワークを指定する。
// 影響: HostConfig.NetworkMode が設定され、既定のネットワーク解決が上書きされる。
// 例:
//
//	id, err := devcontainer.StartDevcontainer(ctx, devcontainer.WithNetwork("host"))
//
// 類似: WithRunArg の --network でも指定できるが、WithNetwork は明示 API。
func WithNetwork(network string) StartOption {
	return func(o *startOptions) {
		o.Network = network
	}
}
