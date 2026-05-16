# CLI

## 方針

`run-weaver` は最初から確認用コマンドと状態表示を持ちます。agentを常駐させる前に `doctor` で依存関係と認証状態を確認し、運用中は `status` で現在のジョブを確認します。

## `run-weaver doctor`

実行環境がagentを動かせる状態か確認します。

```sh
run-weaver doctor --target wsl
run-weaver doctor --target windows
run-weaver doctor --target wsl --json
```

確認項目:

- OSとtargetの整合性
- `git` の有無
- `gh` の有無とGitHub auth状態
- `codex` の有無と認証状態
- `doppler` の有無とtokenまたは設定状態
- WSL targetの場合は `tmux` の有無
- WSL targetの場合は `systemctl --user` が利用可能で、systemd user serviceを登録できること
- Codex専用cloneの存在
- worktree作成先の書き込み可否
- state fileの読み書き可否
- 常駐設定の有無。WSLはsystemd user、WindowsはTask Scheduler

`doctor --json` は初期実装から提供します。人間向け出力とJSON出力は同じ検査結果を使い、JSONは自動検査や将来のdashboard連携で使います。

人間向け出力例:

```text
run-weaver doctor --target wsl

Target: wsl
Overall: blocked

Checks:
  [ok] os target             WSL detected
  [ok] git                   /usr/bin/git
  [ok] gh auth               logged in as octocat
  [ok] codex                 codex exec available
  [warn] doppler             DOPPLER_TOKEN is not set; will use local doppler config
  [ok] tmux                  /usr/bin/tmux
  [ok] systemd user          systemctl --user is running
  [missing] codex clone       /home/takeru/.local/share/run-weaver/src does not exist
  [ok] worktree root         writable: /home/takeru/.local/share/run-weaver/worktrees
  [ok] state file            writable: /home/takeru/.local/state/run-weaver/state.json
  [missing] service           run-weaver.service is not installed

Next:
  run-weaver install --target wsl
```

JSON出力例:

```json
{
  "target": "wsl",
  "overall": "blocked",
  "stateFile": "/home/takeru/.local/state/run-weaver/state.json",
  "checks": [
    {
      "id": "os_target",
      "label": "OS target",
      "status": "ok",
      "message": "WSL detected"
    },
    {
      "id": "codex_clone",
      "label": "Codex clone",
      "status": "missing",
      "message": "/home/takeru/.local/share/run-weaver/src does not exist"
    }
  ],
  "next": [
    "run-weaver install --target wsl"
  ]
}
```

`overall` と各checkの `status` は `ok`、`warn`、`missing`、`auth_required`、`blocked` のいずれかです。`warn` だけなら終了コードは `0` にします。

終了コード:

- `0`: 実行可能
- `1`: 必須依存が不足
- `2`: 認証不足
- `3`: 設定不足
- `4`: targetと実行OSが矛盾している

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

人間向け出力例:

```text
run-weaver status

Target: wsl
Daemon: running (pid 18422)
State file: /home/takeru/.local/state/run-weaver/state.json

Current job:
  Issue: #42 Add account export
  Label state: running
  Branch: codex/issue-42-add-account-export
  Worktree: /home/takeru/.local/share/run-weaver/worktrees/issue-42
  Claim: run-weaver:wsl:20260516T093000Z
  Claimed at: 2026-05-16T09:30:00Z
  tmux: run-weaver:issue-42

Reconciliation:
  GitHub: running
  Process: running
  tmux: attached=false, window exists
  Last GitHub comment: 2026-05-16T09:31:10Z
  Last error: none
```

`status --json` のトップレベル構造:

