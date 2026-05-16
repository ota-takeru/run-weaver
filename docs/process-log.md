# Process Log

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
