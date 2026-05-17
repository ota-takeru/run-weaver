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
- 2026-05-16: `doctor --json` と `status --json` は初期実装から提供する。
- 2026-05-16: GitHub Issueのclaim成功は、`running` ラベル付与だけでなく、claim ID付き開始コメントの再取得で判定する。
- 2026-05-16: Codex CLIは初期実装では非対話モードの `codex exec` で起動する。
- 2026-05-17: Windows targetの常駐方式はper-user Task Schedulerとし、state / logs / issue logsは `%LOCALAPPDATA%\run-weaver` 配下に保存する。
- 2026-05-17: Campaign Issueは `run-weaver:campaign` と `run-weaver:ready` の両方を入口ラベルにし、Plannerが子Issueへtaskを実体化する。taskごとにdraft PRを作成し、Campaign progressはstate fileの `campaign` に保存する。
- 2026-05-17: 複数repository運用は `run-weaver repo add` で登録したrepo一覧を使い、repo別state / clone / worktreeで分離し、repo間は並列実行する。
- 2026-05-17: Dopplerはrepositoryごとの任意依存とし、`dopplerMode` が `auto` の場合はrepo rootのDoppler設定ファイルで必須性を判定する。Doppler必須repositoryだけCodex起動を `doppler run --` 経由にし、不要repositoryではDoppler未インストールでも続行する。

## Superseded Decisions

現時点ではなし。
