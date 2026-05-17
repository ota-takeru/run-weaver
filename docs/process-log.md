# Process Log

## 2026-05-18 - Release Readiness Audit

Review:

- Immediate fixes:
  - Doppler必須repositoryで `doppler.yaml` などの設定ファイルがあるだけで認証OK扱いになる穴を修正した。
  - 必須repositoryではCodex起動前に `doppler run -- git --version` を実行し、失敗時は `auth_required` / `blocked` として止めるようにした。
  - Doppler事前検査の単体テストを追加し、CLI / architecture / progress / handoffを更新した。
  - `go test ./...`、`go test -race ./...`、`go vet ./...`、Linux / Windows amd64 / arm64のrelease相当cross-buildを確認した。
  - latest release v0.1.2のasset取得、release binaryの `--help`、`update --check` の最新/更新あり判定を確認した。
  - 最新push CIでWindows jobの `TestWSLServiceFile` が失敗したため、WSL service PATH生成をOS非依存でLinux形式に固定した。
  - direct runner系テストがWindows state pathを書き込む際に `LOCALAPPDATA` 未設定だとホーム配下へ漏れるテスト隔離不備を修正した。
- Future tasks:
  - 実GitHub Issueへのdaemon実行、Windows実機direct runner、Doppler必須repoの実blocked確認は、外部書き込みや実機環境が必要なため継続する。
  - 次push後にGitHub Actions CIの再実行結果を確認する。
- Human-facing reports:
  - `ota-takeru/run-weaver#1` は `running` ラベル付きだがtmux windowがなく、JSONLに `codex: command not found` が残っている。更新後daemon pollで `blocked` へ寄せるか、人間が `running` を外して再投入する必要がある。
  - latest release v0.1.2のasset自体は取得・起動できるが、今回のDoppler修正は未リリースのため、次releaseへ含める必要がある。
  - 今回はtag push、GitHub Release作成、Issueラベル/コメント変更、draft PR作成、secret表示、外部アカウント設定変更は行っていない。

## 2026-05-17 - WSL Codex PATH Startup Handling

Review:

- Immediate fixes:
  - `run-weaver install --target wsl` がsystemd user serviceへ実行時PATHを `Environment=PATH=...` として保存するようにした。
  - tmux window終了後のJSONLログが `codex: command not found` などCodex起動失敗を示す場合、daemonがIssue/stateを `blocked` に更新するようにした。
  - README、architecture、CLI docs、progress、handoffにWSL service PATHとCodex startup失敗時の扱いを反映した。
  - `go test ./internal/cli` と `go test ./...` で確認した。
- Future tasks:
  - nvmやNode更新で `codex` のPATHが変わった場合は、`run-weaver install --target wsl` の再実行を運用手順として扱う。
- Human-facing reports:
  - 既に stuck しているIssueは、更新後daemonのpollで `blocked` へ寄せるか、人間が `running` を外して再投入する必要がある。

## 2026-05-17 - Release Watch Flow

Review:

- Immediate fixes:
  - `scripts/release.sh` に `--watch` を追加し、`--push --watch` でtag push後にrelease workflow完了まで待てるようにした。
  - workflow完了後、GitHub ReleaseにLinux / Windows、amd64 / arm64の4 assetが揃っていることを確認する処理を追加した。
  - README、architecture、CLI docs、progress、handoffにmaintainer release flowの `--push --watch` を反映した。
- Future tasks:
  - 実release workflowの成功確認は、人間が `scripts/release.sh --push --watch` を明示実行した後に行う。
- Human-facing reports:
  - `--watch` は `--push` 必須で、dry-runではtag作成、push、GitHub Release作成を行わない。
  - 今回は実tag作成、push、GitHub Release作成、外部アカウント設定、secretには触れていない。

## 2026-05-17 - Repo Registration Status Error Fix

Review:

- Immediate fixes:
  - `run-weaver repo add` がGit repository外で失敗した場合、`git remote get-url origin` の失敗であることとGit stderrを表示するようにした。
  - legacy単一repo stateを読む `run-weaver status` で、state内のrepositoryまたはIssue URLから `owner/repo` を推定し、`gh --repo` を付けてGitHub照合するようにした。
  - 複数repository表示でrepo別stateが未作成の場合、別repositoryのlegacy単一repo stateを誤って表示しないようにした。
  - Issue URLからrepositoryを推定する単体テストと、`repo add` のエラー文言テストを追加した。
- Future tasks:
  - `run-weaver repo add` は引き続き対象repository内で実行する運用。必要になれば、将来 `repo add --repo-url` 形式を検討する。
- Human-facing reports:
  - ユーザーが `~` から `run-weaver repo add` を実行すると、対象repositoryの `origin` が読めないため失敗する。対象repositoryへ `cd` してから実行するか、既存state確認は `run-weaver status --repo owner/repo` を使う。

## 2026-05-17 - Codex Planner Campaign E2E

Review:

- Immediate fixes:
  - `ota-takeru/truth-table-app#12` でE2E用 `docs/roadmap.md` を追加し、Campaign Plannerがrepo docsを優先して読める状態にした。
  - `ota-takeru/truth-table-app#13` を親Issue本文「roadmap進めてください」形式のCampaignとして作成し、Codex Plannerが `docs/roadmap.md` を読んで3 task + 1 decisionのJSON graphを生成することを確認した。
  - Planner完了後に子Issue #14 / #15 / #16 とDecision Requestが作成され、state上のCampaign progressにtask / decision / planner情報が保存されることを確認した。
  - Decision未回答の状態で継続可能task #14 / #15 が順に実行され、draft PR #17 / #18 が作成されることを確認した。
  - Campaignが `decision_required` で停止した後、`run-weaver-decision:export-feature-scope:approve` コメントで再開し、blocked task #16 からdraft PR #19 が作成され、Campaignが `done` になることを確認した。
