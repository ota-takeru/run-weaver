# Decision Prep Standard

重要判断をユーザーへ求める前に、以下を短くまとめる。

## Required Sections

- Decision: 決めたいこと
- Context: 現在の制約、既存決定、関連ドキュメント
- Options: 2から3個の現実的な選択肢
- Recommendation: 推奨案と理由
- Impact: 実装、運用、セキュリティ、移行への影響
- Reversibility: 後から戻せるか

## Important Decisions

以下は重要判断として扱う。

- CLI名や製品名の変更
- Issue入口条件、ラベル仕様、claim方式の変更
- state fileの正本性や保存場所の変更
- Doppler secret/token運用の変更
- WSL/Windowsの常駐方式の変更
- GitHub連携を `gh` CLIからAPI直実装へ移す変更
- dashboardをMVP範囲へ入れる変更
