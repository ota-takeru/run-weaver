# Progress

## Current Work Queue

現在の優先タスクは、draft PR作成までのdaemon flow実装です。

Definition of Done:

- ready Issueのclaim後にworktreeを作る
- prompt file、Codex log path、last message pathを準備する
- WSL tmux runnerでCodexを起動する
- 成功時にdraft PRを作成し `done` に更新する処理枠がある
- 失敗時に `blocked` に更新する処理枠がある
- 単体テストと `go test ./...` が通る

Recommended Next Step:

- worktree manager、prompt作成、draft PR作成用の `gh` wrapperを追加し、daemonの一回分処理へ接続する。

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
- state fileの型と読み書き処理を追加した
- `status` がstate fileを読み込むようにした
- state file未作成時の `status --json` は構造化出力と終了コード1を返すようにした
- `status` のprocess、tmux、GitHub照合境界を分けた
- `gh` CLI wrapperを追加した
- `run-weaver:ready` ラベル付きopen Issue取得と管理ラベル除外処理を追加した
- claim ID付き開始コメント投稿と、最新claimコメント再取得による勝敗判定を追加した
- claim競合負けではstate fileを更新しない方針をコード上の境界として実装した
- tmux session `run-weaver` の作成または再利用処理を追加した
- Issueごとのtmux window名 `issue-<number>` を作る処理を追加した
- `codex exec --json --cd <worktree>` の起動コマンドを組み立てる処理を追加した
- JSONLログと最終応答パスをstate配下に向ける処理を追加した
- tmux runnerの単体テストを追加した

## Upcoming Sequence

現時点ではなし。Current Work Queueを継続する。

## Open Decisions To Watch

- `run-weaver:ready` 以外のフィルタ。assignee、milestone、repository allowlistなど
- stale `running` の解除を将来も人間判断に固定するか、自動解除へ拡張するか
- Windows targetのログ保存場所
- GitHub API直実装へ移行する時期