- Future tasks:
  - 複数repository登録後の実GitHub Issue処理を確認する。
  - Windows target direct runnerを実機で確認する。
  - tag push後にrelease workflowとGitHub Release assetを確認する。
  - Doppler必須repoの実blocked確認を、secret値を出さずに検証できる対象repoで行う。
- Human-facing reports:
  - `ota-takeru/truth-table-app` にはE2E用PR #12、Campaign #13、子Issue #14 / #15 / #16、draft PR #17 / #18 / #19 を作成済み。
  - #17 / #18 / #19 はdraft PRのまま残している。
  - secret値、外部アカウント設定、release tag push、GitHub Release作成は行っていない。

## 2026-05-17 - Codex Driven Campaign Planner

Review:

- Immediate fixes:
  - Campaign開始時に子Issueを即作成せず、Codex PlannerをCampaign用worktreeで非同期起動するようにした。
  - Planner promptはrepo docsとroadmap/progress/handoff系ファイルを優先して読ませ、親Issue本文を補助入力として扱うようにした。
  - Planner最終応答のJSON `tasks[]` / `decisions[]` を検証し、通過後だけ子IssueとDecision Requestを作成するようにした。
  - 不正JSON、空task、重複task ID、未知dependency、未知task参照decision、Planner state欠落、WSLでPlanner終了後にlast-messageが無い場合は親Campaignを `blocked` にするようにした。
  - `status` のhuman / JSON outputに `campaign.status: planning` とPlanner tmux/log情報を表示するようにした。
  - README、architecture、CLI docs、GitHub Issue flow、decision log、progress、handoffをCodex主導Planner仕様へ更新した。
- Future tasks:
  - `ota-takeru/truth-table-app` で「roadmap進めてください」形式の実GitHub E2Eを行う。
  - 複数repository登録後の実GitHub Issue処理、Windows direct runner、release workflow、Doppler必須blocked確認を継続する。
- Human-facing reports:
  - Planner生成taskは人間承認なしで子Issue化される。人間判断が必要な箇所だけDecision Requestで止まる。
  - 今回は実GitHub Issue、push、release tag、secret、外部アカウント設定には触れていない。

## 2026-05-17 - Truth Table App E2E And Campaign Context Fix

Review:

- Immediate fixes:
  - `ota-takeru/truth-table-app#3` / `#4` を作成し、同一repository内のIssue番号順処理、依存待機、stacked draft PR作成を実GitHubで確認した。
  - #3 はdraft PR #5を `main` baseで作成し、#4 は依存先 #3 のbranchをbaseにしたdraft PR #6として作成された。
  - `ota-takeru/truth-table-app#7` をCampaignとして作成し、子Issue #8 / #9、Decision Request、`plan` / `implement` / `review` / `verify` pipeline、draft PR #10 / #11、Campaign progress、decision gate停止/再開を確認した。
  - Campaign子Issue #8 / #9 が `run-weaver:campaign-task` 付きで通常ready Issue取得から除外され、Campaign完了後のdaemonが `no ready issue` になることを確認した。
  - E2E中に、Campaign task promptがroadmapの1行だけを渡して親Campaign本文の詳細指示を落とす問題を発見した。task bodyへ親Campaign本文contextを含めるよう修正し、単体テストを追加した。
  - Campaign planning時に既存stateの `completedIssues` を落とさないよう修正し、単体テストを追加した。
- Future tasks:
  - 複数repository登録後の実GitHub Issue処理を確認する。
  - Windows target direct runnerを実機で確認する。
  - tag push後にrelease workflowとGitHub Release assetを確認する。
  - Doppler必須repoでの実blocked確認を、secret値を出さずに検証できる対象repoで行う。
- Human-facing reports:
  - `ota-takeru/truth-table-app` にはE2E用Issue #3 / #4 / #7 / #8 / #9 とdraft PR #5 / #6 / #10 / #11 が作成済み。
  - #10 / #11 は修正前のCampaign context不足を含むE2Eで作られたため、task詳細どおりのdocs追加ではなく `index.html` 変更になっている。原因はこの作業内で修正済み。
  - secret値、外部アカウント設定、release tag push、GitHub Release作成は行っていない。

## 2026-05-17 - Release Preflight Cross-Build

Review:

- Immediate fixes:
  - `scripts/release.sh` の事前検証にLinux / Windows、amd64 / arm64のrelease cross-buildを追加した。
  - cross-buildではrelease workflowと同じ `-ldflags` で `internal/cli.Version` に予定tagを埋め込む。
  - README、architecture、CLI docs、progress、handoffにrelease preflightの内容を反映した。
- Future tasks:
  - 実release workflowの成功確認は、人間が `scripts/release.sh --push` を明示実行した後に行う。
  - release workflowのarchive作成とasset uploadは引き続きGitHub Actions上で確認する。
- Human-facing reports:
  - 今回は実tag作成、push、GitHub Release作成、外部アカウント設定、secretには触れていない。

## 2026-05-17 - Maintainer Release Script

