# Handoff

## Current State

- Go実装の初期WSL flowは成功ケースまで確認済み。
- CLI名は `run-weaver` に統一済み。
- Issue入口条件、状態ラベル、二重実行防止、state file、Doppler service tokenの方針を文書化済み。
- `doctor` / `status` の人間向け出力例とJSON出力例を文書化済み。
- state fileの最小JSON schemaを文書化済み。
- WSL systemd user service確認、claim競合検証、Codex CLI非対話起動仕様を文書化済み。
- Go moduleとCLI skeletonを作成済み。
- `doctor` / `status` / `install` / `daemon` のサブコマンド枠と基本target validationを実装済み。
- `doctor --target wsl` の実checkと `doctor --json` の構造化出力を実装済み。
- `doctor --target windows` のWindows target確認とTask Scheduler確認枠を実装済み。
- state fileの型とread/write処理を実装済み。
- `status` がstate fileを読み込み、human / JSONで表示するようになった。
- `status` のprocess、tmux、GitHub照合を実装済み。`--repo` で `gh` CLIへowner/repoを渡せる。
- `gh` CLI wrapper、ready Issue取得、管理ラベル除外、claim ID付き開始コメント、claim勝敗判定を実装済み。
- WSL tmux runner、`run-weaver` session作成、`issue-<number>` window作成、`codex exec` コマンド組み立てを実装済み。
- worktree manager、prompt file生成、draft PR作成wrapperを実装済み。
- `daemon --once` でready Issue取得、claim、worktree作成、prompt生成、tmux起動、state保存まで接続済み。
- Codex完了検出、git push、draft PR作成、`done` ラベル、結果コメント、state更新を実装済み。
- `daemon` の継続poll loop、`--poll-interval`、claim後失敗時のblocked state file更新を実装済み。
- 実GitHub Issue `ota-takeru/truth-table-app#1` で、本文付きIssueからCodex実行、README追加、commit、branch push、draft PR #2作成、`done` ラベル更新まで確認済み。
- Issue本文と人間コメントはCodex promptへ渡される。本文なしでもタイトルが具体的なら実行し、情報不足の場合だけblockする方針。
- Windows targetの `status` は、PID照合に `tasklist` のCSV出力を使うようにした。
- Windows targetのdoctor / statusは、OS判定、Task Scheduler、state file path、process照合、tmux不使用、fake `gh` GitHub照合を単体テストで確認済み。
- GitHub ActionsにLinux / Windows jobを追加済み。Windows jobでは `go test ./...`、`go build ./cmd/run-weaver`、`go test ./internal/cli -run Windows` を実行する。
- 任意の手元Windows確認用に `scripts/check-windows.ps1` を追加済み。
- Windows targetの常駐方式はper-user Task Scheduler、ログ保存場所は `%LOCALAPPDATA%\run-weaver\logs\daemon.log` に決定済み。
- `run-weaver install --target windows` はper-user Task Scheduler task `run-weaver` を作成または更新する。
- GitHub ActionsのLinux / Windows jobは最新確認時点で成功済み。
- `status` は `labelState` に加えて表示用の `runtimeState` を返す。`codex_running`、`codex_completed`、`needs_attention` を区別する。
- Codexがrate limitで中断した場合、daemonはJSONLログからsession idを取得し、同じworktreeで `codex exec resume <session>` を起動する。session idが取れない場合は同じworktreeで `codex exec resume --last` を試す。
- `install` は常駐設定を作るが、実際に使える状態には `doctor` が確認する依存関係と認証、`run-weaver repo add` による対象repository登録、対象Issueの `run-weaver:ready` ラベルが必要。
- release buildの `run-weaver daemon` は起動時にGitHub Releases latestを確認し、新しいassetがあればself-updateしてから処理を続ける。開発ビルドはversion `dev` のため自動更新しない。
- `run-weaver update --check` / `run-weaver update` を追加済み。
- release assetのzip / tar.gz展開処理は共通のbinary書き込み helper を使うように整理済み。
- `.github/workflows/release.yml` はtag `v*` push時にLinux / Windows、amd64 / arm64のrelease assetを作成する。
- maintainer用 `scripts/release.sh` は次tag計算と事前検証をdry-runで行い、`--push` 指定時だけannotated tagをpushしてrelease workflowを起動する。事前検証では `go test ./...` とLinux / Windows、amd64 / arm64のrelease cross-buildを実行する。`--push --watch` ではworkflow完了待ちとrelease asset確認まで行う。
- `scripts/install.sh` と `scripts/install.ps1` はGitHub Releasesからbinaryを取得するため、ローカルにproject cloneがなくても初回導入できる。
- `run-weaver install --target wsl` はsystemd user service `run-weaver.service` を作成または更新し、`systemctl --user enable --now run-weaver.service` を実行する。
- WSL service fileにはinstall時のPATHを `Environment=PATH=...` として保存する。nvmなどユーザーPATH上の `codex` を追加・変更した場合は `run-weaver install --target wsl` を再実行してserviceを更新する。
- `--repo` 未指定でも `--repo-url` がGitHub URLならowner/repoを自動推定し、互換用の単一repo daemon起動引数へ渡す。
- `install` / `daemon` の `--repo-url` 未指定時は、通常 `repos.json` の登録repositoryを使う。対象repository内では `run-weaver repo add` で登録する。
- Campaign Issueは `run-weaver:campaign` と `run-weaver:ready` の両方が付いたopen Issueとして検出する。
- Campaign PlannerはCodexを非同期起動し、repo docs優先で親Issue本文とrepository内roadmap/docsを読ませ、JSON task graphを生成させる。
- Planner JSONが有効な場合だけ通常taskを子Issueとして作成する。Planner不正出力、空task、ID重複、未知dependency、未知task参照decisionは親Campaignを `blocked` にする。
- Campaign Dispatcherはstate fileの `campaign` を正本にし、依存関係が解けた次taskを `plan` / `implement` / `review` / `verify` の順に同じworktreeで実行する。
- Campaign taskは `verify` 完了後にcommit、push、draft PR作成へ進み、PR URL、completed tasks、current taskをCampaign stateへ保存する。
- Campaign task promptには親Campaign本文の詳細contextも含める。E2Eでroadmapの1行だけではtask詳細が落ちることを確認したため修正済み。
- `status` はCampaign progressと通常Issueの `readyQueue` をhuman / JSONで表示する。state schemaは `3` で、schema `1` / `2` は読み込み時に互換扱いする。
- `run-weaver repo add/list/remove` で監視対象repositoryを管理できる。repo設定は `repos.json` に保存し、secret値は保存しない。
- 複数repository登録時、daemonはrepoごとに `gh --repo`、repo別clone、repo別worktree、repo別stateを使い、repo間は同時実行する。
- `status` は登録済みrepositoryのstateを集約表示し、`--repo owner/repo` で対象repoだけを表示する。
- legacy単一repo stateを読む場合でも、state内のrepositoryまたはIssue URLから `owner/repo` を推定できれば `gh --repo` を付けてGitHub照合する。複数repository表示では、legacy stateが対象repositoryに属すると確認できる場合だけfallbackする。
- `run-weaver repo add --doppler auto|required|optional` を追加済み。既存repo設定は `dopplerMode` 未指定なら `auto` として読む。
- Doppler auto判定はrepo root直下の `doppler.yaml`、`doppler.yml`、`.doppler.yaml`、`.doppler.yml` を見る。Doppler不要repoでは未インストールでも進め、必須repoではCodex起動前に `doppler run -- git --version` が通ることを確認し、失敗すれば `blocked` にする。
- Doppler必須repoでは `doppler run -- codex ...`、不要repoでは通常の `codex ...` を使う。Doppler設定ファイルがあるだけでは認証済み扱いにしない。tokenやsecret値はstate、Issueコメント、PR本文、docs例に保存しない。
- Windows targetのCodex起動はtmuxではなくdirect runnerに分岐する。WSL targetはtmux runnerを継続する。
- WSLでtmux windowが終了し、JSONLログが `codex: command not found` などCodex起動失敗を示す場合、daemonは stuckした `running` のままにせずIssue/stateを `blocked` に更新する。
- Campaign子Issueには `run-weaver:campaign-task` を付け、通常ready Issue取得から除外する。
- pending decision gateがある場合、Dispatcherは `can continue tasks` に含まれるtaskだけを実行し、それ以外は `decision_required` で停止する。
- 同一repository内の通常IssueはIssue番号昇順で評価し、repo内では常に最大1 jobだけ実行する。
- 通常Issueのタイトル、本文、人間コメントから `depends: #123`、`blocked by #123`、`stacked on #123`、`依存: #123`、`#123 の後` などを検出し、依存先が未完了なら待機、曖昧またはPR branch / URL不足なら対象Issueを `blocked` にする。
- 依存が解決済みの通常Issueは、依存先branchをbaseにしたstacked draft PRとして作成する。通常Issue完了後は後続依存解決用に `completedIssues` へPR URLとbranchを保存する。
- `ota-takeru/truth-table-app` でWSL E2Eを実施済み。通常Issue #3 / #4 からdraft PR #5 / #6を作成し、#6 が #5 のbranchをbaseにしたstacked PRであることを確認した。Campaign #7 から子Issue #8 / #9、draft PR #10 / #11を作成し、decision gate停止/再開とCampaign progress表示を確認した。
- `ota-takeru/truth-table-app#12` でE2E用 `docs/roadmap.md` を追加し、Codex主導Campaign Planner E2Eを実施済み。Campaign #13 は親Issue本文を「roadmap進めてください」形式にし、Plannerが `docs/roadmap.md` を読んで3 task + 1 decisionのJSON graphを生成した。子Issue #14 / #15 / #16、draft PR #17 / #18 / #19、Decision Request、decision gate停止/回答再開、Campaign doneを確認した。

