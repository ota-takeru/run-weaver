# Architecture

## 全体方針

`run-weaver` はGoで実装する単一バイナリです。WindowsとWSLで別実装を持たず、同じagentを `--target` だけ変えて常駐させます。

```sh
run-weaver daemon --target wsl
run-weaver daemon --target windows
```

環境差分はtarget adapterに閉じ込めます。Issue検出、状態更新、worktree管理、Codex起動、PR作成の中核ロジックは共通化します。

## コンポーネント

### Daemon

GitHub Issueを定期的に監視し、実行対象を選びます。対象Issueごとに専用worktreeを作成し、Codex CLIを実行します。

初期実装ではGitHub操作に `gh` CLIを使います。これにより、既存のGitHub認証状態をそのまま利用できます。

### Target Adapter

targetごとのOS差分を扱います。

- `wsl`: systemd user serviceで常駐し、Codex実行ログをtmux sessionに残す
- `windows`: Task Schedulerで常駐し、Windows側のログ出力先を使う

`--target` は必須にし、agentが暗黙に環境を推測して破壊的な操作をしないようにします。

### Worktree Manager

Codexは人間用 `src` を直接触りません。Codex専用cloneを作り、Issueごとに専用worktreeを作成します。

想定する作業単位:

- Codex専用clone: agentが管理する永続clone
- Issue専用worktree: `issue-<number>` 単位で作成
- branch: `codex/issue-<number>-<slug>` のようにIssue由来で作成

完了後はdraft PRを作成し、Issueにリンクを返します。

### Secret Manager

DB認証情報や環境変数はDopplerで一元管理します。CodexにはDopplerの `dev-agent` secretだけを渡します。

agentはCodex起動時にDoppler経由で必要な環境変数を注入します。secretの実体をログ、Issueコメント、PR本文には出しません。

### Runner

Codex CLIはworktree上で起動します。

WSL targetではtmux sessionを使い、作業ログを人間が確認できるようにします。

```sh
tmux attach -t run-weaver
```

tmux window名はIssue番号を含めます。

## 状態管理

状態は3つに絞ります。

- `running`: agentがIssueを処理中
- `done`: draft PR作成まで完了
- `blocked`: 認証、依存関係、Codex失敗、競合、人間確認待ちなどで停止

状態はGitHub Issueのラベルとコメントで返します。CLIの `status` でも同じ情報を確認できます。

## 更新方式

agent自体はGitHub Releases経由で配布します。

導入は以下の2段階を想定します。

- `install.sh` / `install.ps1`: 初回取得と基本設定
- `run-weaver install`: targetごとの常駐設定

## 将来拡張

`run-weaver dashboard` を追加し、localhost上で状態確認できるようにします。

初期段階ではdashboardを実装せず、CLI、tmux、GitHub Issueコメントを状態確認の主経路にします。