Review:

- Immediate fixes:
  - 利用者向け `run-weaver` CLIにはrelease commandを追加せず、maintainer用 `scripts/release.sh` を追加した。
  - scriptはremote tagから次の `vMAJOR.MINOR.PATCH` を計算し、既定patch bump、初回 `v0.1.0`、`--version` 明示指定に対応する。
  - dry-run既定で、`--push` 指定時だけannotated tagを作成してpushし、既存release workflowを起動するようにした。
  - repo専用remote、clean worktree、branch、remote HEAD、tag重複、`gh auth status`、`go test ./...` を事前検証する。
  - README、architecture、CLI docs、progress、handoffにmaintainer release flowを反映した。
- Future tasks:
  - 実release workflowの成功確認は、人間が `scripts/release.sh --push` を明示実行した後に行う。
  - Windows maintainer向けrelease scriptが必要になった場合は、PowerShell版を追加する。
- Human-facing reports:
  - 今回は実tag作成、push、GitHub Release作成、外部アカウント設定、secretには触れていない。

## 2026-05-17 - Same Repository Issue Queue

Review:

- Immediate fixes:
  - 通常ready Issue取得結果をIssue番号昇順で扱い、同一repository内では実行中jobがある間に別Issueを開始しないようにした。
  - 通常Issueのタイトル、本文、人間コメントから `depends: #123`、`blocked by #123`、`stacked on #123`、`依存: #123`、`#123 の後` などを検出する依存resolverを追加した。
  - 依存先が未完了なら待機扱いにして次候補へ進み、曖昧な依存やPR branch / URL不足は対象Issueだけ `blocked` にするようにした。
  - 依存解決済みIssueでは依存先branchをbaseにworktreeを作り、draft PRも同じbase branchを指定するstacked PRにした。
  - state schemaを `3` に上げ、通常Issueの `baseBranch`、`dependencies`、`completedIssues` を保存するようにした。
  - `status` のhuman / JSON outputに `readyQueue` を追加した。
  - ready queue、stacked PR、schema互換、同一repo実行中job保護の単体テストを追加した。
- Future tasks:
  - 実GitHub Issueで複数ready Issue、依存待機、stacked PR作成を確認する。
  - 実運用で依存表現の揺れが足りなければ、決定的パターンを追加する。
  - stale `running` 自動解除は従来どおり未実装で、人間判断に残す。
- Human-facing reports:
  - stacked PRは依存先PRがdraft/openで存在し、branchを復元できる場合だけ作る。
  - 今回は実GitHub Issue、push、draft PR、外部アカウント、secretには触れていない。

## 2026-05-17 - Doppler Optional Projects And Runner Hardening

Review:

- Immediate fixes:
  - repo設定に `dopplerMode` を追加し、`auto` / `required` / `optional` を `run-weaver repo add --doppler` で保存できるようにした。
  - 既存repo設定は `dopplerMode` 未指定なら `auto` として読み込み、secretやtoken値は保存しない方針を維持した。
  - `auto` ではrepo root直下の `doppler.yaml`、`doppler.yml`、`.doppler.yaml`、`.doppler.yml` でDoppler必須性を判定するようにした。
  - `doctor --repo` とdaemonがDoppler必須repositoryだけDoppler CLI / 認証を必須扱いにし、不要repositoryではDopplerなしで続行できるようにした。
  - Doppler必須repositoryでは `doppler run -- codex ...`、不要repositoryでは通常の `codex ...` を使うようにした。
  - Windows targetのCodex起動をtmuxではなくdirect runnerに分岐した。
  - Campaign子Issueに `run-weaver:campaign-task` を付け、通常ready Issue取得から除外するようにした。
  - pending decision gateがある場合、`can continue tasks` に含まれないtaskを実行せず `decision_required` にするようにした。
  - repo別rate limit resumeで `<repo-slug>-issue-<number>` window名を維持するようにした。
- Future tasks:
  - 実GitHub Campaign IssueでPlanner / DispatcherとDoppler auto判定の統合テストを行う。
  - Windows targetのdirect runnerを実機で確認する。
  - Doppler設定ファイルがsubdirectoryにあるrepoが必要になった場合、auto検出範囲を再検討する。
- Human-facing reports:
  - Doppler必須repoは `run-weaver repo add --doppler required` で明示できる。Doppler不要repoは `--doppler optional` で自動検出を上書きできる。
  - 今回は実GitHub Issue、push、draft PR、外部アカウント、secretには触れていない。

## 2026-05-17 - Multi Repository Support

Review:

- Immediate fixes:
  - `run-weaver repo add/list/remove` を追加し、カレントGitHub repositoryを `repos.json` に登録できるようにした。
  - repo別state、clone、worktree、issue log pathを追加し、既存単一repo stateは読み取り互換として残した。
  - daemonが登録済みrepositoryを読み、repoごとに独立した処理ループと `gh --repo` で実行できるようにした。
  - 常駐daemonがpollごとに `repos.json` を読み直し、`repo add` 後のrepositoryを再起動なしで拾えるようにした。
  - `status` が登録済みrepositoryを集約表示し、`--repo owner/repo` で対象repoだけを表示できるようにした。
  - tmux window名をrepo別実行では `<repo-slug>-issue-<number>` にしてIssue番号衝突を避けるようにした。
  - 複数repo同時開始時のtmux session作成競合を再確認で吸収するようにした。
  - repo登録、repo別path、repo別state、status集約、install互換の単体テストを追加した。