```json
{
  "schemaVersion": 1,
  "target": "wsl",
  "stateFile": "/home/takeru/.local/state/run-weaver/state.json",
  "daemon": {
    "running": true,
    "pid": 18422,
    "service": "run-weaver.service"
  },
  "job": {
    "issue": {
      "number": 42,
      "title": "Add account export",
      "url": "https://github.com/example/repo/issues/42"
    },
    "labelState": "running",
    "branch": "codex/issue-42-add-account-export",
    "worktree": "/home/takeru/.local/share/run-weaver/worktrees/issue-42",
    "claimId": "run-weaver:wsl:20260516T093000Z",
    "claimedAt": "2026-05-16T09:30:00Z",
    "tmux": {
      "session": "run-weaver",
      "window": "issue-42"
    },
    "lastGitHubCommentAt": "2026-05-16T09:31:10Z",
    "lastError": null
  },
  "reconciliation": {
    "githubLabelState": "running",
    "processState": "running",
    "tmuxState": "window_exists",
    "conflicts": []
  }
}
```

`status` の終了コード:

- `0`: state file、process、target固有runner、GitHub状態が整合している
- `1`: state fileがない、または読めない
- `2`: GitHub照合に認証が必要
- `3`: GitHub、process、tmux、state fileの状態が矛盾している
- `4`: targetと実行OSが矛盾している

state fileとGitHubの状態が矛盾する場合、表示上の `Label state` はGitHubの `running` / `done` / `blocked` ラベルを優先します。state fileの値は破棄せず、矛盾内容を `lastError` と `reconciliation.conflicts` に残します。

## State File

state fileは `status` の主情報源です。secret、token、環境変数の値は保存しません。最小JSON schemaは以下です。

```json
{
  "schemaVersion": 1,
  "target": "wsl",
  "updatedAt": "2026-05-16T09:31:10Z",
  "daemon": {
    "pid": 18422,
    "service": "run-weaver.service"
  },
  "job": {
    "issue": {
      "number": 42,
      "title": "Add account export",
      "url": "https://github.com/example/repo/issues/42",
      "repository": "example/repo"
    },
    "labelState": "running",
    "branch": "codex/issue-42-add-account-export",
    "worktree": "/home/takeru/.local/share/run-weaver/worktrees/issue-42",
    "claimId": "run-weaver:wsl:20260516T093000Z",
    "claimedAt": "2026-05-16T09:30:00Z",
    "lastGitHubCommentAt": "2026-05-16T09:31:10Z",
    "tmux": {
      "session": "run-weaver",
      "window": "issue-42"
    },
    "codex": {
      "sessionId": null,
      "lastMessagePath": "/home/takeru/.local/state/run-weaver/issues/42/last-message.txt",
      "jsonLogPath": "/home/takeru/.local/state/run-weaver/issues/42/codex.jsonl"
    },
    "lastError": null
  }
}
```

`job` は処理中Issueがない場合 `null` にします。`labelState` は `running`、`done`、`blocked` のいずれかです。時刻はUTCのRFC 3339形式に統一します。

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
run-weaver daemon --target wsl --once --repo-url https://github.com/example/repo.git
run-weaver daemon --target wsl --repo-url https://github.com/example/repo.git --poll-interval 1m
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

Codex CLI起動の初期仕様:

- 非対話実行として `codex exec` を使う
- worktreeを `codex exec --cd <worktree>` で指定する
- 標準出力のJSONLログはstate配下のIssue別ディレクトリに保存する
- 最終応答は `--output-last-message` でstate配下に保存する
- 終了コード `0` 以外、JSONLログの異常終了、draft PR作成失敗は `blocked` とする
- Codexに渡すDoppler service tokenは環境変数から注入し、ログ、Issueコメント、PR本文、state fileには値を出さない

WSL targetでは、この `codex exec` をtmux window内で起動します。tmux session名は `run-weaver`、window名は `issue-<number>` とします。

`--once` は1件だけ処理して終了します。指定しない場合は `--poll-interval` ごとに継続pollします。`--repo-url` はCodex専用cloneを新規作成する場合に使うrepository URLです。`--repo` は `gh` CLIへ渡すowner/repo指定で、未指定の場合はカレントディレクトリのGitHub repository解決に任せます。

## `run-weaver dashboard`

将来追加予定のコマンドです。

```sh
run-weaver dashboard
```

localhostで状態確認用UIを起動します。初期実装では対象外とし、CLI、tmux、GitHub Issueで状態を確認します。
