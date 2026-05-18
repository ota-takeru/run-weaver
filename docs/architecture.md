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

daemonは通常、`run-weaver repo add` で登録したrepository一覧を `repos.json` から読みます。継続pollではpollごとに `repos.json` を読み直すため、登録追加後もdaemon再起動なしで次回pollから対象になります。repoごとに `gh --repo <owner/repo>`、repo専用clone、repo専用state fileを使い、repo内では常に最大1 jobだけ実行し、repo間では同時実行します。`--repo-url` と `--repo` を指定した場合は互換用の単一repoモードとして動きます。`--once` は登録済みrepositoryを1回ずつ確認し、各repoで最大1件開始します。

同一repository内に通常Issueが複数ある場合は、Issue番号の小さい順に候補を評価します。Issueタイトル、本文、人間コメントから `depends: #123`、`depends on #123`、`blocked by #123`、`after #123`、`stacked on #123`、`依存: #123`、`#123 の後` などを検出した場合、その依存先Issueが `done` で、PR URLとbranchをstate fileまたは完了コメントから復元できるまで待機します。依存が解決済みなら依存先branchをbaseにしたstacked PRとして開始します。依存表現があるがIssue番号が読めない場合、対象Issueを `blocked` にして次の実行可能Issueを探します。

初期実装ではGitHub操作に `gh` CLIを使います。これにより、既存のGitHub認証状態をそのまま利用できます。

Issueを取得したagentは、`running` ラベル付与、開始コメント、ローカルstate file更新をclaimとして扱います。別targetが同じIssueを処理しようとした場合は、GitHub上の `running` ラベルと最新のclaimコメントを再確認し、既に処理中ならスキップします。

### Campaign Planner / Dispatcher

Campaign PlannerはCodexを非同期に起動し、親Campaign Issue本文の大枠指示とrepository内docsからPR単位のtask graphを生成します。入力源はrepo docs優先で、親Issue本文は進めたいroadmapや目的を伝える補助入力です。Plannerの最終応答はJSONだけを正とし、`tasks[]` と `decisions[]` をstateへ取り込みます。Plannerが有効なJSON planを返せない場合は親Campaignを `blocked` にします。子Issue本文には親Campaign番号とtask IDを含め、子Issueには `run-weaver:campaign-task` を付けて通常task取得から除外します。

Dispatcherはローカルstate fileの `campaign` を正本にして、依存関係が解決済みの次taskを選びます。taskごとに既存のclaim、worktree、tmux、Codex、draft PR flowを再利用します。Campaign taskは同じworktree上で `plan`、`implement`、`review`、`verify` の順にCodexを起動し、`verify` 完了後だけcommit、push、draft PR作成へ進みます。task開始時またはphase進行時にworktree、Doppler、prompt、runnerで失敗した場合は、子Issue、Campaign task、Campaign status、state jobを `blocked` に揃えます。task完了後はPR URLをCampaign stateへ記録し、次taskへ進めます。

Decision Requestには `options`、`recommendation`、`blocked tasks`、`can continue tasks` を含めます。人間は `run-weaver-decision:<decision-id>:<option>` を含むコメントで回答します。pending decisionがある間は `can continue tasks` に含まれるtaskだけを実行し、実行可能taskがないdecision gateではCampaignを `decision_required` として停止し、回答を読み取ったらstateへ保存して再開します。task dependencyは `depends: task-...` のtask ID形式を正とします。

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

通常Issueは原則としてIssueごとに独立branchとdraft PRを作ります。依存が明確なIssueだけ、直前依存Issueのbranchを `git worktree add` のbase refとdraft PRのbase branchに使います。

複数repository登録時はcloneとworktreeをrepositoryごとに分離します。cloneは `<dataRoot>/repos/<repo-slug>/src`、worktreeは `<dataRoot>/repos/<repo-slug>/worktrees/issue-<number>` を使います。

### Secret Manager

