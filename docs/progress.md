# Progress

## Current Work Queue

現在の優先タスクは、Windows targetのdoctor / status追加検証です。

Definition of Done:

- Windows実機またはWindows相当環境で `run-weaver doctor --target windows` を実行する
- Windows targetのstatus表示でstate file、process、GitHub照合の挙動を確認する
- Windows固有の未実装または環境依存事項をdocsに記録する
- 変更があれば単体テストと `go test ./...` が通る

Recommended Next Step:

- Windows targetを実行できる環境を用意して、doctor / statusの実機挙動を確認する。

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
- worktree manager、prompt file生成、draft PR作成wrapperを追加した
- `daemon --once` でready Issue取得、claim、worktree作成、prompt作成、tmux起動、state保存まで接続した
- `daemon --once` の `--repo-url` と `--repo` の仕様を `docs/cli.md` に追加した
- Codex完了の最小条件としてlast message fileの存在とtmux window終了を使うようにした
- 完了済みjobでgit push、draft PR作成、`done` ラベル、結果コメント、state更新を行う処理を追加した
- draft PR作成flowの単体テストを追加した
- `daemon` の継続poll loopと `--poll-interval` を追加した
- claim後の失敗時に `blocked` ラベル、blockedコメント、state fileのblocked状態を残す処理を追加した
- `status --repo` でGitHub管理ラベルを照合し、state fileと矛盾する場合にconflictを出すようにした
- 実GitHub Issueで試す前のWSL統合テスト手順を `docs/handoff.md` に追加した
- README、`docs/architecture.md`、`docs/cli.md`、`docs/github-issue-flow.md` の実装済み仕様を確認した
- `run-weaver doctor --target wsl --json`、`status --json`、`daemon -h` 相当の基本動作を手元で確認した
- 実GitHub Issue `ota-takeru/truth-table-app#1` でWSL統合テストを行い、本文付きIssueからCodex実行、README追加、commit、branch push、draft PR #2作成、`done` ラベル更新まで確認した
- Issue本文をCodex promptへ渡し、本文なしでもタイトルが具体的なら実行できるようにした

## Upcoming Sequence

1. Windows targetのdoctor / status追加検証

## Open Decisions To Watch

- `run-weaver:ready` 以外のフィルタ。assignee、milestone、repository allowlistなど
- stale `running` の解除を将来も人間判断に固定するか、自動解除へ拡張するか
- Windows targetのログ保存場所
- GitHub API直実装へ移行する時期
