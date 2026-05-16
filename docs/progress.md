# Progress

## Current Work Queue

現在の優先タスクは、Go moduleを作成し、CLI skeletonの実装に着手することです。

Definition of Done:

- `go.mod` が作成されている
- `run-weaver` CLIのentrypointがある
- `--target` の基本validationがある
- `doctor` / `status` / `daemon` / `install` のサブコマンド枠がある
- 最小の単体テストまたはビルド確認が通る

Recommended Next Step:

- Go moduleとCLI skeletonを作成する。

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

## Upcoming Sequence

1. CLI skeletonを作る
2. `doctor` を実装する
3. state file読み書きと `status` を実装する
4. GitHub Issue取得とclaim処理を実装する
5. WSL targetのtmux runnerを実装する
6. draft PR作成までのdaemon flowを実装する

## Open Decisions To Watch

- `run-weaver:ready` 以外のフィルタ。assignee、milestone、repository allowlistなど
- stale `running` の解除を将来も人間判断に固定するか、自動解除へ拡張するか
- Windows targetのログ保存場所
- GitHub API直実装へ移行する時期