- Future tasks:
  - 複数repository登録後の実GitHub Issue処理を確認する。
  - 実GitHub Campaign IssueでPlanner / Dispatcherの統合テストを行う。
  - repo設定のdisable/enable CLIやrepoごとの検証コマンド設定は必要になった時点で検討する。
- Human-facing reports:
  - `run-weaver repo remove` は設定から外すだけで、clone、worktree、state fileは削除しない。
  - 実GitHub Issue、push、draft PR、外部アカウント、secretには触れていない。

## 2026-05-17 - Campaign Planner / Dispatcher

Review:

- Immediate fixes:
  - `run-weaver:campaign` + `run-weaver:ready` のCampaign Issue検出を追加した。
  - 通常ready Issue取得からCampaign Issueを除外し、誤claimを避けるようにした。
  - Campaign PlannerでMarkdown roadmapからtask graphとdecision gateを抽出し、子Issue作成とDecision Request投稿へ接続した。
  - Campaign Dispatcherで次task選択、`plan` / `implement` / `review` / `verify` phase進行、task完了時のPR URL記録を実装した。
  - Decision Requestへの `run-weaver-decision:<decision-id>:<option>` コメントを読み取り、Campaign stateへ保存して再開できるようにした。
  - state schemaを `2` に上げ、schema `1` は読み込み互換扱いにした。
  - `status` のhuman / JSON outputにCampaign progressを追加した。
  - Planner、GitHub filter、Dispatcher、state互換、status Campaign表示の単体テストを追加した。
- Future tasks:
  - 実GitHub Campaign IssueでPlanner / Dispatcherの統合テストを行う。
  - Decision Requestの回答形式を専用CLIにするか、コメント形式のまま運用するかを実運用後に見直す。
  - verify phaseをrepositoryごとの明示的な検証コマンド設定へ拡張する。
- Human-facing reports:
  - Campaign統合テストは子Issue、Issueコメント、branch、draft PRを実際に作るため、対象repository確認が必要。
  - secret、外部アカウント設定、pushは今回の手元実装では行っていない。

## 2026-05-17 - Current Repository Install Shortcut

Review:

- Immediate fixes:
  - `install` / `daemon` で `--repo-url` が未指定の場合、カレントディレクトリの `git remote get-url origin` からCodex専用clone用URLを推定するようにした。
  - GitHub URLからowner/repoを推定する既存処理と接続し、対象repo内から `run-weaver install --target wsl` だけでserviceに `--repo-url` と `--repo` が入るようにした。
  - README、CLI docs、architecture、handoffに簡略化後の手順を反映した。
  - `go test ./...` と `go build ./cmd/run-weaver` で既存挙動とbuildを確認した。
- Future tasks:
  - install scriptをpipe実行する場合も、対象repository内で実行する前提をREADMEに維持する。
  - 複数repository管理を始める場合は、単一service前提のままでよいか別途検討する。
- Human-facing reports:
  - Git remoteがないディレクトリでは従来どおり `--repo-url` 指定が必要。
  - 外部アカウント、secret、GitHub Issue、release、pushには触れていない。

## 2026-05-17 - Release Asset Extraction Refactor

Review:

- Immediate fixes:
  - release assetのzip / tar.gz展開で重複していたbinary書き込み処理を `writeReleaseBinary` に共通化した。
  - tar.gz展開時にarchive fileを明示的にcloseするようにした。
  - `go test ./...` で既存挙動が維持されることを確認した。
- Future tasks:
  - self-updateとclone不要install手順のCI / release workflow結果確認を継続する。
- Human-facing reports:
  - 今回は挙動変更なしの内部リファクタリングで、GitHub Issue、release、外部アカウント、secretには触れていない。

## 2026-05-17 - GitHub Release Self Update

Review:

- Immediate fixes:
  - release buildで `Version` にtag名を埋め込むためのversion変数とrelease workflowを追加した。
  - `daemon` 起動時にGitHub Releases latestを確認し、新しいassetがある場合だけself-updateする処理を追加した。
  - `run-weaver update --check` / `run-weaver update` を追加した。
  - 開発ビルドでは自動更新せず、`RUN_WEAVER_NO_UPDATE=1` でrelease buildでも一時停止できるようにした。
  - project clone不要の `scripts/install.sh` / `scripts/install.ps1` を追加した。
  - WSL installがskeletonのままだとclone不要install後に常駐できないため、systemd user service作成を追加した。
  - `--repo-url` がGitHub URLならowner/repoを自動推定し、install後のdaemonがrepository外のcwdから動いても `gh` にrepository指定を渡せるようにした。
  - release version比較、asset選択、tar.gz / zip展開の単体テストを追加した。
- Future tasks:
  - tag `v*` push時のrelease workflow結果を確認する。
  - systemd user serviceの実WSL環境での有効化結果を確認する。
  - systemd user serviceで `gh`、`codex`、`doppler` のPATHやDoppler環境変数が不足する環境があれば、service fileのEnvironment設定を追加する。
- Human-facing reports:
  - 実release作成にはtag pushが必要で、pushは自動では行わない。
  - GitHub Releasesから取得するため、install script実行時はnetwork accessが必要。

## 2026-05-17 - Codex Rate Limit Resume

Review:

