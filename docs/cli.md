# CLI

## 方針

`run-weaver` は最初から確認用コマンドと状態表示を持ちます。agentを常駐させる前に `doctor` で依存関係と認証状態を確認し、運用中は `status` で現在のジョブを確認します。

## `run-weaver doctor`

実行環境がagentを動かせる状態か確認します。

```sh
run-weaver doctor --target wsl
run-weaver doctor --target windows
run-weaver doctor --target wsl --repo example/repo
run-weaver doctor --target wsl --json
```

確認項目:

- OSとtargetの整合性
- `git` の有無
- `gh` の有無とGitHub auth状態
- `codex` の有無と認証状態
- `doppler` の有無と `doppler run` の実行可否。Doppler必須repositoryのみ必須
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
  [ok] doppler               optional; use repo add --doppler required for repositories that need Doppler
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

`overall` と各checkの `status` は `ok`、`warn`、`missing`、`auth_required`、`blocked` のいずれかです。`warn` だけなら終了コードは `0` にします。`doctor --repo owner/repo` はrepo設定の `dopplerMode` とrepo rootのDoppler設定ファイルを見てDoppler必須性を判定し、必須repositoryでは `doppler run -- git --version` が通ることまで確認します。設定ファイルがあるだけでは認証済みとは扱いません。

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
run-weaver status --repo example/repo
```

表示項目:

- target
- daemon running
- current issue
- current label state
- runtime state
- current branch
- current worktree
- tmux session/window。WSL targetのみ
- state file path
- last GitHub comment timestamp
- last error
- campaign progress。Campaign実行中のみ

人間向け表示を標準にし、将来の連携用に `--json` を用意します。`status` はローカルstate fileを主情報源にし、process、tmux、GitHub Issueの状態を照合します。

`--repo` は `gh` CLIへ渡すowner/repo指定です。未指定の場合はカレントディレクトリのGitHub repository解決に任せます。

`run-weaver repo add` で複数repositoryが登録されている場合、`status` は登録済みrepositoryごとのstateを集約して表示します。`--repo owner/repo` を指定した場合は、そのrepositoryのrepo別stateだけを表示します。JSON出力ではトップレベルの `repositories[]` に各repositoryのstatus構造を入れます。

process照合はtargetごとの実行環境に合わせます。WSLではPIDへsignal 0を送り、Windowsでは `tasklist` のPID検索結果を使います。tmux照合はWSL targetのみ対象です。

`runtimeState` は `labelState` とreconciliationから算出する表示用の状態です。主な値は `running`、`codex_running`、`codex_completed`、`rate_limited_waiting`、`blocked`、`done`、`needs_attention` です。

`readyQueue` は同一repository内の通常Issue候補をIssue番号昇順で表示します。各項目の `state` は `ready`、`waiting_dependency`、`blocked_dependency` のいずれかです。依存関係はIssue本文、タイトル、人間コメントの明確な `depends: #123` などから判定し、run-weaver管理コメントは無視します。

人間向け出力例:

```text
run-weaver status

Target: wsl
Daemon: running (pid 18422)
State file: /home/takeru/.local/state/run-weaver/state.json

Current job:
  Issue: #42 Add account export
  Label state: running
  Runtime state: codex_running
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
  "schemaVersion": 3,
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
    "runtimeState": "codex_running",
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
  "campaign": {
    "issue": {
      "number": 7,
      "title": "Campaign roadmap"
    },
    "status": "running",
    "currentTaskId": "task-add-planner",
    "completedTasks": [],
    "tasks": [
      {
        "id": "task-add-planner",
        "title": "Add planner",
        "issueNumber": 101,
        "status": "running",
        "phase": "implement",
        "retryCount": 0
      }
    ],
    "decisions": [],
    "prs": []
  },
  "readyQueue": [
    {
      "issue": {
        "number": 43,
        "title": "Build follow-up"
      },
      "state": "waiting_dependency",
      "dependencies": [42]
    }
  ],
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

state fileは `status` の主情報源です。secret、token、環境変数の値は保存しません。現在のJSON schemaは `3` です。schema version `1` と `2` の既存state fileは読み込み時に互換扱いします。最小JSON schemaは以下です。

複数repository登録時のstate fileはrepositoryごとに分離します。WSL/Linuxでは `$XDG_STATE_HOME/run-weaver/repos/<repo-slug>/state.json`、Windowsでは `%LOCALAPPDATA%\\run-weaver\\repos\\<repo-slug>\\state.json` を使います。既存の単一repo state fileは読み取り互換として残します。

```json
{
  "schemaVersion": 3,
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
    "baseBranch": "codex/issue-41-foundation",
    "dependencies": [
      {
        "issueNumber": 41,
        "branch": "codex/issue-41-foundation",
        "prUrl": "https://github.com/example/repo/pull/41"
      }
    ],
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
  },
  "campaign": {
    "issue": {
      "number": 7,
      "title": "Campaign roadmap",
      "url": "https://github.com/example/repo/issues/7"
    },
    "status": "running",
    "currentTaskId": "task-add-planner",
    "tasks": [
      {
        "id": "task-add-planner",
        "title": "Add planner",
        "issueNumber": 101,
        "status": "running",
        "phase": "implement",
        "retryCount": 0,
        "prUrl": ""
      }
    ],
    "decisions": [],
    "prs": [],
    "completedTasks": []
  },
  "completedIssues": [
    {
      "issue": {
        "number": 41,
        "title": "Foundation",
        "url": "https://github.com/example/repo/issues/41",
        "repository": "example/repo"
      },
      "branch": "codex/issue-41-foundation",
      "prUrl": "https://github.com/example/repo/pull/41"
    }
  ]
}
```

`job` は処理中Issueがない場合 `null` にします。Campaign実行中で次task待ちの場合も `job: null` になり、Campaignの進捗は `campaign` に残ります。`labelState` は `running`、`done`、`blocked` のいずれかです。通常Issueのstacked PRでは `baseBranch` と `dependencies` を保存します。`completedIssues` は後続Issueの依存解決用に直近完了IssueのPR URLとbranchを保存します。時刻はUTCのRFC 3339形式に統一します。

## `run-weaver repo`

監視対象repositoryを登録・確認・削除します。

```sh
run-weaver repo add
run-weaver repo add --doppler required
run-weaver repo add --doppler optional
run-weaver repo list
run-weaver repo list --json
run-weaver repo remove example/repo
```

`repo add` はカレントディレクトリの `git remote get-url origin` からrepository URLと `owner/repo` を推定して登録します。repo設定ファイルはWSL/Linuxでは `$XDG_CONFIG_HOME/run-weaver/repos.json`、未設定時は `~/.config/run-weaver/repos.json`、Windowsでは `%LOCALAPPDATA%\\run-weaver\\repos.json` です。

repo設定のschemaは `schemaVersion` と `repositories[]` で、各entryは `repository`、`repoUrl`、`addedFrom`、`enabled`、`addedAt`、`dopplerMode` を持ちます。`dopplerMode` は `auto`、`required`、`optional` のいずれかで、未指定の既存設定は `auto` として読み込みます。`auto` ではrepo rootの `doppler.yaml`、`doppler.yml`、`.doppler.yaml`、`.doppler.yml` を検出した場合だけDoppler必須とします。secretやtokenは保存しません。同じrepositoryを再度 `repo add` した場合は重複追加せず、URL、登録元、Doppler modeを更新します。`repo remove` は設定から外すだけで、clone、worktree、state fileは削除しません。

## `run-weaver install`

agentの導入と常駐設定を行います。

```sh
curl -fsSL https://raw.githubusercontent.com/ota-takeru/run-weaver/main/scripts/install.sh | sh -s -- --target wsl --repo-url https://github.com/example/repo.git --repo example/repo
iwr https://raw.githubusercontent.com/ota-takeru/run-weaver/main/scripts/install.ps1 -OutFile $env:TEMP\install-run-weaver.ps1
powershell -ExecutionPolicy Bypass -File $env:TEMP\install-run-weaver.ps1 -RepoUrl https://github.com/example/repo.git -Repo example/repo
run-weaver install --target wsl
run-weaver install --target wsl --repo-url https://github.com/example/repo.git
run-weaver install --target windows --repo-url https://github.com/example/repo.git
```

WSL target:

- `scripts/install.sh` がGitHub Releasesからバイナリを配置
- systemd user serviceを作成
- tmux利用前提のログ確認導線を設定

Windows target:

- `scripts/install.ps1` がGitHub Releasesからバイナリを `%LOCALAPPDATA%\\run-weaver\\bin\\run-weaver.exe` に配置
- per-user Task Schedulerに `run-weaver` taskを登録
- `--repo-url` でCodex専用clone作成元のrepository URLを明示指定できる。未指定時はdaemonが `repos.json` の登録repositoryを読む
- `--repo` を指定した場合はdaemonのGitHub CLI操作にowner/repoを渡す。未指定でも `--repo-url` がGitHub URLならowner/repoを自動推定する
- 標準出力と標準エラーを `%LOCALAPPDATA%\\run-weaver\\logs\\daemon.log` に追記

`install.sh` と `install.ps1` は、初回取得と `run-weaver install` 呼び出しを行う薄いラッパーです。ローカルにこのprojectのcloneがなくても使えます。release asset名は `run-weaver_linux_amd64.tar.gz`、`run-weaver_linux_arm64.tar.gz`、`run-weaver_windows_amd64.zip`、`run-weaver_windows_arm64.zip` です。

`install` は常駐設定を作成します。複数repository運用ではservice / Task Schedulerのdaemon起動引数にrepositoryを固定せず、daemonが `repos.json` を読みます。既存互換として `--repo-url` / `--repo` を指定した場合は、そのrepositoryをrepo設定にも登録し、単一repo daemon引数も生成します。実際にIssue処理を開始できる状態にするには、事前に `doctor` が確認する依存関係と認証、対象repositoryの `run-weaver:ready` Issueが必要です。

## `run-weaver update`

GitHub Releasesのlatest releaseを確認し、現在の実行ファイルを更新します。

```sh
run-weaver update --check
run-weaver update
```

release buildの `run-weaver daemon` は起動時に同じ更新確認を行います。`install.sh` / `install.ps1` は常にlatest releaseを取得するため、`run-weaver install` 自体では自動更新を行いません。`Version` が `dev` の開発ビルドでは自動更新しません。更新確認を一時停止する場合は `RUN_WEAVER_NO_UPDATE=1` を設定します。

更新確認はGitHub APIの `ota-takeru/run-weaver` latest releaseを読み、実行OS/ARCHに対応するassetだけを取得します。Windowsでは実行中のexeを直接置き換えられないため、更新用の一時exeを置いて現在のprocessを終了し、短い遅延後に置換して同じ引数で起動し直します。

release作成は利用者向けCLIには含めません。このrepositoryのmaintainerは `scripts/release.sh` でdry-run確認し、`scripts/release.sh --push` でtag pushしてGitHub Actionsのrelease workflowを起動します。dry-runでも `go test ./...` とLinux / Windows、amd64 / arm64のrelease cross-buildを実行します。`scripts/release.sh --push --watch` ではworkflow完了まで待ち、Linux / Windows、amd64 / arm64のrelease assetが揃ったことを確認します。

## `run-weaver daemon`

Issue監視とCodex実行を行う常駐プロセスです。

```sh
run-weaver daemon --target wsl
run-weaver daemon --target windows
run-weaver daemon --target wsl --once --repo-url https://github.com/example/repo.git
run-weaver daemon --target wsl --repo-url https://github.com/example/repo.git --poll-interval 1m
```

主な処理:

- `run-weaver:campaign` + `run-weaver:ready` ラベル付きのopen Campaign Issueを取得し、Codex Plannerでrepo docs優先のtask graphを生成してから子Issueへ展開
- 通常taskとして `run-weaver:ready` ラベル付きのopen IssueをIssue番号昇順で取得し、同じrepository内では最大1件だけ実行
- 明確な依存関係がある通常Issueは、依存先Issueの完了とPR branchを確認してstacked PRとして実行
- `running` ラベルと開始コメントを投稿
- ローカルstate fileにclaimを保存
- Codex専用cloneからIssue専用worktreeを作成
- Doppler必須repositoryでは `doppler run --` 経由で環境変数を注入し、不要repositoryではDopplerなしで続行
- Codex CLIをworktree上で実行
- draft PRを作成
- Issueへ結果コメントを投稿
- `done` または `blocked` ラベルへ更新

Campaign開始時はまず `campaign.status: planning` としてCodex Plannerを非同期に起動します。Plannerはrepository docsと親Issue本文を読み、JSONの `tasks[]` / `decisions[]` を返します。JSON planの検証に通った場合だけ子Issueを作成します。Campaign taskでは `plan`、`implement`、`review`、`verify` のphaseを順に実行します。phaseごとにstate配下の別ログディレクトリを使い、Codex reasoning effortはrunner設定内で `campaign-planner` / `plan` / `review` をmedium、`implement` をlowとして渡せる場合だけCLI config overrideで渡します。

Decision Requestへの回答は、親Campaign Issueコメント内の `run-weaver-decision:<decision-id>:<option>` を読み取り、state fileの `campaign.decisions[].answer` に保存します。

Codex CLI起動の初期仕様:

- 非対話実行として `codex exec` を使う
- worktreeを `codex exec --cd <worktree>` で指定する
- 標準出力のJSONLログはstate配下のIssue別ディレクトリに保存する
- 最終応答は `--output-last-message` でstate配下に保存する
- 終了コード `0` 以外、JSONLログの異常終了、draft PR作成失敗は `blocked` とする
- tmux window終了後にJSONLログが `codex: command not found` などCodex起動失敗を示す場合は、stuckした `running` のままにせず `blocked` とする
- JSONLログがrate limitを示し、Codex session idを取得できる場合は `blocked` にせず、次回pollで `codex exec resume <session>` により同じworktreeと前回sessionから再開する
- session idを取得できない場合でも、同じworktreeへ移動して `codex exec resume --last` を試す
- rate limit検出時はIssueに中間コメントを投稿し、resume attempt番号、session、worktree、JSONLログpath、検出時刻を残す。secret値やJSONLログ本文はコメントしない
- rate limit resume attemptはstate fileの `retryCount`、`lastGitHubCommentAt`、`lastError` にも反映する
- Doppler必須repositoryでDoppler CLIまたは `doppler run` 可能な認証/設定がない場合はCodex起動前に `blocked` とし、Doppler不要repositoryではDoppler未インストールでも続行する
- Codexに渡すDoppler service tokenは環境変数から注入し、ログ、Issueコメント、PR本文、state fileには値を出さない

WSL targetでは、この `codex exec` をtmux window内で起動します。`run-weaver install --target wsl` は実行時PATHをsystemd user serviceの `Environment=PATH=...` に保存し、nvmなどユーザーPATH上の `codex` もserviceから見えるようにします。tmux session名は `run-weaver`、window名は単一repo互換では `issue-<number>`、repo登録経由では `<repo-slug>-issue-<number>` とします。Windows targetではtmuxを使わず、PowerShell経由のdirect runnerでCodexを非同期起動します。

`--once` は1回だけ処理して終了します。複数repository登録時は各repositoryで最大1件ずつ確認・開始します。指定しない場合は `--poll-interval` ごとに継続pollし、登録repositoryごとに独立した処理ループを起動します。`--repo-url` はCodex専用cloneを新規作成する場合に使うrepository URLです。`--repo` と併用した場合は互換用の単一repoモードとして動きます。

## `run-weaver dashboard`

将来追加予定のコマンドです。

```sh
run-weaver dashboard
```

localhostで状態確認用UIを起動します。初期実装では対象外とし、CLI、tmux、GitHub Issueで状態を確認します。
