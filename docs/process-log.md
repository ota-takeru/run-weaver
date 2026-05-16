# Process Log

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