## Next Step

`docs/progress.md` の Remaining Task Inventory が残タスクの正本。次に進める順序は、複数repository登録後の実GitHub Issue処理確認、Windows direct runner実機確認、Doppler必須repoの実blocked確認、tag push後のrelease workflow / clone不要install / self-update確認。release workflow確認はtag pushが必要なため、人間が `scripts/release.sh --push --watch` を明示実行できるタイミングで行う。2026-05-18時点のリリース前ローカル確認では、latest release v0.1.2のasset取得と `update --check`、release相当cross-build、Goテスト/静的チェックは通過済み。

## Remaining Task Inventory Summary

1. 複数repository登録後の実GitHub Issue処理確認
   - 少なくとも2つの検証repositoryで `run-weaver repo add` を行い、repo別clone / worktree / state、`gh --repo`、repo間同時処理、同一repo内1 job制限、`status` 集約表示を確認する。
   - 完了時は作成されたIssue / PR番号と、`done` または期待どおりの `blocked` まで進んだ結果を `docs/process-log.md` に記録する。
2. Windows target direct runnerの実機確認
   - Windows実機で `scripts/check-windows.ps1`、`install --target windows`、`doctor --target windows`、daemon起動、direct runnerのCodex起動、daemon log、issue別JSONL log、state更新を確認する。
   - 実GitHub書き込みを行う場合は対象repoとIssueを事前に明確にする。
