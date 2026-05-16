# Handoff

## Current State

- 文書中心の初期設計フェーズ。
- CLI名は `run-weaver` に統一済み。
- Issue入口条件、状態ラベル、二重実行防止、state file、Doppler service tokenの方針を文書化済み。

## Next Step

`run-weaver doctor` と `run-weaver status` の出力形式を確定し、その後Go moduleとCLI skeletonを作成する。

## Notes

- pushは自動では行わない。
- secretやtokenの実値をログやドキュメントに書かない。
- stale `running` の自動奪取は初期実装では行わない。
