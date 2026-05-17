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

GitHub Issueを定期的に監視し、実行対象を選びます。通常taskは `run-weaver:ready` ラベルが付いたopen Issueを対象にします。Campaignは `run-weaver:campaign` と `run-weaver:ready` の両方が付いたopen Issueを親Issueとして扱い、通常task取得からは除外します。対象Issueごとに専用worktreeを作成し、Codex CLIを実行します。

daemonは `--repo-url` でCodex専用clone作成元のrepository URLを受け取ります。未指定の場合はカレントディレクトリの `git remote get-url origin` から推定します。`--once` を指定した場合は1件だけ処理して終了し、指定しない場合は `--poll-interval` ごとに継続pollします。

初期実装ではGitHub操作に `gh` CLIを使います。これにより、既存のGitHub認証状態をそのまま利用できます。

Issueを取得したagentは、`running` ラベル付与、開始コメント、ローカルstate file更新をclaimとして扱います。別targetが同じIssueを処理しようとした場合は、GitHub上の `running` ラベルと最新のclaimコメントを再確認し、既に処理中ならスキップします。

### Campaign Planner / Dispatcher

Campaign Plannerは親Campaign Issue本文のMarkdown roadmapをtask graphへ変換します。箇条書きの通常項目は大きめの実行taskとして子Issueに作成し、`decision`、`gate`、`判断`、`決定` を含む項目はdecision gateとして親Campaign IssueへDecision Requestコメントを投稿します。子Issue本文には親Campaign番号とtask IDを含めます。

Dispatcherはローカルstate fileの `campaign` を正本にして、依存関係が解決済みの次taskを選びます。taskごとに既存のclaim、worktree、tmux、Codex、draft PR flowを再利用します。Campaign taskは同じworktree上で `plan`、`implement`、`review`、`verify` の順にCodexを起動し、`verify` 完了後だけcommit、push、draft PR作成へ進みます。task完了後はPR URLをCampaign stateへ記録し、次taskへ進めます。

Decision Requestには `options`、`recommendation`、`blocked tasks`、`can continue tasks` を含めます。人間は `run-weaver-decision:<decision-id>:<option>` を含むコメントで回答します。実行可能taskがないdecision gateではCampaignを `decision_required` として停止し、回答を読み取ったらstateへ保存して再開します。

### Target Adapter

targetごとのOS差分を扱います。

- `wsl`: systemd user serviceで常駐し、Codex実行ログをtmux sessionに残す
- `windows`: per-user Task Schedulerで常駐し、Windows側のログ出力先を使う

`--target` は必須にし、agentが暗黙に環境を推測して破壊的な操作をしないようにします。

WSL targetでは、`doctor` が `systemctl --user` の利用可否、systemd user managerの稼働状態、service file配置先の書き込み可否、`tmux` の有無を確認します。systemd user managerが使えない環境は初期実装では `blocked` とし、別常駐方式への自動フォールバックは行いません。

### Worktree Manager

Codexは人間用 `src` を直接触りません。Codex専用cloneを作り、Issueごとに専用worktreeを作成します。

想定する作業単位:

- Codex専用clone: agentが管理する永続clone
- Issue専用worktree: `issue-<number>` 単位で作成
- branch: `codex/issue-<number>-<slug>` のようにIssue由来で作成

完了後はdraft PRを作成し、Issueにリンクを返します。

### Secret Manager

DB認証情報や環境変数はDopplerで一元管理します。CodexにはDoppler service token `dev-agent` だけを渡します。

agentはCodex起動時にDoppler経由で必要な環境変数を注入します。service tokenやsecretの実体をログ、Issueコメント、PR本文には出しません。

### Runner

Codex CLIはworktree上で起動します。

WSL targetではtmux sessionを使い、作業ログを人間が確認できるようにします。

```sh
tmux attach -t run-weaver
```

tmux window名はIssue番号を含めます。

初期実装ではCodexを非対話モードの `codex exec` で起動します。worktreeは `--cd` で指定し、JSONLログと最終応答はstate配下のIssue別ディレクトリへ保存します。終了コード `0` 以外、JSONLログの異常終了、draft PR作成失敗は `blocked` として扱います。

