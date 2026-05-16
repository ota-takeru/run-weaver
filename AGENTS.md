
# AGENTS.md

このリポジトリで作業するコーディングエージェントは、回答・報告を日本語で行う。

## 基本姿勢

- 逐次確認型ではなく、自律実行型で進める。
- ユーザーに確認するのは、大きな意思決定、不可逆または高コストな変更、秘密情報・外部アカウント・支払い・権限操作など人間の関与が必要な場合に限る。
- 通常の実装、調査、テスト、軽微な設計判断、ドキュメント更新、次に明らかなタスクへの着手は、エージェントが妥当な仮定を置いて進める。
- 仮定を置いた場合は、最終報告または関連ドキュメントにその仮定を明記する。

## プロジェクト固有ルール

- 製品名、CLI名、tmux session名は `run-weaver` で統一する。旧名称は新規追加しない。
- このプロジェクトは、GitHub Issueを入口にCodex CLIをworktree上で動かすGo製agentを作る。
- 人間用 `src` は直接触らず、Codex専用cloneとIssue専用worktreeで作業する方針を維持する。
- 初期実装の対象Issueは、同一repository内のopen Issueかつ `run-weaver:ready` ラベル付きに限定する。
- agent管理ラベルは `running`、`done`、`blocked` の3つで、排他的に扱う。
- WSL/Windowsの二重実行防止は、GitHub上の `running` ラベル、開始コメント、ローカルstate fileのclaimを使う。
- `run-weaver status` はローカルstate fileを主情報源にし、process、tmux、GitHub Issueを照合する設計にする。
- Dopplerについては、Codexへ渡すものをDoppler service token `dev-agent` と呼ぶ。tokenやsecretの値をログ、Issueコメント、PR本文、ドキュメント例に書かない。
- GitHub操作は初期実装では `gh` CLI優先とする。GitHub API直実装へ変更する場合はdecision logまたはADRに残す。
- dashboardは将来拡張であり、初期実装の必須範囲に含めない。

## 最初に読む順序

1. `AGENTS.md`: このリポジトリでの最上位の作業規約。
2. `docs/progress.md` の `Current Work Queue`: 現在のタスク、Definition of Done、作業範囲。
3. `docs/decision-log.md` と `docs/adr/`: 既に受け入れられた重要判断。
4. 必要に応じて `docs/process-log.md`、設計メモ、README。

長い履歴や古い設計メモより、現在の作業キューと accepted decision / ADR を優先する。

## 自律的な進行ルール

- `docs/progress.md` の `Current Work Queue`、Recommended Next Step、Definition of Done を現在の作業キューとして扱う。
- 1つのタスクが完了したら、次のタスクへ進む前に、必ず別のエージェントの視点からレビューを行う。実際にサブエージェントを使える場合は使い、使えない場合は同じエージェントがレビュー観点を切り替えてセルフレビューする。
- サブエージェントが利用可能で、かつ実行環境の上位指示が許可している場合は、カスタムサブエージェントではなく Codex の built-in subagents を優先して使う。調査・読み取り中心は `explorer`、実装・修正レビューは `worker`、汎用フォールバックは `default` を使う。
- レビュー結果は、以下のいずれかに分類して処理する。
  - すぐ修正する: バグ、回帰、設計意図との不整合、品質チェック失敗、テスト不足で現在の変更に閉じているもの。
  - 将来タスクに追加する: 現在の Definition of Done を超える改善、スコープ拡張、後続機能で扱うべき設計課題。
  - 人間にレポートする: 重要な意思決定、コスト・運用・外部サービス契約・秘密情報・プロダクト方針に関わるもの。
