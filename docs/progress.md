# Progress

## Current Work Queue

現在の優先タスクは、リリース前の残り実機確認です。

Definition of Done:

- 複数repository登録後の実GitHub Issue処理を確認する
- Windows target direct runnerを実機で確認する
- self-updateとclone不要install手順のCI / release workflow結果を確認する
- Doppler必須repositoryでCodex起動前にblockedへ止まることを実repoまたは明示的な検証repoで確認する

Recommended Next Step:

- 複数repository登録後の実GitHub Issue処理、Windows target direct runner実機確認、release workflow結果確認を進める。Doppler必須repoの実blocked確認は、secret値を出さずに検証できる対象repoで行う。

## Completed

- CLI名を `run-weaver` に統一した
- Issue入口条件を `run-weaver:ready` ラベルとして明記した
- `running` / `done` / `blocked` の排他運用を明記した
- WSL/Windowsの二重実行防止方針を明記した
- `status` の主情報源としてローカルstate fileを明記した
- Doppler service token `dev-agent` の扱いを明記した
- AGENTS.mdの参照先ドキュメントを作成した
- `doctor` / `status` の人間向け出力例を追加した
- `doctor --json` を初期実装から提供する方針にした
- `status --json` のトップレベル構造を決めた
- state fileの最小JSON schemaを決めた
- WSL systemd user service確認、claim競合検証、Codex CLI非対話起動仕様を明記した
- Go moduleを作成した
- 標準ライブラリだけでCLI skeletonを作成した
- `--target` の基本validationを追加した
- `doctor` / `status` / `daemon` / `install` のサブコマンド枠を追加した
- CLI skeletonの単体テスト、`go test ./...`、`go build ./cmd/run-weaver` を確認した
- `doctor --target wsl` の依存関係、認証、WSL固有条件checkを実装した
- `doctor --target windows` のOS targetとTask Scheduler確認枠を実装した
- `doctor --json` を構造化出力にした
- `doctor` の終了コードを `docs/cli.md` の方針に合わせた
- state fileの型と読み書き処理を追加した
- `status` がstate fileを読み込むようにした
- state file未作成時の `status --json` は構造化出力と終了コード1を返すようにした
- `status` のprocess、tmux、GitHub照合境界を分けた
- `gh` CLI wrapperを追加した
- `run-weaver:ready` ラベル付きopen Issue取得と管理ラベル除外処理を追加した
- claim ID付き開始コメント投稿と、最新claimコメント再取得による勝敗判定を追加した
- claim競合負けではstate fileを更新しない方針をコード上の境界として実装した
- tmux session `run-weaver` の作成または再利用処理を追加した
- Issueごとのtmux window名 `issue-<number>` を作る処理を追加した
- `codex exec --json --cd <worktree>` の起動コマンドを組み立てる処理を追加した
- JSONLログと最終応答パスをstate配下に向ける処理を追加した
- tmux runnerの単体テストを追加した
- worktree manager、prompt file生成、draft PR作成wrapperを追加した
- `daemon --once` でready Issue取得、claim、worktree作成、prompt作成、tmux起動、state保存まで接続した
- `daemon --once` の `--repo-url` と `--repo` の仕様を `docs/cli.md` に追加した
- Codex完了の最小条件としてlast message fileの存在とtmux window終了を使うようにした
- 完了済みjobでgit push、draft PR作成、`done` ラベル、結果コメント、state更新を行う処理を追加した
- draft PR作成flowの単体テストを追加した
- `daemon` の継続poll loopと `--poll-interval` を追加した
- claim後の失敗時に `blocked` ラベル、blockedコメント、state fileのblocked状態を残す処理を追加した
- `status --repo` でGitHub管理ラベルを照合し、state fileと矛盾する場合にconflictを出すようにした
- 実GitHub Issueで試す前のWSL統合テスト手順を `docs/handoff.md` に追加した
- README、`docs/architecture.md`、`docs/cli.md`、`docs/github-issue-flow.md` の実装済み仕様を確認した
- `run-weaver doctor --target wsl --json`、`status --json`、`daemon -h` 相当の基本動作を手元で確認した
- 実GitHub Issue `ota-takeru/truth-table-app#1` でWSL統合テストを行い、本文付きIssueからCodex実行、README追加、commit、branch push、draft PR #2作成、`done` ラベル更新まで確認した
- Issue本文をCodex promptへ渡し、本文なしでもタイトルが具体的なら実行できるようにした
- Windows targetのstatus process照合を `tasklist` ベースにし、CSV解析の単体テストを追加した
- Windows targetのdoctor / status検証用にOS判定、LookPath、外部コマンド出力をテスト注入可能にした
- Windows targetのdoctor、Task Scheduler、state file path、status process、tmux不使用、fake `gh` 照合の単体テストを追加した
- Linux / WindowsのGitHub Actions CIと任意の手元確認用 `scripts/check-windows.ps1` を追加した
- Windows targetの常駐方式をper-user Task Scheduler、ログ保存場所を `%LOCALAPPDATA%\run-weaver\logs\daemon.log` としてaccepted decisionにした
- `run-weaver install --target windows --repo-url <url>` でper-user Task Scheduler taskを作成する処理を追加した
- `doctor --target windows` がlogs directoryの書き込み可否を確認するようにした
- GitHub ActionsのLinux / Windows jobが最新コミット `847e6d3` で成功した
- `status` のJSONとhuman outputに `runtimeState` を追加し、`codex_running`、`codex_completed`、`needs_attention` を区別するようにした
- Codexがrate limitで中断した場合に、同じworktreeと前回sessionから `codex exec resume` で自動再開する処理を追加した
- READMEとCLI docsにインストール手順と、install後に使うための前提条件を追記した
- release buildの `daemon` 起動時にGitHub Releases latestを確認してself-updateする処理を追加した
- `run-weaver update --check` / `run-weaver update` を追加した
- GitHub Release asset作成workflowと、project clone不要の `scripts/install.sh` / `scripts/install.ps1` を追加した
- `run-weaver install --target wsl --repo-url <url>` でsystemd user serviceを作成または更新する処理を追加した
- `install` / `daemon` の `--repo-url` 未指定時に、カレントディレクトリの `git remote get-url origin` から対象repository URLを自動推定するようにした
- `run-weaver:campaign` + `run-weaver:ready` のCampaign Issue検出を追加した
- Campaign Plannerがtask graphとdecision gateを抽出するようにした
- Campaign taskを子Issueとして作成し、Decision Requestを親Issueへ投稿する処理を追加した
- Campaign Dispatcherがstate上の次taskを選び、`plan` / `implement` / `review` / `verify` phaseでCodexを順に起動するようにした
- Decision Requestへの `run-weaver-decision:<decision-id>:<option>` コメントを読み取ってstateへ保存するようにした
- task完了時にPR URL、completed tasks、current taskをCampaign stateへ保存するようにした
- `status` のJSONとhuman outputにCampaign progressを追加した
- `run-weaver repo add/list/remove` を追加し、カレントGitHub repositoryを監視対象として登録できるようにした
- repo設定 `repos.json`、repo別state / clone / worktree / issue log pathを追加した
- `daemon` が登録済みrepositoryを読み、repoごとに独立してIssue処理できるようにした
- `status` が登録済みrepositoryのstateを集約し、`--repo` でrepo別表示できるようにした
- repo設定に `dopplerMode` を追加し、`run-weaver repo add --doppler auto|required|optional` でDoppler要否を管理できるようにした
- `doctor --repo` とdaemonがDoppler必須repositoryだけDoppler CLI / 認証を必須扱いにするようにした
- Doppler必須repositoryでは `doppler run -- codex ...`、不要repositoryでは通常の `codex ...` を使うようにした
- Windows targetのCodex起動をtmuxではなくdirect runnerへ分岐した
- Campaign子Issueに `run-weaver:campaign-task` を付け、通常task取得から除外するようにした
- pending decision gateがある場合、`can continue tasks` に含まれないtaskを実行せず `decision_required` にするようにした
- repo別rate limit resumeで `<repo-slug>-issue-<number>` window名を維持するようにした
- 同一repository内の通常IssueをIssue番号昇順で評価し、実行中jobがある場合は別Issueを開始しないようにした
- 通常Issue本文、タイトル、人間コメントから `depends: #123` などの依存関係を検出し、依存先が未完了なら待機、曖昧またはPR情報不足なら対象Issueをblockedにするようにした
- 依存先IssueのPR branchをbaseにしたstacked draft PR作成を追加した
- state schemaを `3` に上げ、通常Issueの依存情報、base branch、`completedIssues` を保存するようにした
- `status` のJSONとhuman outputに `readyQueue` を追加した
- maintainer用 `scripts/release.sh` を追加し、dry-run既定で次tag計算、事前検証、明示的なtag pushによるrelease workflow起動を行えるようにした
- `scripts/release.sh` の事前検証にLinux / Windows、amd64 / arm64のrelease cross-buildを追加した
- `scripts/release.sh --push --watch` でtag push後のrelease workflow完了待ちとrelease asset確認を行えるようにした
- 実GitHub Issue `ota-takeru/truth-table-app#3` / `#4` で同一repository内のIssue番号順処理、依存待機、依存先branchをbaseにしたstacked draft PR #5 / #6作成を確認した
- 実GitHub Campaign Issue `ota-takeru/truth-table-app#7` でCampaign検出、子Issue #8 / #9 作成、decision request、`plan` / `implement` / `review` / `verify` pipeline、draft PR #10 / #11作成、Campaign progress表示、decision gate停止/再開、Campaign子Issueの通常ready除外を確認した
- Campaign task promptに親Campaign本文の詳細contextを含めるように修正し、Campaign planning時に既存の `completedIssues` を保持するようにした
- Campaign開始時にCodex Plannerを非同期起動し、repo docs優先のJSON task graphを検証してから子Issue化する方式へ変更した
- Planner出力の正常parse、不正JSON、空task、重複ID、未知dependency、未知task参照decision、Planner失敗blocked、planning status表示の単体テストを追加した
- 実GitHub Campaign Issue `ota-takeru/truth-table-app#13` で、親Issue本文は「roadmap進めてください」形式にし、repo docs優先のCodex Planner JSON生成、子Issue #14 / #15 / #16 作成、Decision Request、decision gate停止/回答再開、draft PR #17 / #18 / #19 作成、Campaign doneを確認した
- E2E準備として `ota-takeru/truth-table-app#12` で `docs/roadmap.md` を追加し、Plannerが同ファイルを読んでtask graphを生成することを確認した
- Git repository外での `run-weaver repo add` 失敗理由を分かりやすくし、legacy単一repo stateの `status` がIssue URLからrepositoryを推定して `gh --repo` を使えるようにした。複数repository表示では別repositoryのlegacy stateを誤表示しないようにした
- WSL installがsystemd user serviceへ実行時PATHを保存するようにし、nvmなどユーザーPATH上の `codex` をserviceから見えるようにした
- tmux window終了後にJSONLログが `codex: command not found` などCodex起動失敗を示す場合、daemonがIssue/stateを `blocked` に更新するようにした
- Doppler必須repositoryでは設定ファイルの存在だけで認証OKにせず、Codex起動前に `doppler run -- git --version` が通ることを確認するようにした
- WSL service PATH生成をOS非依存でLinux形式に固定し、Windows CI上でもWSL service fileテストが通るようにした
- GitHub Actions CIがLinux / Windowsとも成功する状態になった
- リリース前確認として、`go test ./...`、`go test -race ./...`、`go vet ./...`、Linux / Windows amd64 / arm64のrelease相当cross-build、latest release asset取得、`update --check` を確認した

## Upcoming Sequence

1. 複数repository登録後の実GitHub Issue処理確認
2. Windows target direct runnerの実機確認
3. self-updateとclone不要install手順のCI / release workflow結果確認
4. Doppler必須repositoryでの実blocked確認
5. `run-weaver:ready` 以外のフィルタ検討

## Open Decisions To Watch

- `run-weaver:ready` 以外のフィルタ。assignee、milestone、repository allowlistなど
- stale `running` の解除を将来も人間判断に固定するか、自動解除へ拡張するか
- GitHub API直実装へ移行する時期
- Campaign decision answerをGitHubコメント形式のまま運用するか、専用CLIを追加するか
