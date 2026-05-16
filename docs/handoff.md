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
- `status` のprocess、tmux、GitHub照合境界を分けた。GitHub照合は未接続。
- `gh` CLI wrapper、ready Issue取得、管理ラベル除外、claim ID付き開始コメント、claim勝敗判定を実装済み。

## Next Step

WSL targetのtmux runnerを実装する。

## Notes

- pushは自動では行わない。
- secretやtokenの実値をログやドキュメントに書かない。
- stale `running` の自動奪取は初期実装では行わない。
- 初期実装のCodex起動は `codex exec` を使う。
- 現時点ではtmux runner、worktree作成、daemon flowは未実装。
- GitHub claim処理は実装済みだが、runner未実装のためdaemon flowにはまだ接続していない。
- state fileがない状態の `status` は終了コード1で、JSON/human出力は返す。
- Windows targetのdoctor checkはWindows実機で追加検証が必要。
