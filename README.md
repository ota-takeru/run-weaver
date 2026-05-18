# run-weaver

`run-weaver` は、GitHub Issueを入口にしてCodex CLIを安全に実行するための常駐agent構想です。

中心になる実行体はGo製の単一バイナリ `run-weaver` です。WindowsとWSLで同じagentを動かし、環境差分は `--target` で切り替えます。作業は人間用の `src` ではなく、Codex専用cloneとIssue専用worktree上で行います。

## 目的

- GitHub IssueをCodex作業のタスク入口にする
- Codex専用cloneとIssue専用worktreeで人間用作業領域を汚さない
- Dopplerが必要なrepositoryではDB認証情報や環境変数を一元管理する
- Doppler必須repositoryではCodexにDoppler service token `dev-agent` だけを渡す
- 作業状態をCLI、tmux、GitHub Issue上で確認できるようにする
- 作業後はdraft PRを作成する
- agent自体はGitHub Releases経由で更新する
- Campaign Issueから大きめtaskを子Issue化し、taskごとにPRを作る
- 同一repository内の複数Issueを古いIssue順に1件ずつ処理し、明確な依存があるIssueはstacked PRにする

## 初期CLI

最初から確認用コマンドと状態表示を持たせます。

```sh
run-weaver doctor
run-weaver doctor --json
run-weaver repo add
run-weaver repo add --doppler required
run-weaver repo list
run-weaver repo remove example/repo
run-weaver decision answer --repo example/repo '#7' decision-id approve
run-weaver status
run-weaver status --json
run-weaver status --repo example/repo
run-weaver install --target wsl
run-weaver install --target windows
run-weaver update --check
run-weaver daemon --target wsl
run-weaver daemon --target windows
run-weaver daemon --target wsl --once --repo-url https://github.com/example/repo.git
run-weaver daemon --target wsl --repo-url https://github.com/example/repo.git --poll-interval 1m
```

将来的に、ブラウザから状態確認するための `run-weaver dashboard` を追加します。

## 基本フロー

1. 人間がGitHub Issueを作る
2. 対象repository内で `run-weaver repo add` を実行し、監視対象として登録する
3. 人間が対象Issueに `run-weaver:ready` ラベルを付ける。Campaignの場合は `run-weaver:campaign` も付ける
4. `run-weaver daemon` が登録済みrepositoryの対象Issueを検出する
5. Issueに `running` ラベルを付け、claim ID付きの開始コメントを投稿する
6. 最新claimコメントを再確認し、勝ったagentだけがローカルstate fileにclaimを保存する
7. Codex専用cloneからIssue専用worktreeを作る
8. repositoryのDoppler要否を判定し、必須ならDoppler経由で環境変数を注入する
9. WSL側ではtmux session、Windows側ではdirect runnerでCodex CLIをworktree上で実行する
10. 作業後、draft PR作成前に最新baseを取り込み、競合があればCodexで1回だけ解消を試す
11. 競合解消と検証を通過した場合だけdraft PRを作成する
12. Issueに結果コメントを投稿し、`done` または `blocked` ラベルへ更新する

Codex promptでは、repositoryの `AGENTS.md` が禁止しておらず、上位の実行時指示が許可し、Codex実行環境が提供している場合、調査、レビュー、委譲可能な小タスクにCodex built-in subagentsを使うよう指示します。利用不可または禁止されている場合はセルフレビューで続行します。

Codexがrate limitで中断した場合、`run-weaver` はIssueを `blocked` にせず `running` のまま自動resumeを試します。このときIssueにはresume attempt番号、session、worktree、JSONLログpath、検出時刻だけを含む中間コメントを投稿し、secret値やログ本文は載せません。

draft PR作成前のbase取り込みで通常のmerge conflictが出た場合、`run-weaver` は同じworktree上でCodexを `conflict-resolve` phaseとして1回だけ起動します。未解消のconflict marker、unmerged file、`git diff --check` の失敗が残る場合、またはlockfile、GitHub Actions workflow、migrationなど高リスクな競合を検出した場合はPRを作らず `blocked` にします。

Campaign Issueでは、親Issue本文の大枠指示とrepository内docsをCodex Plannerが読み、PR単位のtask graphとdecision gateをJSONで生成します。Plannerはrepo docsを優先し、親Issue本文は「どのroadmapを進めるか」を伝える補助入力として扱います。Plannerが作った通常taskは `run-weaver:campaign-task` 付きの子Issueとして作成し、通常ready Issue取得からは除外します。人間判断が必要な箇所だけDecision Requestとして親Issueへコメントします。Decision Requestには判断理由、証拠、選択肢ごとの詳細、影響、可逆性を含め、secret値やログ本文は載せません。PCでは `run-weaver status` に表示される `run-weaver decision answer --repo <owner/repo> '#<campaign-issue>' <decision-id> <option>` で回答できます。外出先ではDecision Request内のquick replyから1行を選び、そのままGitHub Issueへ新規コメントとして貼り付けます。daemonは親Issueコメントの `run-weaver-decision:<decision-id>:<option>` markerを読み取り、optionがDecision Requestの選択肢に含まれる場合だけstateへ保存します。Dispatcherは回答済みdecisionの内容をtask promptへ渡し、子Issueを `plan`、`implement`、`review`、`verify` の順に処理してtaskごとにdraft PRを作成します。