- Immediate fixes:
  - `codex exec resume` が利用できることを確認した。
  - Codex JSONLログがrate limitを示す場合、blockedへ落とさず、次回daemon pollで同じworktreeから `codex exec resume <session>` を起動する処理を追加した。
  - JSONLログから `session_id` / `sessionId` を抽出してstateへ保存するようにした。
  - session idが取得できない場合は、同じworktreeで `codex exec resume --last` を試すfallbackを追加した。
  - `status` がrate limit待機中を `runtimeState: rate_limited_waiting` として表示するようにした。
  - rate limit resume、nested session id抽出、`--last` fallback、resume command生成の単体テストを追加した。
  - READMEとCLI docsにインストール手順と、install後に使うための前提条件を追記した。
- Future tasks:
  - GitHub ActionsのLinux / Windows job結果を確認する。
  - `run-weaver:ready` 以外のフィルタを検討する。
- Human-facing reports:
  - installは常駐設定を作るが、利用開始には `doctor` が確認する依存関係と認証、`--repo-url`、対象Issueの `run-weaver:ready` ラベルが必要。

## 2026-05-17 - Status Runtime State

Review:

- Immediate fixes:
  - GitHub ActionsのLinux / Windows jobが最新コミット `847e6d3` で成功したことを確認した。
  - `status` のJSONとhuman outputに `runtimeState` を追加した。
  - `blocked`、GitHub label conflict時の `needs_attention`、last messageありかつtmux window終了済みの `codex_completed` の単体テストを追加した。
- Future tasks:
  - status表示細分化のGitHub Actions結果を確認する。
  - `run-weaver:ready` 以外のフィルタを検討する。
- Human-facing reports:
  - `runtimeState` は表示用の追加フィールドで、既存の `labelState` は維持する。

## 2026-05-17 - Windows Install Implementation

Review:

- Immediate fixes:
  - Windows targetの常駐方式とログ保存場所をaccepted decisionとして記録した。
  - `run-weaver install --target windows --repo-url <url>` でper-user Task Scheduler task `run-weaver` を作成または更新する処理を追加した。
  - Windows daemon stdout/stderrを `%LOCALAPPDATA%\run-weaver\logs\daemon.log` へ追記するtask commandを生成するようにした。
  - `doctor --target windows` がlogs directoryの書き込み可否を確認するようにした。
  - Windows install command生成、Task Scheduler登録、`--repo-url` 必須、command失敗の単体テストを追加した。
- Future tasks:
  - GitHub ActionsのWindows job結果を確認し、Windows固有差分があれば修正する。
  - Windowsでの実GitHub Issue書き込み検証は、認証と外部副作用を伴うため引き続き対象外。
- Human-facing reports:
  - CI確認には再pushが必要。

## 2026-05-17 - Windows Daemon Decision Prep

Review:

- Immediate fixes:
  - Windows daemon常駐方式とログ保存場所の判断に向けて、`docs/decision-prep-windows-daemon.md` を作成した。
  - 推奨案はper-user Task Scheduler + `%LOCALAPPDATA%\run-weaver` 配下のstate / logs / issue logs。
- Future tasks:
  - ユーザー判断後、Windows install / doctor仕様を実装する。
  - GitHub ActionsのWindows job結果確認は、pushまたはPR作成後に行う。
- Human-facing reports:
  - pushは自動では行わないため、CI確認にはユーザーのpush依頼またはユーザー操作が必要。
  - Windows daemon常駐方式とログ保存場所は重要判断のため、ユーザー判断が必要。

## 2026-05-17 - Windows CI Validation

Review:

- Immediate fixes:
  - `doctor` / `status` のOS判定、PATH探索、外部コマンド出力をテスト注入可能にした。
  - Windows targetのdoctor、Task Scheduler、`%LOCALAPPDATA%` 配下state file / worktree root、`tasklist` process照合、tmux不使用、fake `gh` によるGitHubラベル照合のテストを追加した。
  - GitHub ActionsにLinux / Windows jobを追加し、Windowsでは `go test ./...`、`go build ./cmd/run-weaver`、`go test ./internal/cli -run Windows` を実行するようにした。
  - 任意の手元Windows確認用に `scripts/check-windows.ps1` を追加した。
  - `go test ./...`、`go build ./cmd/run-weaver`、`go test ./internal/cli -run Windows` を確認した。
- Future tasks:
  - GitHub ActionsのWindows job結果を確認する。
  - Windows daemon常駐方式とWindowsログ保存場所を決める。
- Human-facing reports:
  - 今回のWindows GitHub照合はfake `gh` によるCLI境界テストで、実GitHub IssueへのWindowsからの書き込み検証は行っていない。

## 2026-05-17 - Windows Status Process Check

Review:

- Immediate fixes:
  - Windows targetの `status` で `os.FindProcess` だけではPID生存確認にならないため、`tasklist /FI "PID eq <pid>" /FO CSV /NH` のCSV出力を解析してprocess照合するようにした。
  - `tasklist` の一致、不一致、実行失敗の単体テストを追加した。
  - `go test ./...` と `go build ./cmd/run-weaver` を確認した。
- Future tasks:
  - Windows実機またはWindows相当環境で `run-weaver doctor --target windows` と `run-weaver status` を実行し、Task Scheduler、state file path、process照合、GitHub照合を確認する。
  - Windows targetのdaemon起動方式は初期実装の必須範囲外として、実機検証時に未実装事項へ分類する。
- Human-facing reports:
  - この環境はLinux/WSLのため、Windows実機でのdoctor / status検証は未実施。

