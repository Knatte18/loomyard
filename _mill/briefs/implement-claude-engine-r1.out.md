{"status":"success","commit_sha":"a01a22c5d0b1108310230be4758afa5a6297d940","session_id":"1f3018fb-31e0-4c58-be1b-e7cde9a1c99e"}

Summary of work done for batch `03-claude-engine` (round 1):

- `internal/shuttleengine/engine.go` (new) — the `Engine` interface and value types (`Outcome`, `Launch`, `PaneInput`, `StopEvent`, `StartupState`).
- `internal/shuttleengine/seam_enforcement_test.go` (new) — import-scan test enforcing the provider-seam import rule.
- `internal/shuttleengine/claudeengine/doc.go`, `claudeengine.go`, `command.go`, `command_test.go` — package scaffold, `Claude` type/`New`, launch/resume pwsh command composition (`--resume`, never `--continue`).
- `internal/shuttleengine/claudeengine/settings.go`, `settings_test.go` — `buildSettings` (Stop hook + PreToolUse Agent/AskUserQuestion denies) and `Prepare` (mints session id, writes `prompt.md`/`settings.json`, returns `Launch`).
- `internal/shuttleengine/claudeengine/events.go`, `events_test.go` — lenient `ParseEvents` over `events.jsonl`.
- `internal/shuttleengine/claudeengine/startup.go`, `startup_test.go` — `Startup` classification, `InterruptSequence`, `ComposeSend`.

Commits: `2dd9fa7`, `5d4395f`, `34a9bb0`, `a01a22c` (all pushed to `internal-shuttle`). Verify (`go test ./internal/shuttleengine/...`) passes; no gofmt drift; working tree clean.

{"status":"success","commit_sha":"a01a22c5d0b1108310230be4758afa5a6297d940","session_id":"1f3018fb-31e0-4c58-be1b-e7cde9a1c99e"}
