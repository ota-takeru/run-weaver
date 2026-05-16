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
- WSL tmux runner、`run-weaver` session作成、`issue-<number>` window作成、`codex exec` コマンド組み立てを実装済み。
- worktree manager、prompt file生成、draft PR作成wrapperを実装済み。
- `daemon --once` でready Issue取得、claim、worktree作成、prompt生成、tmux起動、state保存まで接続済み。
- Codex完了検出、git push、draft PR作成、`done` ラベル、結果コメント、state更新を実装済み。

## Next Step

daemon loop化と失敗時blocked処理を強化する。

## Notes

- pushは自動では行わない。
- secretやtokenの実値をログやドキュメントに書かない。
- stale `running` の自動奪取は初期実装では行わない。
- 初期実装のCodex起動は `codex exec` を使う。
- 現時点では継続poll loop、失敗時のstate file更新強化、status表示の細分化は未実装。
- Codex完了判定はlast message fileの存在とtmux window終了を最小条件にしている。
- state fileがない状態の `status` は終了コード1で、JSON/human出力は返す。
- Windows targetのdoctor checkはWindows実機で追加検証が必要。
