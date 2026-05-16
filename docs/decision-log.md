# Decision Log

## Accepted Decisions

- 2026-05-16: CLI名、製品名、tmux session名は `run-weaver` に統一する。
- 2026-05-16: 実装言語はGo、配布物は単一バイナリとする。
- 2026-05-16: WindowsとWSLは同一agentを `--target` で切り替える。
- 2026-05-16: GitHub Issueをタスク入口にする。
- 2026-05-16: 初期実装では `run-weaver:ready` ラベル付きopen Issueだけを処理対象にする。
- 2026-05-16: 状態ラベルは `running`、`done`、`blocked` の3つで、排他的に扱う。
- 2026-05-16: GitHub操作は初期実装では `gh` CLIを優先する。
- 2026-05-16: `status` はローカルstate fileを主情報源にし、GitHub Issue、process、tmuxを照合する。
- 2026-05-16: Codexへ渡すDoppler資格情報はDoppler service token `dev-agent` と呼ぶ。
- 2026-05-16: stale `running` は初期実装では自動奪取せず、人間確認に回す。

## Superseded Decisions

現時点ではなし。