## 2026-05-16 - WSL Integration Success

Review:

- Immediate fixes:
  - Issue #1 の古い `running` ラベルとローカルstateを再実行前に整理した。
  - 本文付きIssue #1で `daemon --target wsl --once` を再実行した。
  - Codexが `README.md` を追加し、daemon完了処理で `Implement issue #1` commit、branch push、draft PR作成、`done` ラベル更新まで進むことを確認した。
  - `status --repo ota-takeru/truth-table-app` でGitHubとstate fileの照合conflictがないことを確認した。
  - Issue本文をpromptに含める修正と、タイトルのみでも具体的なら実行するprompt方針を統合テストで確認した。
- Future tasks:
  - Windows targetのdoctor / status追加検証を行う。
  - stale `running` を人間操作で整理した手順を、将来の自動解除設計で再検討する。
- Human-facing reports:
  - `ota-takeru/truth-table-app#1` は `done` になり、draft PR `https://github.com/ota-takeru/truth-table-app/pull/2` が作成された。

## 2026-05-16 - Issue Body Prompt Handling

Review:

- Immediate fixes:
  - Issue本文を `gh issue view --json body` で取得するようにした。
  - Codex promptにIssue本文を含めるようにした。
  - run-weaver claim/statusコメントを除外したうえで、人間コメントをpromptに含めるようにした。
  - Issue本文が空でもタイトルが具体的なら実行し、タイトル・本文・人間コメントを合わせても不十分な場合だけblockするようpromptを明記した。
  - 再実行時に `codex exec --ask-for-approval` が不正な引数順として失敗したため、`codex --ask-for-approval never exec ...` に修正した。
  - prompt生成の単体テストを追加した。
- Future tasks:
  - 本文追加済みIssue #1で再実行したため完了。
- Human-facing reports:
  - 今後は本文なしでも、タイトルが「README追加」など具体的な変更を示していればCodexへ進める。

## 2026-05-16 - Runner Command Fix

Review:

- Immediate fixes:
  - WSL統合テストでtmux windowが即終了し、Codexログに `mkdir: unrecognized option '--json'` が出ることを確認した。
  - tmux内コマンドの連結を空白連結から `&&` 連結へ修正した。
  - 追加確認で `&&` がCodex引数間にも入る問題を見つけ、`mkdir -p <dir> && codex exec ...` の形に修正した。
  - Codexを `--sandbox workspace-write --ask-for-approval never` で起動するようにした。
  - 既存worktreeがある場合に `git worktree add` を再実行せず再利用するようにした。
  - Codex完了後にworktreeの変更をcommitしてからpushする処理を追加した。
  - Codexが変更なしで完了した場合はdraft PR作成へ進まず `blocked` にするようにした。
  - draft PR作成失敗時にもstate fileを `blocked` へ更新するようにした。
  - runner commandとworktree再利用の単体テストを追加した。
- Future tasks:
  - Issue #1 の `running` を外して再度WSL統合テストを行う。
- Human-facing reports:
  - 1回目の統合テストでIssue #1にはclaimコメントと `running` ラベルが残っている。

## 2026-05-16 - Managed Label Creation

Review:

- Immediate fixes:
  - WSL統合テストで対象repoに `running` が存在せず、`gh issue edit --add-label running` が失敗することを確認した。
  - `running` / `done` / `blocked` を `gh label create --force` で事前作成する処理を追加した。
  - claim前に3つの管理ラベルをensureするようにした。
  - `blocked` / `done` 更新前にも対象ラベルをensureするようにした。
  - 管理ラベルensureの単体テストを追加した。
- Future tasks:
  - WSL統合テストを再実行する。
- Human-facing reports:
  - 対象repositoryにはrun-weaver管理ラベルが作成または更新される。

## 2026-05-16 - Finish Check Before WSL Integration

Review:

- Immediate fixes:
  - `go test ./...` を再実行した。
  - `go build ./cmd/run-weaver` を再実行した。
  - `status -h` と `daemon -h` の基本表示を確認した。
  - `doctor --target wsl --json` と `status --json` の基本出力を確認した。
  - 実装済みCLI仕様とREADME / CLI / Architecture / GitHub Issue Flowの差分を確認した。
  - `docs/progress.md` のCurrent Work Queueを実GitHub Issue統合テストへ進めた。
- Future tasks:
  - ユーザー確認後に実GitHub IssueでWSL統合テストを行う。
  - Windows targetのdoctor / statusをWindows実機で検証する。
- Human-facing reports:
  - WSL統合テストはGitHub Issueのラベル、コメント、branch、draft PRを実際に作るため、人間による対象確認が必要。

## 2026-05-16 - Status GitHub Reconciliation and WSL Test Prep

Review:

- Immediate fixes:
  - `status --repo` を追加し、`gh issue view` でGitHub上の `running` / `done` / `blocked` ラベルを照合するようにした。
  - state fileとGitHub管理ラベルが矛盾する場合、表示上のlabel stateはGitHubを優先し、`github_label_mismatch` をconflictに残すようにした。
  - GitHub照合ができない場合は `github_unavailable` として終了コード2を返すようにした。
  - READMEと `docs/cli.md` に `status --repo` を追記した。
  - 実GitHub Issueで試す前のWSL統合テスト手順と注意点を `docs/handoff.md` に追加した。
