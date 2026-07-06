All 6 cards committed (with an additional plan-correction commit and a follow-up fix commit), verify passed, working tree clean.

{"status":"success","commit_sha":"02d87dd47d1295afe179fa07b8eb097355fc0795","session_id":"19abe59e-ebfd-4d5a-931c-c32d2b72a6fa"}

Summary of work for this batch (ask-signal, round 1):

- `internal/shuttleengine/engine.go` — renamed `StopEvent` → `Event`, added `EventKind` (`EventStop`/`EventAsk`), renamed `LastAssistantMessage` → `Message`, updated `Engine.ParseEvents` signature/doc.
- `internal/shuttleengine/claudeengine/events.go` + `events_test.go` — `ParseEvents` now emits `EventStop` for `Stop` lines and `EventAsk` for `PreToolUse`+`AskUserQuestion` lines (newline-joined question text, lenient on unexpected shape); added `askQuestionText` helper and new tests.
- `internal/shuttleengine/claudeengine/settings.go` + `settings_test.go` — `buildSettings` now emits a non-denying `AskUserQuestion` marker hook (reusing the Stop hook's exact append command) for interactive runs, mutually exclusive with the existing autonomous deny; updated the deny-toggle-matrix test.
- `internal/shuttleengine/wait.go` — `pollEventsTick` migrated to `[]Event`/`Message`, with an expanded comment explaining why the done/asking branch stays a two-way check rather than a `Kind` switch.
- `internal/shuttleengine/fakes_test.go` + `wait_test.go` — `fakeEngine.ParseEvents` now returns `[]Event` and recognizes an `ASK:` fixture line; added two new wait tests (real-time live-ask classification, done-first-still-wins).
- `docs/overview.md` — added one line to the shuttle module entry documenting the real-time live-ask signal.
- `_mill/plan/02-ask-signal.md` — corrected a factual gap discovered mid-implementation (the Shared Decision's grep claim was wrong): `internal/shuttlecli/cli_test.go`'s `specCapturingEngine` test double also referenced the old `StopEvent` type. Added it to Card 6's `Edits:` list, documented the correction, then migrated its `ParseEvents` signature to keep `internal/shuttlecli` compiling.

Verify (`go test ./internal/shuttleengine/...`) passes; also confirmed `go build ./...` and `go test ./internal/shuttlecli/...` are clean.
