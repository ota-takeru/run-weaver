# Progress

## Current Work Queue

現在の優先タスクは、GitHub Releases配布と起動時self-updateの検証です。

Definition of Done:

- release buildで `internal/cli.Version` にtag名を埋め込み、GitHub Release assetを作成できる
- `daemon` 起動時にlatest releaseを確認し、newer versionがある場合だけ対応assetを取得する
- 開発ビルドでは自動更新せず、`RUN_WEAVER_NO_UPDATE=1` でrelease buildでも一時停止できる
- ローカルproject cloneなしで `scripts/install.sh` / `scripts/install.ps1` からGitHub Releasesのbinaryを導入できる
- 更新処理とinstall手順をdocsに記録する

Recommended Next Step:

- GitHub ActionsのLinux / Windows CIと、tag release workflowの結果を確認する。

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
- Windows targetのstatus process照合を `tasklist` ベースにし、CSV解析の単体テストを追加した
- Windows targetのdoctor / status検証用にOS判定、LookPath、外部コマンド出力をテスト注入可能にした
- Windows targetのdoctor、Task Scheduler、state file path、status process、tmux不使用、fake `gh` 照合の単体テストを追加した
- Linux / WindowsのGitHub Actions CIと任意の手元確認用 `scripts/check-windows.ps1` を追加した
- Windows targetの常駐方式をper-user Task Scheduler、ログ保存場所を `%LOCALAPPDATA%\run-weaver\logs\daemon.log` としてaccepted decisionにした
- `run-weaver install --target windows --repo-url <url>` でper-user Task Scheduler taskを作成する処理を追加した
- `doctor --target windows` がlogs directoryの書き込み可否を確認するようにした
- GitHub ActionsのLinux / Windows jobが最新コミット `847e6d3` で成功した
- `status` のJSONとhuman outputに `runtimeState` を追加し、`codex_running`、`codex_completed`、`needs_attention` を区別するようにした
- Codexがrate limitで中断した場合に、同じworktreeと前回sessionから `codex exec resume` で自動再開する処理を追加した
- READMEとCLI docsにインストール手順と、install後に使うための前提条件を追記した
- release buildの `daemon` 起動時にGitHub Releases latestを確認してself-updateする処理を追加した
- `run-weaver update --check` / `run-weaver update` を追加した
- GitHub Release asset作成workflowと、project clone不要の `scripts/install.sh` / `scripts/install.ps1` を追加した
- `run-weaver install --target wsl --repo-url <url>` でsystemd user serviceを作成または更新する処理を追加した
- `install` / `daemon` の `--repo-url` 未指定時に、カレントディレクトリの `git remote get-url origin` から対象repository URLを自動推定するようにした

## Upcoming Sequence

1. self-updateとclone不要install手順のCI / release workflow結果確認
2. `run-weaver:ready` 以外のフィルタ検討

## Open Decisions To Watch

- `run-weaver:ready` 以外のフィルタ。assignee、milestone、repository allowlistなど
- stale `running` の解除を将来も人間判断に固定するか、自動解除へ拡張するか
- GitHub API直実装へ移行する時期