Codexがrate limitで中断した場合、agentはJSONLログからsession idを取得し、ローカルstateに保存します。次回pollでtmux windowが終了済みなら、同じworktree上で `codex exec resume <session>` を起動し、前回sessionと作業ツリーを引き継ぎます。session idを取得できない場合は、同じworktree上で `codex exec resume --last` を試します。rate limit再開は人間確認条件を迂回しません。push、deploy、外部課金、外部アカウント設定変更、secret表示、破壊的操作、ADR矛盾が必要な場合は従来どおり人間判断に回します。

Codex完了の初期判定は、最終応答ファイルが存在し、対象tmux windowが終了していることです。完了後はbranchをpushし、draft PRを作成してIssueを `done` に更新します。

CodexにはDoppler service token `dev-agent` を環境変数経由で渡します。agentはtokenの値をstate file、tmuxログ、構造化ログ、Issueコメント、PR本文に出しません。tmux上でもtoken値が表示されないよう、コマンドライン引数ではなく環境変数として注入します。

## 状態管理

状態は3つに絞ります。

- `running`: agentがIssueを処理中
- `done`: draft PR作成まで完了
- `blocked`: 認証、依存関係、Codex失敗、競合、人間確認待ちなどで停止

状態はGitHub Issueのラベルとコメントで返します。CLIの `status` でも同じ情報を確認できます。

ローカルの正本はstate fileです。既定パスは以下です。

- WSL: `$XDG_STATE_HOME/run-weaver/state.json`。未設定なら `~/.local/state/run-weaver/state.json`
- Windows: `%LOCALAPPDATA%\\run-weaver\\state.json`

`status` はstate fileを主情報源にし、process、tmux、GitHub Issueを照合して表示します。state fileとGitHubの状態が矛盾する場合はGitHubの `running` / `done` / `blocked` ラベルを優先し、矛盾を `last error` に表示します。

process照合はtargetごとに行います。WSLではPIDへsignal 0を送り、Windowsでは `tasklist` のPID検索結果を使います。tmux照合はWSL targetのみで行います。

Windows targetのdaemon標準出力と標準エラーは `%LOCALAPPDATA%\\run-weaver\\logs\\daemon.log` に追記します。issue別のCodex JSONLログとlast messageは `%LOCALAPPDATA%\\run-weaver\\issues\\<issue>\\` に保存します。

state fileには現在のtarget、Issue番号、branch、worktree、tmux session/window、claim時刻、最終コメント時刻、最終エラーを保存します。Campaign実行中は親Issue、task graph、current task、completed tasks、decisions、PRs、retry count、pipeline phaseも `campaign` に保存します。secret値は保存しません。

state fileの最小schemaと `status --json` のトップレベル構造は `docs/cli.md` を正本にします。時刻はUTCのRFC 3339形式に統一し、処理中Issueがない場合は `job: null` とします。

## 更新方式

agent自体はGitHub Releases経由で配布します。

導入は以下の2段階を想定します。

- `install.sh` / `install.ps1`: GitHub Releasesのlatest asset取得と基本設定
- `run-weaver install`: targetごとの常駐設定

release assetは `.github/workflows/release.yml` がtag `v*` push時に作成します。asset名はOS/ARCHごとに固定し、Linuxは `run-weaver_linux_<arch>.tar.gz`、Windowsは `run-weaver_windows_<arch>.zip` とします。release buildでは `-ldflags` で `internal/cli.Version` にtag名を埋め込みます。

`run-weaver daemon` は起動時にGitHub Releasesのlatest releaseを確認します。latestが現在versionより新しければ対応assetをdownloadし、現在の実行ファイルを置き換えます。Linux / WSLでは置換後に同じPIDで更新後binaryへexecします。Windowsでは実行中のexeを直接置換できないため、一時ファイルを置いてから現在processを終了し、`cmd` 経由で置換後に同じ引数で起動し直します。`install.sh` / `install.ps1` は常にlatest releaseを取得するため、`run-weaver install` 自体では自動更新を行いません。

開発ビルドのversionは `dev` であり、自動更新は行いません。運用上、自動更新を止める必要がある場合は `RUN_WEAVER_NO_UPDATE=1` を設定します。

## 将来拡張

`run-weaver dashboard` を追加し、localhost上で状態確認できるようにします。

初期段階ではdashboardを実装せず、CLI、tmux、GitHub Issueコメントを状態確認の主経路にします。
