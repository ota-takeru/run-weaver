# Progress

## Current Work Queue

現在の優先タスクは、`run-weaver doctor` と `run-weaver status` の出力形式を確定することです。

Definition of Done:

- `doctor` の人間向け出力例がある
- `doctor --json` を作るかどうかが決まっている
- `status` の人間向け出力例がある
- `status --json` のトップレベル構造が決まっている
- state fileの最小JSON schemaが決まっている
- 終了コードとエラー表示方針が `docs/cli.md` に反映されている

Recommended Next Step:

- `docs/cli.md` に `doctor` / `status` の出力例とstate file schemaを追加する。

## Completed

- CLI名を `run-weaver` に統一した
- Issue入口条件を `run-weaver:ready` ラベルとして明記した
- `running` / `done` / `blocked` の排他運用を明記した
- WSL/Windowsの二重実行防止方針を明記した
- `status` の主情報源としてローカルstate fileを明記した
- Doppler service token `dev-agent` の扱いを明記した
- AGENTS.mdの参照先ドキュメントを作成した

## Upcoming Sequence

1. Go moduleを作成する
2. CLI skeletonを作る
3. `doctor` を実装する
4. state file読み書きと `status` を実装する
5. GitHub Issue取得とclaim処理を実装する
6. WSL targetのtmux runnerを実装する
7. draft PR作成までのdaemon flowを実装する

## Open Decisions To Watch

- state fileのJSON schema詳細
- `run-weaver:ready` 以外のフィルタ。assignee、milestone、repository allowlistなど
- stale `running` の解除を自動化するか、人間判断に固定するか
- Windows targetのログ保存場所
- GitHub API直実装へ移行する時期
