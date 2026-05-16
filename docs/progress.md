# Progress

## Current Work Queue

現在の優先タスクは、state file読み書きと `run-weaver status` の実装です。

Definition of Done:

- state fileの読み込み処理がある
- state file未作成時の `status` 表示と終了コードが決まっている
- `status --json` が文書化済みトップレベル構造で出力する
- process、tmux、GitHub照合の実装境界が分かれている
- 単体テストと `go test ./...` が通る

Recommended Next Step:

- state fileの型と読み込み処理を追加し、`status` のplaceholderを実データ読み込みへ置き換える。

## Completed

- CLI名を `run-weaver` に統一した
- Issue入口条件を `run-weaver:ready` ラベルとして明記した
- `running` / `done` / `blocked` の排他運用を明記した
- WSL/Windowsの二重実行防止方針を明記した
- `status` の主情報源としてローカルstate fileを明記した
- Doppler service token `dev-agent` の扱いを明記した
- AGENTS.mdの参照先ドキュメントを作成した
- `doctor` / `status` の人間向け出力例を追加した
- `doctor --json` を初期実装から提供する方針にした
- `status --json` のトップレベル構造を決めた
- state fileの最小JSON schemaを決めた
- WSL systemd user service確認、claim競合検証、Codex CLI非対話起動仕様を明記した
- Go moduleを作成した
- 標準ライブラリだけでCLI skeletonを作成した
- `--target` の基本validationを追加した
- `doctor` / `status` / `daemon` / `install` のサブコマンド枠を追加した
- CLI skeletonの単体テスト、`go test ./...`、`go build ./cmd/run-weaver` を確認した
- `doctor --target wsl` の依存関係、認証、WSL固有条件checkを実装した
- `doctor --target windows` のOS targetとTask Scheduler確認枠を実装した
- `doctor --json` を構造化出力にした
- `doctor` の終了コードを `docs/cli.md` の方針に合わせた

## Upcoming Sequence

1. GitHub Issue取得とclaim処理を実装する
2. WSL targetのtmux runnerを実装する
3. draft PR作成までのdaemon flowを実装する

## Open Decisions To Watch

- `run-weaver:ready` 以外のフィルタ。assignee、milestone、repository allowlistなど
- stale `running` の解除を将来も人間判断に固定するか、自動解除へ拡張するか
- Windows targetのログ保存場所
- GitHub API直実装へ移行する時期