- Future tasks:
  - 実GitHub Issueを使ったWSL統合テストを行う。
  - 統合テスト結果に応じてdaemon flow、status表示、docsを修正する。
  - Windows targetのdoctor / statusをWindows実機で追加検証する。
- Human-facing reports:
  - 実IssueテストはGitHubのラベル、コメント、branch、draft PRを作成するため、対象repositoryとIssueを確認してから実行する必要がある。

## 2026-05-16 - Daemon Loop and Blocked State

Review:

- Immediate fixes:
  - `daemon` の継続poll loopを追加した。
  - `--poll-interval` を追加した。
  - `daemon --once` は従来どおり1件処理して終了するようにした。
  - claim後のworktree、prompt、runner失敗時に `blocked` ラベル、blockedコメント、state fileのblocked jobを残すようにした。
  - blocked state file更新の単体テストを追加した。
  - READMEと `docs/cli.md` に `--poll-interval` を追記した。
- Future tasks:
  - 実GitHub Issueを使ったWSL統合テストを行う。
  - `status` でCodex実行中、完了待ち、blockedをさらに分かりやすく表示する。
  - Windows targetの追加検証を行う。
- Human-facing reports:
  - daemonは外部GitHub Issueを変更するため、実行前に対象Issueと `--repo-url` を確認する必要がある。

## 2026-05-16 - Codex Completion and Draft PR

Review:

- Immediate fixes:
  - Codex完了の最小条件を、last message fileが存在し、tmux windowが残っていないこととして実装した。
  - 完了済みstate fileを検出した場合、branchをpushする処理を追加した。
  - `gh pr create --draft` を呼び出し、draft PR URLを取得する処理をdaemon flowへ接続した。
  - draft PR作成後に `done` ラベルを付け、`running` / `blocked` を外し、結果コメントを投稿する処理を追加した。
  - state fileのjob label stateを `done` に更新する処理を追加した。
  - 完了待ち、draft PR作成成功の単体テストを追加した。
- Future tasks:
  - `daemon --once` だけでなく継続poll loopを実装する。
  - 失敗時のstate file更新を強化する。
  - `status` でCodex実行中と完了待ちをより明確に表示する。
- Human-facing reports:
  - 完了判定は初期実装の最小条件。tmux window終了とlast message file作成を前提にしている。

## 2026-05-16 - Daemon Start Flow

Review:

- Immediate fixes:
  - `daemon --once` を追加し、1件だけ処理する入口を作った。
  - `--repo-url` をCodex専用clone作成用のrepository URLとして追加した。
  - ready Issue取得、claim、worktree作成、prompt生成、tmux runner起動、state file保存を一回分のdaemon flowとして接続した。
  - worktree、prompt、runner、state fileのいずれかで失敗した場合にIssueを `blocked` へ寄せる処理を追加した。
  - daemon flowの単体テストを追加した。
- Future tasks:
  - Codex完了監視を実装する。
  - draft PR作成後に `done` ラベルと結果コメントを接続する。
  - `daemon --once` ではなく継続loopとして動かす処理を追加する。
- Human-facing reports:
  - 現時点のdaemonはCodex起動まで。draft PR作成と `done` 更新は次タスク。

## 2026-05-16 - Worktree and Draft PR Building Blocks

Review:

- Immediate fixes:
  - Codex専用cloneとIssue専用worktreeのパス・branch名を作るworktree managerを追加した。
  - clone未作成時は `git clone`、作成済みなら `git fetch origin` する処理を追加した。
  - Issue専用worktreeを `git worktree add -B <branch> <path> origin/HEAD` で作る処理を追加した。
  - Codexへ渡すprompt file生成処理を追加した。
  - draft PR作成用の `gh pr create --draft` wrapperとPR spec生成処理を追加した。
- Future tasks:
  - daemonの一回分処理として、ready Issue取得、claim、worktree、prompt、runnerを接続する。
  - Codex完了監視後にdraft PR作成と `done` / `blocked` ラベル更新を接続する。
  - default branchが `origin/HEAD` で取れないrepositoryへのフォールバックを追加検討する。
- Human-facing reports:
  - draft PR wrapperは実装済みだが、Codex完了監視が未実装のためdaemon flowからはまだ呼び出していない。

## 2026-05-16 - WSL tmux Runner

Review:

- Immediate fixes:
  - tmux session `run-weaver` を作成または再利用するrunnerを追加した。
  - Issue番号からtmux window名 `issue-<number>` を作るようにした。
  - `codex exec --json --cd <worktree> --output-last-message <path> -` の起動コマンドを組み立てるようにした。
  - JSONLログ、最終応答、prompt fileのパスをstate配下のIssue別ディレクトリへ向けるようにした。
  - shell quote処理とtmux command runnerの単体テストを追加した。
- Future tasks:
  - worktree managerとprompt作成を実装する。
  - daemon flowからclaim、worktree、runner、draft PR作成を接続する。
  - tmux windowが既に存在する場合の再実行ポリシーをdaemon flow側で扱う。
- Human-facing reports:
  - runnerはコマンド組み立てとtmux起動境界まで実装済み。Codex完了監視とPR作成は未接続。

## 2026-05-16 - GitHub Issue Claim

Review:

- Immediate fixes:
  - `gh` CLI wrapperを追加した。
  - `run-weaver:ready` ラベル付きopen Issue取得を追加した。
  - `running` / `done` / `blocked` ラベル付きIssueの除外処理を追加した。
  - claim ID付き開始コメント本文と投稿処理を追加した。
  - 開始コメント再取得で最新claim IDを確認し、claim勝敗を判定する処理を追加した。
  - claim競合に負けた場合はstate file更新側へ進まない結果を返すようにした。
  - claim勝ち、claim負け、管理ラベルskipの単体テストを追加した。
