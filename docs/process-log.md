# Process Log

## 2026-05-16 - GitHub Issue Claim

Review:

- Immediate fixes:
  - `gh` CLI wrapperを追加した。
  - `run-weaver:ready` ラベル付きopen Issue取得を追加した。
  - `running` / `done` / `blocked` ラベル付きIssueの除外処理を追加した。
  - claim ID付き開始コメント本文と投稿処理を追加した。
  - 開始コメント再取得で最新claim IDを確認し、claim勝敗を判定する処理を追加した。
  - claim競合に負けた場合はstate file更新側へ進まない結果を返すようにした。
  - claim勝ち、claim負け、管理ラベルskipの単体テストを追加した。
- Future tasks:
  - tmux runnerとworktree作成を実装してから、daemon flowへclaim処理を接続する。
  - GitHub照合を `status` のreconciliationへ接続する。
  - `gh` CLIの実リポジトリ操作は統合テスト方針を決めてから追加検証する。
- Human-facing reports:
  - runner未実装のままIssueを `running` にすると放置リスクがあるため、daemonからの実claim接続はまだ行っていない。

## 2026-05-16 - State File and Status

Review:

- Immediate fixes:
  - state file schemaに対応するGo型を追加した。
  - state fileのread/write処理を追加した。
  - `status` が既定state fileを読み込み、human / JSONで表示するようにした。
  - state file未作成時は構造化出力を返し、終了コード1にした。
  - daemon process照合、WSL tmux照合、GitHub照合の境界を分けた。GitHub照合はIssue処理実装まで `not_checked` とする。
  - `status` の単体テストを追加した。
- Future tasks:
  - GitHub Issue取得とclaim処理を実装する。
  - GitHub照合を `status` のreconciliationへ接続する。
  - Windows targetのprocess確認はWindows実機で追加検証する。
- Human-facing reports:
  - state fileがない状態での `status` は異常終了ではなく、状態未作成を示す終了コード1として扱う。

## 2026-05-16 - Go Module and CLI Skeleton

Review:

- Immediate fixes:
  - `go.mod` を追加した。
  - `cmd/run-weaver` にCLI entrypointを追加した。
  - `internal/cli` に `doctor` / `status` / `install` / `daemon` のサブコマンド枠を追加した。
  - `doctor` / `install` / `daemon` で `--target` 必須validationを追加した。
  - `wsl` / `windows` 以外のtargetをusage errorにした。
  - `status --json` と `doctor --json` の最小placeholder出力を追加した。
  - root build artifactを誤コミットしないよう `.gitignore` を追加した。
- Future tasks:
  - `doctor` の実checkを実装する。
  - `status` でstate file読み込みとreconciliationを実装する。
  - CLI usageの文面を `docs/cli.md` の終了コード方針にさらに寄せる。
- Human-facing reports:
  - 現時点のCLIはskeletonであり、実際の依存関係検査やdaemon処理は未実装。

## 2026-05-16 - Doctor Implementation

Review:

- Immediate fixes:
  - `doctor --target wsl` でOS target、`git`、`gh auth`、`codex`、`doppler`、`tmux`、`systemctl --user`、Codex専用clone、worktree root、state file、systemd user serviceを確認するようにした。
  - `doctor --target windows` でWindows target確認とTask Scheduler確認枠を追加した。
  - `doctor --json` を `encoding/json` による構造化出力にした。
  - `doctor` の終了コードをtarget mismatch、missing、auth_required、config不足に分類した。
  - `doctor` の単体テストを実環境の依存有無に左右されにくい形に更新した。
- Future tasks:
  - `status` のstate file読み込みとreconciliationを実装する。
  - `doctor` のWindows targetはWindows実機で追加検証する。
  - Doppler checkの失敗理由を将来もう少し細分化する。
- Human-facing reports:
  - 現在の環境で `doctor` を実行すると、Codex専用cloneやservice未作成ならblockedになる想定。

## 2026-05-16 - CLI Output and State Schema

Review:

- Immediate fixes:
  - `doctor` の人間向け出力例と `doctor --json` の出力例を追加した。
  - `doctor --json` は初期実装から提供する方針にした。
  - `status` の人間向け出力例と `status --json` のトップレベル構造を追加した。
  - state fileの最小JSON schemaを追加した。
  - WSL targetの `systemctl --user` / systemd user service確認を `doctor` の確認項目に追加した。
  - GitHubラベル更新がCASではない前提で、claim ID付き開始コメントと再取得による勝敗判定を明記した。
  - Codex CLIは初期実装で `codex exec` を使い、JSONLログと最終応答をstate配下に保存する方針を明記した。
- Future tasks:
  - Go moduleとCLI skeletonを作成する。
  - `doctor` の各checkを実装し、WSL systemd user managerが使えない場合のblocked表示をテストする。
  - GitHub Issue取得とclaim処理の実装時に、開始コメント競合のテストを追加する。
- Human-facing reports:
  - stale `running` の自動奪取は引き続き初期実装の対象外。将来拡張する場合は運用リスクを再評価する。

## 2026-05-16 - Documentation Review

Review:

- Immediate fixes:
  - CLI名を `run-weaver` に統一した。
  - Issue入口条件として `run-weaver:ready` ラベルを追加した。
  - WSL/Windowsの二重実行防止方針を追加した。
  - `status` の主情報源としてローカルstate fileを追加した。
  - Dopplerの用語を service token `dev-agent` に統一した。
  - AGENTS.mdが参照する運用ドキュメントを追加した。
- Future tasks:
  - state fileのJSON schemaを実装前に確定する。
  - `doctor` と `status` の具体的な出力例を追加する。
  - Windows targetのログ保存先を決める。
- Human-facing reports:
  - stale `running` を自動解除するかは運用リスクがあるため、初期方針では自動奪取せず人間確認に回す。
