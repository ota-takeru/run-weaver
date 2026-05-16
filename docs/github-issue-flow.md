# GitHub Issue Flow

## 方針

GitHub IssueをCodex作業のタスク入口にします。agentはIssueの状態をラベルとコメントで返し、人間がGitHub上で進捗を追えるようにします。

初期実装ではGitHub操作に `gh` CLIを使います。

## 状態ラベル

agentが管理する状態ラベルは3つです。

- `running`: 作業中
- `done`: draft PR作成まで完了
- `blocked`: agentが自力で進められない

この3ラベルは排他的に扱います。状態更新時には古い状態ラベルを外し、新しい状態ラベルだけを付けます。

## Issue開始

agentがIssueを処理対象にしたら、以下を行います。

1. `running` ラベルを付ける
2. `done` と `blocked` ラベルを外す
3. 開始コメントを投稿する
4. Issue専用worktreeとbranchを作成する
5. Codex CLIを起動する

開始コメントには以下を含めます。

- target
- worktree名
- branch名
- tmux session/window。WSL targetのみ
- 開始時刻

secretや環境変数の値はコメントに含めません。

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

blockedコメントには以下を含めます。

- 失敗した段階
- エラー概要
- 再実行できるか
- 人間に必要な対応
- 関連するtmux session/window。WSL targetのみ

secretやtokenの値はコメントに含めません。

## 再実行

再実行時は、既存のIssue専用worktreeとbranchの扱いを明示的に決めます。

初期方針:

- `blocked` からの再実行は同じbranchを再利用する
- worktreeが壊れている場合は新しいworktreeを作る
- 再実行コメントをIssueに追記する

## Draft PR

作業完了時は必ずdraft PRを作成します。

PR本文にはIssueへの参照を含めます。

```text
Closes #<issue-number>
```

ただし、draft PR段階では人間レビューを前提にし、自動mergeは行いません。