- Future tasks:
  - tmux runnerとworktree作成を実装してから、daemon flowへclaim処理を接続する。
  - GitHub照合を `status` のreconciliationへ接続する。
  - `gh` CLIの実リポジトリ操作は統合テスト方針を決めてから追加検証する。
- Human-facing reports:
  - runner未実装のままIssueを `running` にすると放置リスクがあるため、daemonからの実claim接続はまだ行っていない。

## 2026-05-16 - State File and Status

Review:

- Immediate fixes:
  - state file schemaに対応するGo型を追加した。
  - state fileのread/write処理を追加した。
  - `status` が既定state fileを読み込み、human / JSONで表示するようにした。
  - state file未作成時は構造化出力を返し、終了コード1にした。
  - daemon process照合、WSL tmux照合、GitHub照合の境界を分けた。GitHub照合はIssue処理実装まで `not_checked` とする。
  - `status` の単体テストを追加した。
- Future tasks:
  - GitHub Issue取得とclaim処理を実装する。
  - GitHub照合を `status` のreconciliationへ接続する。
  - Windows targetのprocess確認はWindows実機で追加検証する。
- Human-facing reports:
  - state fileがない状態での `status` は異常終了ではなく、状態未作成を示す終了コード1として扱う。

## 2026-05-16 - Go Module and CLI Skeleton

Review:

- Immediate fixes:
  - `go.mod` を追加した。
  - `cmd/run-weaver` にCLI entrypointを追加した。
  - `internal/cli` に `doctor` / `status` / `install` / `daemon` のサブコマンド枠を追加した。
  - `doctor` / `install` / `daemon` で `--target` 必須validationを追加した。
  - `wsl` / `windows` 以外のtargetをusage errorにした。
  - `status --json` と `doctor --json` の最小placeholder出力を追加した。
  - root build artifactを誤コミットしないよう `.gitignore` を追加した。
- Future tasks:
  - `doctor` の実checkを実装する。
  - `status` でstate file読み込みとreconciliationを実装する。
  - CLI usageの文面を `docs/cli.md` の終了コード方針にさらに寄せる。
- Human-facing reports:
  - 現時点のCLIはskeletonであり、実際の依存関係検査やdaemon処理は未実装。

## 2026-05-16 - Doctor Implementation

Review:

- Immediate fixes:
  - `doctor --target wsl` でOS target、`git`、`gh auth`、`codex`、`doppler`、`tmux`、`systemctl --user`、Codex専用clone、worktree root、state file、systemd user serviceを確認するようにした。
  - `doctor --target windows` でWindows target確認とTask Scheduler確認枠を追加した。
  - `doctor --json` を `encoding/json` による構造化出力にした。
  - `doctor` の終了コードをtarget mismatch、missing、auth_required、config不足に分類した。
  - `doctor` の単体テストを実環境の依存有無に左右されにくい形に更新した。
- Future tasks:
  - `status` のstate file読み込みとreconciliationを実装する。
  - `doctor` のWindows targetはWindows実機で追加検証する。
  - Doppler checkの失敗理由を将来もう少し細分化する。
- Human-facing reports:
  - 現在の環境で `doctor` を実行すると、Codex専用cloneやservice未作成ならblockedになる想定。

## 2026-05-16 - CLI Output and State Schema

Review:

- Immediate fixes:
  - `doctor` の人間向け出力例と `doctor --json` の出力例を追加した。
  - `doctor --json` は初期実装から提供する方針にした。
  - `status` の人間向け出力例と `status --json` のトップレベル構造を追加した。
  - state fileの最小JSON schemaを追加した。
  - WSL targetの `systemctl --user` / systemd user service確認を `doctor` の確認項目に追加した。
  - GitHubラベル更新がCASではない前提で、claim ID付き開始コメントと再取得による勝敗判定を明記した。
  - Codex CLIは初期実装で `codex exec` を使い、JSONLログと最終応答をstate配下に保存する方針を明記した。
- Future tasks:
  - Go moduleとCLI skeletonを作成する。
  - `doctor` の各checkを実装し、WSL systemd user managerが使えない場合のblocked表示をテストする。
  - GitHub Issue取得とclaim処理の実装時に、開始コメント競合のテストを追加する。
- Human-facing reports:
  - stale `running` の自動奪取は引き続き初期実装の対象外。将来拡張する場合は運用リスクを再評価する。

## 2026-05-16 - Documentation Review

Review:

- Immediate fixes:
  - CLI名を `run-weaver` に統一した。
  - Issue入口条件として `run-weaver:ready` ラベルを追加した。
  - WSL/Windowsの二重実行防止方針を追加した。
  - `status` の主情報源としてローカルstate fileを追加した。
  - Dopplerの用語を service token `dev-agent` に統一した。
  - AGENTS.mdが参照する運用ドキュメントを追加した。
- Future tasks:
  - state fileのJSON schemaを実装前に確定する。
  - `doctor` と `status` の具体的な出力例を追加する。
  - Windows targetのログ保存先を決める。
- Human-facing reports:
  - stale `running` を自動解除するかは運用リスクがあるため、初期方針では自動奪取せず人間確認に回す。
