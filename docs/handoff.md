# Handoff

## Current State

- Go実装の初期skeletonフェーズ。
- CLI名は `run-weaver` に統一済み。
- Issue入口条件、状態ラベル、二重実行防止、state file、Doppler service tokenの方針を文書化済み。
- `doctor` / `status` の人間向け出力例とJSON出力例を文書化済み。
- state fileの最小JSON schemaを文書化済み。
- WSL systemd user service確認、claim競合検証、Codex CLI非対話起動仕様を文書化済み。
- Go moduleとCLI skeletonを作成済み。
- `doctor` / `status` / `install` / `daemon` のサブコマンド枠と基本target validationを実装済み。
- `doctor --target wsl` の実checkと `doctor --json` の構造化出力を実装済み。
- `doctor --target windows` のWindows target確認とTask Scheduler確認枠を実装済み。
- state fileの型とread/write処理を実装済み。
- `status` がstate fileを読み込み、human / JSONで表示するようになった。
- `status` のprocess、tmux、GitHub照合を実装済み。`--repo` で `gh` CLIへowner/repoを渡せる。
- `gh` CLI wrapper、ready Issue取得、管理ラベル除外、claim ID付き開始コメント、claim勝敗判定を実装済み。
- WSL tmux runner、`run-weaver` session作成、`issue-<number>` window作成、`codex exec` コマンド組み立てを実装済み。
- worktree manager、prompt file生成、draft PR作成wrapperを実装済み。
- `daemon --once` でready Issue取得、claim、worktree作成、prompt生成、tmux起動、state保存まで接続済み。
- Codex完了検出、git push、draft PR作成、`done` ラベル、結果コメント、state更新を実装済み。
- `daemon` の継続poll loop、`--poll-interval`、claim後失敗時のblocked state file更新を実装済み。

## Next Step

実装済みCLI仕様とドキュメントの整合確認を行い、実GitHub Issueを使ったWSL統合テスト手順を固める。

## Notes

- pushは自動では行わない。
- secretやtokenの実値をログやドキュメントに書かない。
- stale `running` の自動奪取は初期実装では行わない。
- 初期実装のCodex起動は `codex exec` を使う。
- 現時点ではstatus表示の細分化、GitHub照合のstatus接続、実GitHub Issueでの統合検証は未実施。
- Codex完了判定はlast message fileの存在とtmux window終了を最小条件にしている。
- `daemon` はGitHub Issueのラベルとコメントを実際に変更する。実行前に対象repository、ready Issue、`--repo-url` を確認する。
- state fileがない状態の `status` は終了コード1で、JSON/human出力は返す。
- Windows targetのdoctor checkはWindows実機で追加検証が必要。

## WSL Integration Test Prep

実GitHub Issueで試す前に、以下を確認する。

1. `gh auth status` が対象repositoryへread/writeできる。
2. 対象repositoryにopen Issueを1つ用意し、`run-weaver:ready` を付ける。
3. 対象Issueに `running` / `done` / `blocked` が付いていないことを確認する。
4. `go build ./cmd/run-weaver` でローカルバイナリを作る。
5. `./run-weaver doctor --target wsl` で `git`、`gh`、`codex`、`doppler`、`tmux`、`systemctl --user` を確認する。
6. 初回は `./run-weaver daemon --target wsl --once --repo <owner/repo> --repo-url <repo-url>` だけを使い、継続pollは使わない。
7. 実行後に `./run-weaver status --repo <owner/repo>`、GitHub Issueコメント、tmux windowを確認する。

この手順はIssueラベル、Issueコメント、branch、draft PRを実際に作る。対象Issueを間違えた場合の自動巻き戻しはない。
