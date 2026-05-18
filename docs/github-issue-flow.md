# GitHub Issue Flow

## 方針

GitHub IssueをCodex作業のタスク入口にします。agentはIssueの状態をラベルとコメントで返し、人間がGitHub上で進捗を追えるようにします。

初期実装ではGitHub操作に `gh` CLIを使います。複数repository登録時は、repoごとに `gh --repo <owner/repo>` を明示して操作します。

## 対象Issue

通常taskとして処理対象にするIssueは、同一repository内のopen Issueのうち `run-weaver:ready` ラベルが付いたものです。

`run-weaver repo add` で複数repositoryを登録している場合、daemonは各repositoryを独立して監視します。同じrepository内では1 jobずつ処理し、別repositoryのjobは同時に実行できます。

同じrepository内に複数の通常Issueがある場合、daemonはIssue番号の小さい順に候補を評価します。実行中jobがある間は同じrepositoryで別Issueを開始しません。継続daemonでは1件完了後、次回pollで次の実行可能Issueを開始します。`--once` は互換性のため、各repositoryで最大1アクションだけ行います。

Campaignとして処理対象にするIssueは、同一repository内のopen Issueのうち `run-weaver:campaign` と `run-weaver:ready` の両方が付いたものです。Campaign Issueは通常task取得から除外し、親IssueとしてPlanner / Dispatcherが扱います。

Campaign Plannerが作成した子Issueには `run-weaver:campaign-task` を付けます。このラベル付きIssueは通常task取得から除外し、親Campaign state上のDispatcherだけが処理します。

以下のIssueは処理しません。

- `running` ラベルが付いている
- `done` ラベルが付いている
- `blocked` ラベルが付いている
- `run-weaver:campaign-task` ラベルが付いている。Campaign Dispatcher経由では処理する
- pull request

`run-weaver:ready` は人間がagentへ作業を渡す入口ラベルです。agentがIssueをclaimした後も履歴として残してよいですが、再実行可否は `running` / `done` / `blocked` とstate commentで判断します。

`run-weaver:campaign` はroadmapをtask graphへ展開する親Issueを表します。agentは親Campaign Issueをclaimしたあと、Codex Plannerにrepo docsと親Issue本文を読ませ、JSON task graphを生成してから子Issueを作成します。

## 通常Issueの依存関係

通常Issueは原則として `1 issue = 1 draft PR` で処理します。Issueタイトル、本文、人間コメントに明確な依存表現がある場合だけstacked PRとして扱います。run-weaverのclaim、blocked、completedコメントは依存検出から除外します。

認識する依存表現:

- `depends: #123`
- `depends on #123`
- `blocked by #123`
- `after #123`
- `stacked on #123`
- `依存: #123`
- `#123 の後`

複数依存が書かれている場合、すべての依存先Issueが `done` で、PR URLとbranchをstate fileまたは完了コメントから復元できる場合だけ実行します。worktree作成とdraft PR作成では、最後に列挙された依存Issueのbranchをbaseにします。依存先が未完了なら対象Issueは待機扱いにし、daemonは次の実行可能Issueを探します。依存先が `done` でもPR URLまたはbranchを復元できない場合や、依存があることだけ分かってIssue番号が読めない場合は、対象Issueを `blocked` にして理由をコメントします。

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

Codex promptでは、repositoryの `AGENTS.md` が禁止しておらず、上位の実行時指示が許可し、Codex実行環境が提供している場合、調査、レビュー、委譲可能な小タスクにCodex built-in subagentsを使うよう指示します。利用不可または禁止されている場合はセルフレビューで続行します。

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

## Rate Limit

Codex CLIがrate limitで中断した場合、agentはIssueを `blocked` にせず、`running` のまま自動再開を試します。

rate limitを検出したpollでは、Issueへ中間コメントを投稿します。コメントには以下を含めます。

- resume attempt番号
- Codex session。session idが取れない場合は最後のsessionを使うこと
- worktree path
- JSONL log path
- stateが `running` のまま維持されていること
- 検出時刻

secret、token、環境変数の値、JSONLログ本文はコメントに含めません。resume attemptはstate fileの `retryCount` に保存し、`lastGitHubCommentAt` と `lastError` も更新します。同じsessionでrate limitが繰り返される場合も、再開attemptごとにIssueコメントを残し、人間がGitHub上で停止理由と再開状況を追えるようにします。

## Blocked

agentが自力で進められない場合は `blocked` にします。