Dopplerが必要なrepositoryでは、DB認証情報や環境変数をDopplerで一元管理します。CodexにはDoppler service token `dev-agent` だけを渡します。

repo設定の `dopplerMode` は `auto`、`required`、`optional` のいずれかです。`auto` ではrepo rootの `doppler.yaml`、`doppler.yml`、`.doppler.yaml`、`.doppler.yml` を検出した場合だけDoppler必須とします。必須repositoryでDoppler CLIや `doppler run` 可能な認証/設定がない場合、agentはCodex起動前にIssueを `blocked` にします。Doppler設定ファイルがあるだけでは認証済みとは扱いません。Doppler不要repositoryではDoppler未インストールでも通常の `codex exec` で進めます。service tokenやsecretの実体をログ、Issueコメント、PR本文には出しません。

### Runner

Codex CLIはworktree上で起動します。

WSL targetではtmux sessionを使い、作業ログを人間が確認できるようにします。Windows targetではtmuxを使わず、Task Schedulerから起動したdaemonがdirect runnerでCodexを非同期起動します。

```sh
tmux attach -t run-weaver
```

tmux window名はIssue番号を含めます。

初期実装ではCodexを非対話モードの `codex exec` で起動します。worktreeは `--cd` で指定し、JSONLログと最終応答はstate配下のIssue別ディレクトリへ保存します。promptでは、repositoryの `AGENTS.md` が禁止しておらず、上位の実行時指示が許可し、Codex実行環境が提供している場合、調査、レビュー、委譲可能な小タスクにCodex built-in subagentsを使うよう指示します。利用不可または禁止されている場合はセルフレビューで続行します。終了コード `0` 以外、JSONLログの異常終了、draft PR作成失敗は `blocked` として扱います。WSL targetのsystemd user serviceにはinstall時のPATHを保存し、tmux window終了後のJSONLログが `codex: command not found` などCodex起動失敗を示す場合も `blocked` にします。この判定はshell起動失敗行またはerror/failure系JSONL eventに限定し、command outputやdocs本文の文言ではblockedにしません。

Codexがrate limitで中断した場合、agentはJSONLログからsession idを取得し、ローカルstateに保存します。次回pollでtmux windowが終了済みなら、Issueへrate limit検出と自動resume attemptの中間コメントを投稿してから、同じworktree上で `codex exec resume <session>` を起動し、前回sessionと作業ツリーを引き継ぎます。session idを取得できない場合は、同じworktree上で `codex exec resume --last` を試します。中間コメントにはattempt番号、session、worktree、JSONLログpath、検出時刻だけを含め、secretやログ本文は含めません。state fileには `retryCount`、`lastGitHubCommentAt`、rate limit resume中であることを示す `lastError` を残します。rate limit再開は人間確認条件を迂回しません。push、deploy、外部課金、外部アカウント設定変更、secret表示、破壊的操作、ADR矛盾が必要な場合は従来どおり人間判断に回します。

Codex完了の初期判定は、最終応答ファイルが存在し、対象tmux windowが終了していることです。完了後はbranchをpushし、draft PRを作成してIssueを `done` に更新します。

Doppler必須repositoryでは `doppler run -- codex ...` でCodexを起動します。agentはtokenの値をstate file、tmuxログ、構造化ログ、Issueコメント、PR本文に出しません。tmux上でもtoken値が表示されないよう、コマンドライン引数ではなく環境変数として注入します。

## 状態管理

状態は3つに絞ります。

- `running`: agentがIssueを処理中
- `done`: draft PR作成まで完了
- `blocked`: 認証、依存関係、Codex失敗、競合、人間確認待ちなどで停止

状態はGitHub Issueのラベルとコメントで返します。CLIの `status` でも同じ情報を確認できます。

ローカルの正本はstate fileです。単一repo互換の既定パスは以下です。

