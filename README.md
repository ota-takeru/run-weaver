# run-weaver

`run-weaver` は、GitHub Issueを入口にしてCodex CLIを安全に実行するための常駐agent構想です。

中心になる実行体はGo製の単一バイナリ `run-weaver` です。WindowsとWSLで同じagentを動かし、環境差分は `--target` で切り替えます。作業は人間用の `src` ではなく、Codex専用cloneとIssue専用worktree上で行います。

## 目的

- GitHub IssueをCodex作業のタスク入口にする
- Codex専用cloneとIssue専用worktreeで人間用作業領域を汚さない
- DopplerでDB認証情報や環境変数を一元管理する
- CodexにはDoppler service token `dev-agent` だけを渡す
- 作業状態をCLI、tmux、GitHub Issue上で確認できるようにする
- 作業後はdraft PRを作成する
- agent自体はGitHub Releases経由で更新する

## 初期CLI

最初から確認用コマンドと状態表示を持たせます。

```sh
run-weaver doctor
run-weaver doctor --json
run-weaver status
run-weaver status --json
run-weaver status --repo example/repo
run-weaver install --target wsl --repo-url https://github.com/example/repo.git
run-weaver install --target windows --repo-url https://github.com/example/repo.git
run-weaver update --check
run-weaver daemon --target wsl
run-weaver daemon --target windows
run-weaver daemon --target wsl --once --repo-url https://github.com/example/repo.git
run-weaver daemon --target wsl --repo-url https://github.com/example/repo.git --poll-interval 1m
```

将来的に、ブラウザから状態確認するための `run-weaver dashboard` を追加します。

## 基本フロー

1. 人間がGitHub Issueを作る
2. 人間が対象Issueに `run-weaver:ready` ラベルを付ける
3. `run-weaver daemon` が対象Issueを検出する
4. Issueに `running` ラベルを付け、claim ID付きの開始コメントを投稿する
5. 最新claimコメントを再確認し、勝ったagentだけがローカルstate fileにclaimを保存する
6. Codex専用cloneからIssue専用worktreeを作る
7. Doppler経由で必要なsecretを注入し、Codex CLIをworktree上で実行する
8. WSL側ではtmux sessionに実行ログを残す
9. 作業後にdraft PRを作成する
10. Issueに結果コメントを投稿し、`done` または `blocked` ラベルへ更新する

## 状態確認

`run-weaver status` は、常駐状態と現在のジョブを表示します。

- agent process
- target
- current issue
- current worktree
- tmux session name
- local state file path
- last state comment timestamp
- reconciliation result

WSLではtmuxで実行ログを確認できます。

```sh
tmux attach -t run-weaver
```

## 依存関係確認

`run-weaver doctor` は、実行環境が作業可能かを確認します。

- `git`
- `gh`
- `codex`
- `doppler`
- `tmux`。WSL targetのみ必須
- `systemctl --user`。WSL targetのみ必須
- GitHub認証状態
- Doppler認証またはtoken設定
- Codex CLI認証状態
- target別の常駐設定状態

JSON出力は初期実装から提供し、`doctor --json` と `status --json` は自動検査や将来のdashboard連携に使います。

## Windows確認

Windows固有のdoctor / status挙動はGitHub Actionsの `windows-latest` jobで検証します。手元Windowsで追加確認する場合は、PowerShellで以下を実行します。

```powershell
.\scripts\check-windows.ps1
```

このscriptはGitHub Issueへの書き込みやsecret表示を行いません。

## インストール手順

GitHub Releasesから取得するため、ローカルにこのプロジェクトをcloneしておく必要はありません。`install` は常駐設定を作成しますが、事前に `git`、`gh`、`codex`、`doppler` の導入と認証、Codex専用clone用のrepository URL、対象Issueの `run-weaver:ready` ラベルが必要です。secret値はコマンドやドキュメントに書かず、Doppler service token `dev-agent` は環境変数で渡します。

WSL:

```sh
curl -fsSL https://raw.githubusercontent.com/ota-takeru/run-weaver/main/scripts/install.sh | sh -s -- \
  --target wsl \
  --repo-url https://github.com/example/repo.git \
  --repo example/repo
~/.local/bin/run-weaver doctor --target wsl
```

Windows PowerShell:

```powershell
iwr https://raw.githubusercontent.com/ota-takeru/run-weaver/main/scripts/install.ps1 -OutFile $env:TEMP\install-run-weaver.ps1
powershell -ExecutionPolicy Bypass -File $env:TEMP\install-run-weaver.ps1 -RepoUrl https://github.com/example/repo.git -Repo example/repo
& "$env:LOCALAPPDATA\run-weaver\bin\run-weaver.exe" doctor --target windows
```

Windowsの `install` はper-user Task Scheduler task `run-weaver` を作成または更新します。daemonの標準出力と標準エラーは `%LOCALAPPDATA%\run-weaver\logs\daemon.log` に追記されます。

release buildの `run-weaver daemon` は起動時にGitHub Releasesのlatest releaseを確認し、新しいassetがあれば自動更新してから処理を続けます。インストールscriptは常にlatest releaseを取得し、手動更新は `run-weaver update` で実行できます。開発ビルドはversionが `dev` のため自動更新しません。更新を一時的に止める場合は `RUN_WEAVER_NO_UPDATE=1` を設定します。

## 設計文書

- [Architecture](docs/architecture.md)
- [CLI](docs/cli.md)
- [GitHub Issue Flow](docs/github-issue-flow.md)
