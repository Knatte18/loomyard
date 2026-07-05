HEAD (`b3788742a3a9e4639435c90525d3802b0da449cd`) differs from the baseline, and `git status --porcelain --untracked-files=no` shows no tracked modifications. All verify commands passed. Ready to report.

{"status":"success","commit_sha":"b3788742a3a9e4639435c90525d3802b0da449cd","session_id":"59c08852-e166-4d8a-96b4-eb123fbd7ca6"}

Summary of work done for this holistic-fix round 1 session:

**Baseline HEAD:** `2c8f3b8b150ab4ad02a842f4af3c5630cd3c1841` (mill-go: holistic fix round 1 housekeeping commit)

**Finding 1 (BLOCKING) — sandbox-shuttle-suite.cmd invoked a non-existent subcommand.** Verified accurate: `tools/sandbox/main.go`'s switch had no `"shuttle-suite"` case and `tools/sandbox/suite.go` had no `shuttleSuite` spec, so the pre-existing `SANDBOX-SHUTTLE-SUITE.md` and `sandbox-shuttle-suite.cmd` were unreachable dead files. Fixed by:
- First extending `_mill/plan/05-cli-and-registration.md` card 21's `Edits:` list (was plan-conflict per the brief's rule 6/34) and `_mill/plan/00-overview.md`'s file list, committed separately (`217bdd1`).
- Adding `shuttleSuite` spec (embed `SANDBOX-SHUTTLE-SUITE.md`) to `C:\Code\loomyard\wts\internal-shuttle\tools\sandbox\suite.go`.
- Adding a `"shuttle-suite"` case to `C:\Code\loomyard\wts\internal-shuttle\tools\sandbox\main.go`'s dispatch switch, mirroring `mux-suite` exactly.
- Adding dispatch tests to `C:\Code\loomyard\wts\internal-shuttle\tools\sandbox\main_test.go` (`TestRun_ShuttleSuiteRoutesToLaunch`, `TestRun_ShuttleSuiteFlagsRoutedAfterToken`, `TestRun_ShuttleSuiteErrorPropagation`) and spec tests to `C:\Code\loomyard\wts\internal-shuttle\tools\sandbox\suite_test.go` (`TestRunSuite_ShuttleSpec_*` family), mirroring the mux-suite coverage.
- Committed as `e880318`.

**Finding 2 (NIT) — ParseEvents' Raw stored the trimmed line, not exact bytes.** Verified accurate against `StopEvent.Raw`'s doc comment in `internal/shuttleengine/engine.go` ("the exact JSON line this StopEvent was parsed from"). Fixed `C:\Code\loomyard\wts\internal-shuttle\internal\shuttleengine\claudeengine\events.go` to store the untrimmed original `line` for `Raw` while keeping `trimmed` scoped to the blank-check/JSON-parse only. Added `TestParseEvents_RawPreservesSurroundingWhitespace` to `C:\Code\loomyard\wts\internal-shuttle\internal\shuttleengine\claudeengine\events_test.go` proving the round-trip. Committed as `b378874`.

**Verify:** ran every non-null `verify:` command from batches 1–6 in order (`go test ./internal/muxengine/...`; `go test ./internal/shuttleengine/... ./internal/configreg/...`; `go test ./cmd/lyx/... ./internal/shuttlecli/...`; `go vet -tags smoke ./internal/shuttlecli/...` + `go test ./internal/shuttleengine/... ./internal/shuttlecli/...`) — all passed. Batch 7's verify is `null`. Also ran `go test ./...` for the full repo as a final sanity check — all green.

**Final HEAD:** `b3788742a3a9e4639435c90525d3802b0da449cd`, distinct from the baseline housekeeping commit; `git status --porcelain --untracked-files=no` clean.