- WSL: `$XDG_STATE_HOME/run-weaver/state.json`。未設定なら `~/.local/state/run-weaver/state.json`
- Windows: `%LOCALAPPDATA%\\run-weaver\\state.json`

`status` はstate fileを主情報源にし、process、tmux、GitHub Issueを照合して表示します。state fileとGitHubの状態が矛盾する場合はGitHubの `running` / `done` / `blocked` ラベルを優先し、矛盾を `last error` に表示します。GitHub照合が認証切れなどで失敗しても、process、tmux、last-messageによるローカルruntime照合は続けます。

process照合はtargetごとに行います。WSLではPIDへsignal 0を送り、Windowsでは `tasklist` のPID検索結果を使います。tmux照合はWSL targetのみで行います。

Windows targetのdaemon標準出力と標準エラーは `%LOCALAPPDATA%\\run-weaver\\logs\\daemon.log` に追記します。issue別のCodex JSONLログとlast messageは `%LOCALAPPDATA%\\run-weaver\\issues\\<issue>\\` に保存します。

state fileには現在のtarget、Issue番号、branch、base branch、依存Issue、worktree、tmux session/window、claim時刻、最終コメント時刻、最終エラーを保存します。通常Issueの完了後は、後続stacked PRで参照できるように直近完了IssueのPR URLとbranchを `completedIssues` に保存します。Campaign実行中は親Issue、task graph、current task、completed tasks、decisions、PRs、retry count、pipeline phaseも `campaign` に保存します。secret値は保存しません。

複数repository登録時はrepositoryごとに `<stateRoot>/repos/<repo-slug>/state.json` を使います。`status` は `repos.json` を読んでrepo別stateを集約します。tmux session名は `run-weaver` のまま、window名は `<repo-slug>-issue-<number>` にしてIssue番号衝突を避けます。

state fileの最小schemaと `status --json` のトップレベル構造は `docs/cli.md` を正本にします。時刻はUTCのRFC 3339形式に統一し、処理中Issueがない場合は `job: null` とします。

## 更新方式

agent自体はGitHub Releases経由で配布します。

導入は以下の2段階を想定します。

- `install.sh` / `install.ps1`: GitHub Releasesのlatest asset取得と基本設定
- `run-weaver install`: targetごとの常駐設定

release assetは `.github/workflows/release.yml` がtag `v*` push時に作成します。maintainerは利用者向けCLIではなく `scripts/release.sh` で次tagのdry-run確認、事前検証、明示的なtag pushを行います。事前検証では `go test ./...` に加えて、Linux / Windows、amd64 / arm64のrelease cross-buildを行います。`--push --watch` ではtag push後にrelease workflow完了まで待ち、asset作成まで確認します。asset名はOS/ARCHごとに固定し、Linuxは `run-weaver_linux_<arch>.tar.gz`、Windowsは `run-weaver_windows_<arch>.zip` とします。release buildでは `-ldflags` で `internal/cli.Version` にtag名を埋め込みます。

`run-weaver daemon` は起動時にGitHub Releasesのlatest releaseを確認します。latestが現在versionより新しければ対応assetをdownloadし、現在の実行ファイルを置き換えます。Linux / WSLでは置換後に同じPIDで更新後binaryへexecします。Windowsでは実行中のexeを直接置換できないため、一時ファイルを置いてから現在processを終了し、`cmd` 経由で置換後に同じ引数で起動し直します。`install.sh` / `install.ps1` は常にlatest releaseを取得するため、`run-weaver install` 自体では自動更新を行いません。

開発ビルドのversionは `dev` であり、自動更新は行いません。運用上、自動更新を止める必要がある場合は `RUN_WEAVER_NO_UPDATE=1` を設定します。

## 将来拡張

`run-weaver dashboard` を追加し、localhost上で状態確認できるようにします。

初期段階ではdashboardを実装せず、CLI、tmux、GitHub Issueコメントを状態確認の主経路にします。
