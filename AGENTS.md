# godev2: devcontainer implementation in go

## 目標
- シングルバイナリ
- 外部依存がdocker以外なし
- cgoを使わない

## 要求条件
- テストカバレッジ60%以上
- テストのクリーンアップ
  - image,container,volumeについては、必ず削除すること
  - 必ず、追加したテスト関数をひとつづつ実行し、image,container,volumeが増えていないことを確認すること
- golangci-lintに関して、必ず実行し、すべてのエラーを修正すること
- go fmtについて、すべての作業が終了した後、必ず行うこと

## 仕様について
- devcontainer-specにおいてあるので、必ず調査し、参照すること

## 依存推奨ライブラリについて
詳細は必ずcontext7で調査すること
- oras-go (oci artifact distributionによるfeature/template機能)

## 実装メモ
- devcontainer.json の features を OCI/HTTPS/ローカル参照で解決し、install.sh を build 時に実行する。
- Feature の lifecycle コマンドはインストール順で実行され、ユーザーの lifecycle コマンドより先に実行される。
- docker compose 使用時は features を未サポートとしてエラーにする。

## テスト
- go test ./...
- golangci-lint run
- Docker デーモンが必要
