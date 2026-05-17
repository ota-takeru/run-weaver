# Decision Prep: Windows Daemon and Log Paths

## Decision

Windows targetの常駐方式の詳細と、Windows側のログ保存場所を決める。

## Context

- accepted decisionでは、WindowsとWSLは同じ `run-weaver` binaryを `--target` で切り替える。
- docsではWindows targetはTask Schedulerで常駐するとしているが、登録scope、起動条件、標準出力/標準エラーの保存先は未決定。
- state fileは `%LOCALAPPDATA%\run-weaver\state.json` に決定済み。
- secretやDoppler service token `dev-agent` の値は、state file、ログ、Issueコメント、PR本文、ドキュメント例に出さない。
- 初期daemon flowの実GitHub連携はWSL統合テスト済み。Windowsからの実GitHub書き込み検証は、認証と外部副作用を増やすため現時点では対象外。

## Options

### Option A: Per-user Task Scheduler + `%LOCALAPPDATA%`

- Task Scheduler taskは現在ユーザーの権限で登録する。
- task nameは `run-weaver`。
- daemon stdout/stderrは `%LOCALAPPDATA%\run-weaver\logs\daemon.log` に追記する。
- Codex実行ログとlast messageは、state fileと同じrootの `%LOCALAPPDATA%\run-weaver\issues\<issue>\` に保存する。
- 管理者権限を要求しない。

### Option B: Machine-wide Task Scheduler + `%PROGRAMDATA%`

- Task Scheduler taskはmachine-wideに登録する。
- state / logs / worktreesを `%PROGRAMDATA%\run-weaver\` に寄せる。
- 複数ユーザーで共有できるが、管理者権限、権限設計、secret取り扱いの影響が大きい。

### Option C: Startup folder / foreground-only

- Task Schedulerを初期実装の必須範囲から外し、手動起動またはStartup folderから起動する。
- 実装は軽いが、既存docsの「WindowsはTask Scheduler」と矛盾するためADR/decision更新が必要。

## Recommendation

Option Aを推奨する。

理由:

- 既存の `%LOCALAPPDATA%\run-weaver\state.json` と自然に揃う。
- 管理者権限を不要にでき、ユーザー作業と導入コストが小さい。
- WSL側のuser service方針と同じく、ユーザー単位のagentとして扱える。
- machine-wide権限や共有secretの設計をMVPに持ち込まないで済む。

## Impact

- `run-weaver install --target windows` は、per-user Task Scheduler task `run-weaver` を作成または更新する。
- Windows daemonの標準出力/標準エラーは `%LOCALAPPDATA%\run-weaver\logs\daemon.log` に保存する。
- issue別のCodex JSONLログとlast messageは `%LOCALAPPDATA%\run-weaver\issues\<issue>\` に保存する。
- `doctor --target windows` は、Task Scheduler task `run-weaver` の存在に加え、logs directoryの書き込み可否を確認する。
- docs/cli.md、docs/architecture.md、docs/handoff.md、docs/progress.mdを同期更新する。
- 実GitHub IssueへのWindowsからの書き込み統合テストは引き続き対象外にする。

## Reversibility

- Option AからOption Bへの移行は可能だが、state / logs / worktreesの移行とTask Scheduler task再登録が必要になる。
- Option Aから別常駐方式への変更も可能だが、accepted decisionとdocsの更新が必要になる。
- 初期実装ではTask Scheduler taskをper-userに限定することで、管理者権限やmachine-wide設定に関わる不可逆性を避ける。
