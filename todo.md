# TODO: devcontainer-spec の未実装項目（タスク形式）

作成日: 2026-02-08

このファイルは devcontainer 起動実装で現時点で未対応の仕様項目をタスク形式で列挙したものです。各タスクは小さく分割し、テスト・実装・レビューがしやすい単位にしています。

各タスクは次の形式で記載しています。
- タイトル
- 説明（なぜ必要か）
- 受け入れ条件（完了を判定する基準）

---

- [x] Docker Compose サポート
  - 説明: devcontainer.json の compose/dockerComposeFile/runServices 等を解釈し、Compose ベースのサービス群を起動・管理する機能を追加する。
  - 進捗: 起動/primary service 解決/override まで対応済み。Compose V2 のみ、docker compose CLI を使用。compose build/down と E2E クリーンアップ検証は未対応。
  - 受け入れ条件: 単体サービス定義と multi-service compose ファイルの両方で Build→Start→Stop→Remove が動作し、テスト実行後に image/container/volume が残らないことを自動検証できること。

- [x] ライフサイクルスクリプトの実行
  - 説明: initialize/onCreate/postCreate/postStart/postAttach 等のスクリプトを正しい順序で実行し、失敗時のロールバックやログ収集を行う。
  - 受け入れ条件: 各イベントハンドラが実行されることを示す E2E テストが存在し、失敗時にエラーが適切に返されること。

- [ ] Features の解決とインストール (ORAS 等を経由)
  - 説明: containers.dev の feature を ORAS 等で取得し、コンテナ内に適用するロジックを実装する。外部レジストリや認証の扱いも含む。
  - 受け入れ条件: 少なくとも 1 つの feature を ORAS 経由で取得してコンテナに適用できる E2E テストが存在すること。

- [ ] build.options の全面サポート
  - 説明: devcontainer.json の build.options (buildKit オプション、args、cacheFrom など) を充分に解釈して ImageBuild に正しく反映する。
  - 受け入れ条件: build.options を指定した際に期待通りのビルド結果となる統合テストが存在すること。

- [ ] portsAttributes / advanced port mapping のサポート
  - 説明: devcontainer.schema にある portsAttributes（プロトコル、説明、優先度など）や複雑なポート指定の解釈を追加する。
  - 受け入れ条件: portsAttributes を含む設定で正しいポート公開とメタデータ検出が行えることを示すテストがあること。

- [ ] Run Args / containerUser / forwardPorts の微妙な挙動差分の検証と補完
  - 説明: runArgs や containerUser、forwardPorts に関する edge case を仕様どおりに処理するための追加処理とテスト。
  - 受け入れ条件: 仕様に沿った挙動を保証するユニット／統合テストが追加されていること。

- [ ] E2E 統合テスト基盤の整備と CI 組み込み
  - 説明: Build→Create→Start→Stop→Remove を安全に実行する E2E テスト群と、それを CI（Docker-in-Docker など）で実行するパイプラインを整備する。
  - 受け入れ条件: CI 上で E2E が実行でき、イメージ/コンテナ/ボリュームが残らないことを自動検証できること。

- [ ] ドキュメントと README 更新
  - 説明: 新しい CLI/API の使い方、既知の制約（ローカル Docker デーモン必須など）、テスト手順を README にまとめる。
  - 受け入れ条件: README に起動手順とテスト手順が記載されていること。

---