- レビューで「すぐ修正する」と判断した項目は、可能な限り同じ作業サイクル内で修正し、再度テストする。
- レビューで「将来タスク」と判断した項目は、`docs/progress.md` の Upcoming Sequence、Open Decisions To Watch、または Definition of Done に反映する。
- レビュー結果は `docs/process-log.md` に `Review` として記録し、少なくとも `Immediate fixes`、`Future tasks`、`Human-facing reports` の3分類を残す。
- レビュー、必要な修正、分類、`docs/handoff.md` を含むドキュメント更新を終えてから、次の明確なタスクへ進む。
- Codex Stop hook が有効かつ信頼済みの場合、タスク完了後に Codex は継続可否を判断し、人間判断・秘密情報・外部アカウント・支払い・不可逆操作・管理者権限操作・ADR矛盾が不要なら、handoff 更新後に次タスクへ進む。
- `docs/progress.md` の `Current Work Queue`、Definition of Done、Recommended Next Step が空または曖昧な場合、Stop hook による自律継続は止め、作業キューの明確化を優先する。
- レートリミット解除後の自動再開は、人間確認条件を迂回しない。push、deploy、外部課金、外部アカウント設定変更、秘密情報の表示・送信、破壊的操作、AGENTS.md で確認必須の操作は自動実行しない。

## 確認が必要な条件

以下に該当する場合は、実装を止めてユーザーに判断を求める。

- アーキテクチャ、データモデル、クラウド移行、LLM/エージェント基盤、データソース、MVPスコープに関わる重要判断。
- 既存の accepted decision または ADR と矛盾する変更。
- データ破壊、履歴改変、秘密情報の永続化、外部課金、外部アカウント設定、権限変更が必要な操作。
- ユーザーしか実行できないローカル操作、認証、契約、支払い、管理者権限操作。
- 複数の合理的な選択肢があり、選択によって将来のコストや運用負荷が大きく変わる場合。

重要判断を求める前には、`docs/decision-prep-standard.md` に従って意思決定レポートを作成する。

## 品質と検証

- コード変更後は、変更範囲に応じてテストと品質チェックを実行する。
- ドキュメントのみの変更では、少なくとも関連語句の検索、リンク先ファイルの存在確認、表記揺れ確認を行う。
- `run-weaver` から別名称へ変わる変更、Issue入口条件、ラベル仕様、state file仕様、secret運用、常駐方式を変える変更は重要判断として扱う。

## ドキュメント整合性

- 未決定事項を書く場合は、`docs/decision-log.md`、`docs/progress.md`、設計メモの間で解決済みの項目を未決定として残さない。
- `docs/progress.md` は現在状態と次タスクの正本、`docs/process-log.md` は時系列ログ、`docs/decision-log.md` と `docs/adr/` は確定判断の正本として使い分ける。
- 設計メモは検討材料として扱い、accepted decision / ADR と矛盾する場合は accepted decision / ADR を優先して、必要なら設計メモを更新する。
- README、`docs/architecture.md`、`docs/cli.md`、`docs/github-issue-flow.md` の同じ仕様は同時に更新する。
- AGENTS.mdが参照する運用ドキュメントを増減させる場合は、この「最初に読む順序」と「既存ドキュメントの優先順位」も更新する。

## Git運用

- 標準のGit履歴を使う。
- 編集前後に作業ツリーを確認し、ユーザーや他エージェントの未コミット変更を勝手に戻さない。
- タスク完了時は、品質チェック、レビュー、必要なドキュメント更新を終えたあと、自律的にコミットを作成する。
- コミット前に必ず `git status` と差分を確認し、対象外の変更が混ざっていないことを確認する。
- コミットは、検証済みのまとまった単位で行う。作業ツリー全体を機械的に一括コミットしない。
- 複数の論理単位がある場合は、対象ファイルだけを stage して複数コミットに分ける。
- ドキュメント整理、機能実装、テスト追加、運用ルール変更などは、混ぜる必然性がなければ分離する。
- 1コミットは1つの目的を持ち、そのコミット単体で説明可能な変更にする。分割すると逆に壊れる、または検証単位が不自然になる場合は1コミットにまとめる。
- 無関係な変更を同じコミットに混ぜない。
- 品質チェックが失敗した変更は原則コミットしない。例外的に残す場合は理由を明示する。
- push は自動では行わない。ユーザーが依頼した場合、または別途ルール化された場合のみ行う。

## 既存ドキュメントの優先順位

- 現在状態と次タスク: `docs/progress.md`
- 時系列ログ: `docs/process-log.md`
- 重要意思決定: `docs/decision-log.md`
- 重要判断前の調査基準: `docs/decision-prep-standard.md`
- 長期影響のある設計判断: `docs/adr/`
