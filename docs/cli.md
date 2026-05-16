# CLI

## 方針

`run-weaver` は最初から確認用コマンドと状態表示を持ちます。agentを常駐させる前に `doctor` で依存関係と認証状態を確認し、運用中は `status` で現在のジョブを確認します。

## `run-weaver doctor`

実行環境がagentを動かせる状態か確認します。

```sh
run-weaver doctor --target wsl
run-weaver doctor --target windows
```

確認項目:

- OSとtargetの整合性
- `git` の有無
- `gh` の有無とGitHub auth状態
- `codex` の有無と認証状態
- `doppler` の有無とtokenまたは設定状態
- WSL targetの場合は `tmux` の有無
- Codex専用cloneの存在
- worktree作成先の書き込み可否
- state fileの読み書き可否
- 常駐設定の有無。WSLはsystemd user、WindowsはTask Scheduler

終了コード:

- `0`: 実行可能
- `1`: 必須依存が不足
- `2`: 認証不足
- `3`: 設定不足

## `run-weaver status`

常駐状態と現在のジョブを表示します。

```sh
run-weaver status
run-weaver status --json
```

表示項目:

- target
- daemon running
- current issue
- current label state
- current branch
- current worktree
- tmux session/window。WSL targetのみ
- state file path
- last GitHub comment timestamp
- last error

人間向け表示を標準にし、将来の連携用に `--json` を用意します。`status` はローカルstate fileを主情報源にし、process、tmux、GitHub Issueの状態を照合します。

## `run-weaver install`

agentの導入と常駐設定を行います。

```sh
run-weaver install --target wsl
run-weaver install --target windows
```

WSL target:

- GitHub Releasesからバイナリを配置
- systemd user serviceを作成
- tmux利用前提のログ確認導線を設定

Windows target:

- GitHub Releasesからバイナリを配置
- Task Schedulerに常駐タスクを登録

`install.sh` と `install.ps1` は、初回取得と `run-weaver install` 呼び出しを簡単にする薄いラッパーとして扱います。

## `run-weaver daemon`

Issue監視とCodex実行を行う常駐プロセスです。

```sh
run-weaver daemon --target wsl
run-weaver daemon --target windows
```

主な処理:

- `run-weaver:ready` ラベル付きのopen Issueを取得
- `running` ラベルと開始コメントを投稿
- ローカルstate fileにclaimを保存
- Codex専用cloneからIssue専用worktreeを作成
- Doppler経由で環境変数を注入
- Codex CLIをworktree上で実行
- draft PRを作成
- Issueへ結果コメントを投稿
- `done` または `blocked` ラベルへ更新

## `run-weaver dashboard`

将来追加予定のコマンドです。

```sh
run-weaver dashboard
```

localhostで状態確認用UIを起動します。初期実装では対象外とし、CLI、tmux、GitHub Issueで状態を確認します。
