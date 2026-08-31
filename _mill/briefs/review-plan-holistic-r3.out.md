MILL_REVIEW_BEGIN
# Review: Reed attach dot-fill render artifact on resize and cross-client mouse move — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), used here as "Sonnet 5" per harness naming
reviewed_file: plan/
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Card 14's readback compares a socket-baked literal across separate test runs
**Location:** 03-repaint-entry.md, Card 14 (`TestSmokeDotFillResizeTreatment`), Branches A and B text: "read reed's array with `windowResizedEntries` and pass it to `assertRepaintEntryPresent` with the body the measurement record names."
**Issue:** For Branch A (candidate 1), the recorded body embeds `-L <socket>` twice, and `ServerName` (server.go) derives that socket from a SHA-256 hash of the hub's *absolute path*. Every `newDotFillHarness` call boots a fresh `hubforge.NewHub` in a fresh `t.TempDir()`, so the socket differs between batch 2's one-time measurement run (which produced the literal string recorded in `doc.go`) and this treatment scenario's own later, separate `go test` invocation — the two will essentially never be byte-identical. `assertRepaintEntryPresent` requires exact equality, so the mandatory readback is unimplementable as specified for Branch A; `reedcli` also cannot call `reedengine`'s unexported `repaintHookCommand`/`resizeRepaintHookCommand` to recompute the expected value locally, since the two are separate packages.
**Fix:** Specify how the treatment derives its expected body for Branch A — e.g., export a minimal `reedengine` helper the `reedcli` test can call with the harness's own `tmuxPath`/`reedSocket`/`reedSession` to recompute the expected string live, or relax the readback to a structural check (prefix/substring match on stable tokens) rather than literal equality against the frozen `doc.go` record. Branch B (`"refresh-client"`, no embedded socket) has no such problem and needs no change.

### [NIT:scope] No direct unit test of the `resizeRepaintHookCommand()` Engine wrapper (Branch A)
**Location:** 03-repaint-entry.md, Card 13, "Branch A only — `internal/reedengine/watchdog_test.go`" and the shared `apply_test.go`/`windowsize_test.go` bullets.
**Issue:** Card 13 tests the pure `repaintHookCommand` builder directly (explicit args) and, separately, tests `resizePinHookArgvs` with a literal repaint-body string — neither exercises `e.resizeRepaintHookCommand()`'s own wiring (which GOOS-gates and assembles `shell.ForGOOS()`, `e.TmuxPath()`, `e.Socket()`, `exactSessionTarget(e.SessionName())`). `TestInstallResizePinsLocked_IssuesTheSignalEntryLast`/`TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear` only assert call *count* and *position*, not that the installed body equals `repaintHookCommand(shell.ForGOOS(), e.TmuxPath(), e.Socket(), exactSessionTarget(e.SessionName()))` — unlike the parallel signal-entry assertion in `WatchdogOnAlsoInstallsTheSignalEntry`, which does check `signal[len(signal)-1] != want`. Branch B's card text explicitly requires this content-level pin (`resizeRepaintHookCommand()` "returns the exact constant..."); Branch A does not get the analogous engine-level content check.
**Fix:** Add one assertion (in `TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear`'s watchdog-on subtest, or a new small test) that the installed repaint-entry body equals `repaintHookCommand(shell.ForGOOS(), e.TmuxPath(), e.Socket(), exactSessionTarget(e.SessionName()))`, mirroring the existing signal-entry pattern.

## Verdict

REQUEST_CHANGES
Card 14's live-array readback for the accepted-candidate-1 branch compares against a value that cannot reproduce across test runs.
MILL_REVIEW_END
