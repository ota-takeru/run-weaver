# Decision Prep Standard

重要判断をユーザーへ求める前に、人間が判断できるだけの客観的な意思決定レポートを作成する。レポートは短さより判断材料の不足がないことを優先し、事実、推論、推奨を分ける。

## Required Sections

- Decision: 決めたいこと。回答が必要な選択肢名も明示する。
- Trigger: なぜagentが自律継続できないか。AGENTS.md、ADR、secret、外部アカウント、コスト、不可逆操作などの該当条件を示す。
- Context: 現在の制約、既存決定、関連ドキュメント、対象Issue / PR / branch / state file。
- Evidence: 確認済みの客観事実。参照ファイル、Issue番号、PR番号、コマンド結果の要約、ログpathを含める。secret、token、環境変数値、ログ本文の丸写しは含めない。
- Options: 2から3個の現実的な選択肢。各選択肢について実行内容、得られる結果、主なリスク、ブロックされる作業を説明する。
- Recommendation: 推奨案と理由。推奨は事実ではなく判断として書く。
- Impact: 実装、運用、セキュリティ、移行、コスト、スケジュール、既存ユーザーへの影響。
- Reversibility: 後から戻せるか。戻す場合の作業、失われるもの、migrationや外部操作の有無。
- Required Human Action: 人間が返すべき具体的な回答形式、実行すべきコマンド、または外部画面で必要な操作。

## Objectivity Rules

- 観測済み事実と推測を混ぜない。推測は「推測」または「未確認」と明記する。
- 選択肢は推奨案だけを厚くしない。採用しない選択肢にも実利とリスクを書く。
- 判断に不要なsecret値、token値、個人情報、private log本文は出さない。必要ならローカルpathや確認方法だけを書く。
- 既存のaccepted decision / ADRと矛盾する案は、その矛盾と必要なdecision log / ADR更新を明記する。
- 「何を選べばよいか」だけでなく、「選ばないと何が止まるか」を書く。

## Current Process Audit

2026-05-18時点のプロセスは、人間確認が必要な条件と `docs/decision-prep-standard.md` の存在は定義済みであり、CampaignではDecision Requestコメントで停止と再開も実装済み。ただし、従来の標準は「短くまとめる」前提で、Decision Requestも `options`、`recommendation`、`blocked tasks`、`can continue tasks` だけだったため、以下の不足があった。

- なぜ人間判断が必要かを示すtriggerが明示されず、自律継続できない理由を後から検証しにくい。
- 証拠、参照ドキュメント、Issue / PR / stateの客観情報が必須でなく、推奨案の根拠が薄くなり得る。
- optionごとの影響、リスク、可逆性が必須でなく、短期判断と長期運用コストを比較しにくい。
- secretやログ本文を避ける方針は他文書にあるが、意思決定レポートの標準内では明示が弱かった。

このため、以後は本標準のRequired Sectionsを満たすレポートを作ってから人間に判断を求める。Campaign Planner由来のDecision Requestでも、context、evidence、option details、impact、reversibilityを必須にする。

## Important Decisions

以下は重要判断として扱う。

- CLI名や製品名の変更
- Issue入口条件、ラベル仕様、claim方式の変更
- state fileの正本性や保存場所の変更
- Doppler secret/token運用の変更
- WSL/Windowsの常駐方式の変更
- GitHub連携を `gh` CLIからAPI直実装へ移す変更
- dashboardをMVP範囲へ入れる変更
