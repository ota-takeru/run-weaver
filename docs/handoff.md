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
- `run-weaver install --target windows --repo-url <url>` はper-user Task Scheduler task `run-weaver` を作成または更新する。
- GitHub ActionsのLinux / Windows jobは最新確認時点で成功済み。
- `status` は `labelState` に加えて表示用の `runtimeState` を返す。`codex_running`、`codex_completed`、`needs_attention` を区別する。
- Codexがrate limitで中断した場合、daemonはJSONLログからsession idを取得し、同じworktreeで `codex exec resume <session>` を起動する。session idが取れない場合は同じworktreeで `codex exec resume --last` を試す。
- `install` は常駐設定を作るが、実際に使える状態には `doctor` が確認する依存関係と認証、`--repo-url`、対象Issueの `run-weaver:ready` ラベルが必要。
- release buildの `run-weaver daemon` は起動時にGitHub Releases latestを確認し、新しいassetがあればself-updateしてから処理を続ける。開発ビルドはversion `dev` のため自動更新しない。
- `run-weaver update --check` / `run-weaver update` を追加済み。
- `.github/workflows/release.yml` はtag `v*` push時にLinux / Windows、amd64 / arm64のrelease assetを作成する。
- `scripts/install.sh` と `scripts/install.ps1` はGitHub Releasesからbinaryを取得するため、ローカルにproject cloneがなくても初回導入できる。
- `run-weaver install --target wsl --repo-url <url>` はsystemd user service `run-weaver.service` を作成または更新し、`systemctl --user enable --now run-weaver.service` を実行する。
- `--repo` 未指定でも `--repo-url` がGitHub URLならowner/repoを自動推定し、service / Task Schedulerのdaemon起動引数へ渡す。

## Next Step

self-updateとclone不要install手順のCI結果を確認し、tag release workflowは実際にtagをpushするタイミングで確認する。問題なければ次のフィルタ検討へ進む。

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
- 自動更新を止める場合は `RUN_WEAVER_NO_UPDATE=1` を設定する。
- Codex完了判定はlast message fileの存在とtmux window終了を最小条件にしている。
- `daemon` はGitHub Issueのラベルとコメントを実際に変更する。実行前に対象repository、ready Issue、`--repo-url` を確認する。
- 対象repositoryに `running` / `done` / `blocked` がない場合、daemonが管理ラベルとして作成または更新する。
- state fileがない状態の `status` は終了コード1で、JSON/human出力は返す。
- Windows targetのdoctor / statusはGitHub Actionsの `windows-latest` で自動検証する方針。実GitHub IssueへのWindowsからの書き込み検証は、認証と外部副作用を増やすため今回の範囲外。
- Windows targetのdaemon常駐方式とログ保存場所は `docs/decision-log.md` にaccepted decisionとして記録済み。初期daemon flowの実GitHub連携はWSL統合テストの実績を優先する。
- 手元Windowsで追加確認する場合は、PowerShellで `.\scripts\check-windows.ps1` を実行する。このscriptはGitHub書き込みやsecret表示を行わない。

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
5. `./run-weaver doctor --target wsl` で `git`、`gh`、`codex`、`doppler`、`tmux`、`systemctl --user` を確認する。
6. 初回は `./run-weaver daemon --target wsl --once --repo <owner/repo> --repo-url <repo-url>` だけを使い、継続pollは使わない。
7. 実行後に `./run-weaver status --repo <owner/repo>`、GitHub Issueコメント、tmux windowを確認する。

この手順はIssueラベル、Issueコメント、branch、draft PRを実際に作る。対象Issueを間違えた場合の自動巻き戻しはない。