3. self-updateとclone不要installのrelease workflow確認
   - 未リリースのDoppler修正を含む次tagで、人間が `scripts/release.sh --push --watch` を実行した後、4 asset、`scripts/install.sh` / `scripts/install.ps1`、`run-weaver update --check` / `update` / daemon起動時self-updateを確認する。
4. Doppler必須repositoryでの実blocked確認
   - secret値を出さずに使える検証repoをDoppler必須扱いにし、Dopplerが使えない状態でCodex起動前に `blocked` へ止まり、Issueコメント / state / logにsecret値が出ないことを確認する。
5. stuck `running` Issueの整理
   - `ota-takeru/run-weaver#1` は `running` ラベル付きだがtmux windowなし、JSONLに `codex: command not found` が残っている。Issueラベル / コメント変更を伴うため、人間判断後にdaemonで `blocked` へ寄せるか、`running` を外して再投入する。

## Notes

- pushは自動では行わない。
- secretやtokenの実値をログやドキュメントに書かない。
- stale `running` の自動奪取は初期実装では行わない。
- 初期実装のCodex起動は `codex exec` を使う。
- Codexは `--sandbox workspace-write --ask-for-approval never` で起動する。
- Codex完了後、daemonがworktreeの変更をcommitしてからpush / draft PR作成へ進む。変更なしなら `blocked` にする。
- Codex promptにはIssueタイトル、本文、run-weaver管理コメントを除いた人間コメントを渡す。本文なしでもタイトルが具体的なら実行する。
- status表示の細分化は実装済み。CI確認待ち。
- rate limit再開は人間確認条件を迂回しない。push、deploy、外部課金、外部アカウント設定変更、secret表示、破壊的操作、ADR矛盾が必要な場合は従来どおり人間判断に回す。
- self-updateはGitHub Releasesからbinary assetを取得するだけで、GitHub Issueや外部アカウント設定は変更しない。release作成にはtag pushが必要なため、実release workflow検証は人間のpush判断後に行う。
- release作成は利用者向け `run-weaver` CLIには含めず、maintainer用 `scripts/release.sh` で行う。通常確認ではdry-runまでにし、`--push` は人間が明示実行する。workflowとassetまで一連で確認する場合は `--push --watch` を使う。
- 自動更新を止める場合は `RUN_WEAVER_NO_UPDATE=1` を設定する。
- Codex完了判定はlast message fileの存在とtmux window終了を最小条件にしている。
- `daemon` はGitHub Issueのラベルとコメントを実際に変更する。実行前に対象repository、ready Issue、`run-weaver repo add` 済みであることを確認する。
- 対象repositoryに `running` / `done` / `blocked` がない場合、daemonが管理ラベルとして作成または更新する。
- Campaign実行時は、対象repositoryに子IssueとDecision Requestコメントを作成する。taskごとにdraft PRを作る。
- Campaign decision answerは親Campaign Issueコメント内の `run-weaver-decision:<decision-id>:<option>` を読み取ってstateへ保存する。
- Campaign task dependencyは `depends: task-...` のtask ID形式を使う。
- 通常Issue dependencyは `depends: #123` などのIssue番号形式を使う。依存表現が曖昧な場合、agentは独立PRとして推測実行せず `blocked` にする。
- Dopplerが必要なrepoは `run-weaver repo add --doppler required` で明示できる。Doppler不要repoは `--doppler optional` で自動検出を上書きできる。
- `run-weaver repo add` は対象repository内で実行する。Git repository外で実行した場合は `git remote get-url origin` の失敗内容を表示する。
- state fileがない状態の `status` は終了コード1で、JSON/human出力は返す。
- Windows targetのdoctor / statusはGitHub Actionsの `windows-latest` で自動検証する方針。実GitHub IssueへのWindowsからの書き込み検証は、認証と外部副作用を増やすため今回の範囲外。
- Windows targetのdaemon常駐方式とログ保存場所は `docs/decision-log.md` にaccepted decisionとして記録済み。初期daemon flowの実GitHub連携はWSL統合テストの実績を優先する。
- 手元Windowsで追加確認する場合は、PowerShellで `.\scripts\check-windows.ps1` を実行する。このscriptはGitHub書き込みやsecret表示を行わない。
- 現ローカルstateでは `ota-takeru/run-weaver#1` が `running` ラベルのまま、tmux windowなし、JSONLに `codex: command not found` が残っている。更新後daemonを実行すれば `blocked` へ寄せる見込みだが、Issueラベル/コメントを変更するため人間判断で実行する。