代表例:

- GitHub認証が切れている
- Doppler必須repositoryでDoppler CLI、Doppler token、または設定がない
- Codex CLI認証がない
- Codex CLIが失敗した
- worktree作成に失敗した
- merge conflictや生成物の不整合がある
- draft PR作成に失敗した
- 人間判断が必要な仕様不明点がある
- claim状態がGitHubとstate fileで矛盾している

draft PR作成前に最新baseをmergeしてconflictを検出します。通常のconflictは同じworktree上でCodexを `conflict-resolve` phaseとして1回だけ起動し、unmerged file、conflict marker、`git diff --check` の失敗が残らない場合だけPR作成へ進めます。lockfile、GitHub Actions workflow、migrationなどの高リスク競合、または1回の解消後も残る競合は `blocked` にし、PRは作成しません。

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

作業完了時は、PR作成前のbase取り込みと競合検証を通過した場合にdraft PRを作成します。高リスク競合または未解消競合で `blocked` になった場合、draft PRは作成しません。

PR本文にはIssueへの参照を含めます。

```text
Closes #<issue-number>
```

stacked PRの場合は依存先IssueとPR URLも本文に含め、draft PRのbase branchを依存先branchにします。

ただし、draft PR段階では人間レビューを前提にし、自動mergeは行いません。

## Campaign Flow

Campaign Issueには `run-weaver:campaign` と `run-weaver:ready` を付けます。

Campaign開始時にagentは以下を行います。

1. 親Campaign Issueをclaimする
2. Codex PlannerをCampaign用worktreeで非同期実行する
3. Plannerがrepo docs優先で親Issue本文とrepository内roadmap/docsを読み、JSONのtask graphを返す
4. JSON planを検証し、通常taskを子Issueとして作成して `run-weaver:ready` と `run-weaver:campaign-task` を付ける
5. decision gateをDecision Requestコメントとして親Issueへ投稿する
6. state fileの `campaign` に親Issue、Planner状態、tasks、decisionsを保存する

Planner出力はJSONだけを正とします。`tasks[]` は `id`、`title`、`body`、`dependencies[]` を持ち、`decisions[]` は `id`、`title`、`options[]`、`recommendation`、`blockedTasks[]`、`canContinueTasks[]` を持ちます。task粒度は1 draft PR単位です。JSONが不正、taskが空、ID重複、未知のdependency、未知taskを参照するdecisionがある場合、親Campaignを `blocked` にします。

Campaign taskは `plan`、`implement`、`review`、`verify` の順に進みます。同じtaskの各phaseは同じworktreeとbranchを使います。Campaign Plannerと各phaseのpromptでも、AGENTS.mdが禁止しておらず上位の実行時指示が許可している場合はCodex built-in subagentsの利用を促します。task dependencyを書く場合は `depends: task-...` のtask IDを使います。task開始時またはphase進行時にworktree、Doppler、prompt、runnerで失敗した場合は、子Issue、Campaign task、Campaign status、state jobを `blocked` に揃えます。`verify` 完了後にtaskごとのdraft PRを作成し、PR URLをCampaign stateへ保存します。

Decision Requestコメントには以下を含めます。

- decision
- context
- evidence
- options
- option details
- recommendation
- impact
- reversibility
- blocked tasks
- can continue tasks

Decision Requestでは、観測済み事実、推論、推奨を分けます。secret、token、環境変数値、JSONLログ本文は含めず、必要な場合はローカルpathや確認済み事実の要約だけを含めます。人間が判断しやすいよう、各optionには実行内容、結果、主なリスク、止まるtaskを含めます。

人間がPCから判断を返す場合は、`run-weaver status` に表示される回答用コマンドを実行します。

```sh
run-weaver decision answer --repo <owner/repo> '#<campaign-issue>' <decision-id> <option>
```

外出先などGitHub Issueだけを使える場合は、Decision Request内のquick replyから1行を選び、親Campaign Issueへ新規コメントとして投稿します。CLIはローカルstate fileから対象Campaignとoptionを検証し、quick reply経由ではdaemonがDecision Requestの選択肢に含まれるoptionだけをstateへ保存します。回答済みdecisionの内容は後続Campaign task promptへ含めます。

pending decisionがある場合、Dispatcherは `can continue tasks` に含まれるtaskだけを実行します。実行可能なtaskがなく、人間判断が必要な場合はCampaign stateを `decision_required` にして停止します。
