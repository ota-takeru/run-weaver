# GitHub Issue Flow

## 方針

GitHub IssueをCodex作業のタスク入口にします。agentはIssueの状態をラベルとコメントで返し、人間がGitHub上で進捗を追えるようにします。

初期実装ではGitHub操作に `gh` CLIを使います。複数repository登録時は、repoごとに `gh --repo <owner/repo>` を明示して操作します。

## 対象Issue

通常taskとして処理対象にするIssueは、同一repository内のopen Issueのうち `run-weaver:ready` ラベルが付いたものです。

`run-weaver repo add` で複数repositoryを登録している場合、daemonは各repositoryを独立して監視します。同じrepository内では1 jobずつ処理し、別repositoryのjobは同時に実行できます。

Campaignとして処理対象にするIssueは、同一repository内のopen Issueのうち `run-weaver:campaign` と `run-weaver:ready` の両方が付いたものです。Campaign Issueは通常task取得から除外し、親IssueとしてPlanner / Dispatcherが扱います。

以下のIssueは処理しません。

- `running` ラベルが付いている
- `done` ラベルが付いている
- `blocked` ラベルが付いている
- pull request

`run-weaver:ready` は人間がagentへ作業を渡す入口ラベルです。agentがIssueをclaimした後も履歴として残してよいですが、再実行可否は `running` / `done` / `blocked` とstate commentで判断します。

`run-weaver:campaign` はroadmapをtask graphへ展開する親Issueを表します。agentは親Campaign Issueをclaimしたあと、Markdown箇条書きからtaskを抽出して子Issueを作成します。

## 状態ラベル

agentが管理する状態ラベルは3つです。

- `running`: 作業中
- `done`: draft PR作成まで完了
- `blocked`: agentが自力で進められない

この3ラベルは排他的に扱います。状態更新時には古い状態ラベルを外し、新しい状態ラベルだけを付けます。

## Issue開始

agentがIssueを処理対象にしたら、以下を行います。

1. Issueの現在ラベルを再取得する
2. `running`、`done`、`blocked` が付いていないことを確認する
3. `running` ラベルを付ける
4. claim IDを含む開始コメントを投稿する
5. Issueのラベルと最新claimコメントを再取得する
6. 自分のclaim IDが最新claimとして確認できた場合だけ続行する
7. ローカルstate fileにclaimを保存する
8. Issue専用worktreeとbranchを作成する
9. Codex CLIを起動する

開始コメントには以下を含めます。

- claim ID
- target
- worktree名
- branch名
- tmux session/window。WSL targetのみ
- 開始時刻
- claimしたagent target

secretや環境変数の値はコメントに含めません。

## 二重実行防止

WSL targetとWindows targetが同じIssueを同時に拾う可能性があるため、`running` ラベルと開始コメントをclaimとして扱います。

claim IDは `run-weaver:<target>:<UTC timestamp>` を基本形にし、同一秒に複数起動する可能性に備えて短いランダムsuffixを付けます。

claim手順:

1. `run-weaver:ready` 付きopen Issueを取得する
2. 対象Issueのラベルを直前に再取得する
3. `running`、`done`、`blocked` がなければ `running` を付ける
4. claim IDを含む開始コメントを投稿する
5. Issueのラベルと最新claimコメントを再取得する
6. 最新claimコメントのclaim IDが自分のものなら、ローカルstate fileにIssue番号、target、branch、worktree、claim ID、claim時刻を保存する
7. 最新claimコメントのclaim IDが別agentのものなら、ローカルstate fileを更新せず、そのIssueはスキップする

GitHubのラベル更新はCASではないため、`running` 付与だけでclaim成功とは扱いません。開始コメント投稿後の再取得で、自分のclaim IDが最新claimコメントとして確認できた場合だけ作業を開始します。

別agentが直前に `running` を付けていた場合、または開始コメント競合に負けた場合、そのIssueはスキップします。`running` ラベルだけが残っていてstate commentや更新が古い場合は、初期実装では自動奪取せず `blocked` コメントで人間確認に回します。

## 完了

Codex作業が成功し、draft PRを作成できたら、以下を行います。

1. `done` ラベルを付ける
2. `running` と `blocked` ラベルを外す
3. 結果コメントを投稿する

完了コメントには以下を含めます。

- draft PR URL
- branch名
- worktree名
- 実行した主な確認コマンド
- state fileの最終状態
- 追加で人間確認が必要な点

## Blocked

agentが自力で進められない場合は `blocked` にします。

代表例:

- GitHub認証が切れている
- Doppler tokenまたは設定がない
- Codex CLI認証がない
- Codex CLIが失敗した
- worktree作成に失敗した
- merge conflictや生成物の不整合がある
- draft PR作成に失敗した
- 人間判断が必要な仕様不明点がある
- claim状態がGitHubとstate fileで矛盾している

blockedコメントには以下を含めます。

- 失敗した段階
- エラー概要
- 再実行できるか
- 人間に必要な対応
- state fileの最終状態
- 関連するtmux session/window。WSL targetのみ

secretやtokenの値はコメントに含めません。

## 再実行

再実行時は、既存のIssue専用worktreeとbranchの扱いを明示的に決めます。

初期方針:

- `blocked` からの再実行は同じbranchを再利用する
- worktreeが壊れている場合は新しいworktreeを作る
- 再実行コメントをIssueに追記する
- `running` が古く残っている場合は自動再実行せず、人間が `blocked` へ戻してから再実行する

## Draft PR

作業完了時は必ずdraft PRを作成します。

PR本文にはIssueへの参照を含めます。

```text
Closes #<issue-number>
```

ただし、draft PR段階では人間レビューを前提にし、自動mergeは行いません。

## Campaign Flow

Campaign Issueには `run-weaver:campaign` と `run-weaver:ready` を付けます。

Campaign開始時にagentは以下を行います。

1. 親Campaign Issueをclaimする
2. 本文のroadmap箇条書きからtask graphを作る
3. 通常taskを子Issueとして作成し、子Issueに `run-weaver:ready` を付ける
4. decision gateをDecision Requestコメントとして親Issueへ投稿する
5. state fileの `campaign` に親Issue、tasks、decisionsを保存する

Campaign taskは `plan`、`implement`、`review`、`verify` の順に進みます。同じtaskの各phaseは同じworktreeとbranchを使います。`verify` 完了後にtaskごとのdraft PRを作成し、PR URLをCampaign stateへ保存します。

Decision Requestコメントには以下を含めます。

- options
- recommendation
- blocked tasks
- can continue tasks

人間が判断を返す場合は、親Campaign Issueに以下の形式を含むコメントを追加します。

```text
run-weaver-decision:<decision-id>:<option>
```

実行可能なtaskがなく、人間判断が必要な場合はCampaign stateを `decision_required` にして停止します。