## Windows CI / Local Check

GitHub Actions:

1. Linux jobで `go test ./...` と `go build ./cmd/run-weaver` を実行する。
2. Windows jobで `go test ./...`、`go build ./cmd/run-weaver`、`go test ./internal/cli -run Windows` を実行する。
3. fake commandを使うため、GitHub token、Doppler token、実GitHub repositoryは不要。

任意の手元Windows確認:

```powershell
.\scripts\check-windows.ps1
```

このscriptは `run-weaver.exe` をbuildし、`doctor --target windows --json`、稼働中PIDの `status --json`、存在しないPIDの `status --json` を順に実行する。`status` 用stateは `job: null` にするため、GitHub接続は不要。

## WSL Integration Test Prep

実GitHub Issueで試す前に、以下を確認する。

1. `gh auth status` が対象repositoryへread/writeできる。
2. 対象repositoryにopen Issueを1つ用意し、`run-weaver:ready` を付ける。
3. 対象Issueに `running` / `done` / `blocked` が付いていないことを確認する。
4. `go build ./cmd/run-weaver` でローカルバイナリを作る。
5. `./run-weaver doctor --target wsl` で `git`、`gh`、`codex`、`tmux`、`systemctl --user` を確認する。Doppler必須repoでは `./run-weaver doctor --target wsl --repo <owner/repo>` でDopplerも確認する。
6. 対象repository内で `./run-weaver repo add` を実行する。必要なら `--doppler required` または `--doppler optional` を付ける。
7. 初回は `./run-weaver daemon --target wsl --once` だけを使い、継続pollは使わない。
8. 実行後に `./run-weaver status --repo <owner/repo>`、GitHub Issueコメント、tmux windowを確認する。

この手順はIssueラベル、Issueコメント、branch、draft PRを実際に作る。対象Issueを間違えた場合の自動巻き戻しはない。