通常Issueが同じrepositoryに複数ある場合、daemonはIssue番号の小さい順に最大1件ずつ処理します。Issue本文、タイトル、人間コメントに `depends: #123`、`blocked by #123`、`stacked on #123`、`依存: #123`、`#123 の後` などの明確な依存があれば、依存先Issueが `done` になりPR branchを復元できるまで待機します。依存先が完了済みなら、そのbranchをbaseにしたstacked draft PRを作ります。依存表現が曖昧なIssueは `blocked` にし、次の実行可能Issueへ進みます。

## 状態確認

`run-weaver status` は、常駐状態と現在のジョブを表示します。複数repositoryを登録している場合はrepositoryごとのstateを集約し、`--repo owner/repo` で対象repositoryだけを表示します。

- agent process
- target
- current issue
- current worktree
- tmux session name
- local state file path
- last state comment timestamp
- reconciliation result
- campaign progress。Campaign実行中のみ
- ready queue。通常Issueの待ち行列と依存状態

WSLではtmuxで実行ログを確認できます。

```sh
tmux attach -t run-weaver
```

WSLのsystemd user serviceには、`run-weaver install --target wsl` 実行時のPATHを保存します。nvmなどユーザーPATH上にある `codex` を導入した後は、再度 `run-weaver install --target wsl` を実行してserviceを更新します。

## 依存関係確認

`run-weaver doctor` は、実行環境が作業可能かを確認します。

- `git`
- `gh`
- `codex`
- `doppler`。Doppler必須repositoryのみ必須
- `tmux`。WSL targetのみ必須
- `systemctl --user`。WSL targetのみ必須
- GitHub認証状態
- Doppler必須repositoryでのDoppler認証またはtoken設定
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

GitHub Releasesから取得するため、ローカルにこのプロジェクトをcloneしておく必要はありません。`install` は常駐設定を作成しますが、事前に `git`、`gh`、`codex` の導入と認証、対象Issueの `run-weaver:ready` ラベルが必要です。Campaign Issueを使う場合は親Issueに `run-weaver:campaign` も付けます。対象repository内で `run-weaver repo add` を実行すると、Codex専用clone用のrepository URLは `git remote get-url origin` から登録されます。Doppler要否は既定で `auto` になり、repo rootの `doppler.yaml`、`doppler.yml`、`.doppler.yaml`、`.doppler.yml` を検出した場合だけ必須扱いにします。誤検出を避けたい場合は `run-weaver repo add --doppler required` または `--doppler optional` で明示します。secret値はコマンドやドキュメントに書かず、Doppler service token `dev-agent` は環境変数で渡します。

WSL:

```sh
curl -fsSL https://raw.githubusercontent.com/ota-takeru/run-weaver/main/scripts/install.sh | sh -s -- \
  --target wsl
~/.local/bin/run-weaver doctor --target wsl
```

Windows PowerShell:

```powershell
iwr https://raw.githubusercontent.com/ota-takeru/run-weaver/main/scripts/install.ps1 -OutFile $env:TEMP\install-run-weaver.ps1
powershell -ExecutionPolicy Bypass -File $env:TEMP\install-run-weaver.ps1
& "$env:LOCALAPPDATA\run-weaver\bin\run-weaver.exe" doctor --target windows
```

Windowsの `install` はper-user Task Scheduler task `run-weaver` を作成または更新します。daemonの標準出力と標準エラーは `%LOCALAPPDATA%\run-weaver\logs\daemon.log` に追記されます。

release buildの `run-weaver daemon` は起動時にGitHub Releasesのlatest releaseを確認し、新しいassetがあれば自動更新してから処理を続けます。手動更新は `run-weaver update` で実行できます。手動更新時はdaemon更新要求をstate rootの `update-request.json` に残し、継続中daemonは次のpoll安全地点で自己更新・再起動します。実行中のCodex sessionやtmux windowは停止せず、再起動後daemonがstate fileとログを再照合します。インストールscriptは常にlatest releaseを取得します。開発ビルドはversionが `dev` のため自動更新しません。更新を一時的に止める場合は `RUN_WEAVER_NO_UPDATE=1` を設定します。

## Maintainer向けリリース

リリース作成は利用者向け `run-weaver` CLIではなく、このrepositoryのmaintainer用scriptで行います。既定はdry-runで、tag作成やpushは行いません。

```sh
scripts/release.sh
scripts/release.sh --bump minor
scripts/release.sh --version v0.1.0
scripts/release.sh --push
scripts/release.sh --push --watch
```

`scripts/release.sh` はremote tagから次の `vMAJOR.MINOR.PATCH` を計算します。tagがまだ無い場合の初回は `v0.1.0`、既定の採番はpatch bumpです。事前検証では `go test ./...` とLinux / Windows、amd64 / arm64のrelease cross-buildを実行します。`--push` 指定時だけannotated tagを作成してpushし、既存のGitHub Actions release workflowがGitHub Releaseとassetを作成します。`--push --watch` ではtag push後にrelease workflow完了まで待ち、4つのrelease assetが作成されたことも確認します。

## 設計文書

- [Architecture](docs/architecture.md)
- [CLI](docs/cli.md)
- [GitHub Issue Flow](docs/github-issue-flow.md)
