MILL_REVIEW_BEGIN
# Review: webster standalone mode: run refuses to start Master; logs write untracked into target repo — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (best-effort self-assessment; env labels this session "Sonnet 5")
reviewed_file: plan/
date: 2026-09-06
```

## Findings

### [BLOCKING:design] Cards 5/6 assert unobservable `internal/logger` private state
**Location:** Batch 3, cards 5 and 6 **Issue:** Both cards require a `wiring_test.go` test asserting "the durable sink directory was set to `standalonegeom.LogsDir(stateDir)`" and a sibling asserting `wireHub` "leaves the durable sink directory untouched," but `internal/logger/sink.go` exposes no accessor for `sinkDirOverride` (verified: only `LogsDir`, `Arm`, `SetDurableSinkDir`, `NotifyExit` are exported in that file, and no `export_test.go`/getter exists anywhere in the package) — `webstercli`/`burlercli` are separate packages and cannot read it, and neither card adds one. Unlike card 8's integration test (which forces a real write via `t.Setenv("LYX_TRACE","1")` plus a driven verb and checks the filesystem), cards 5/6 describe no such mechanism, and `wireStandalone`'s own new code (`SetDurableSinkDirWithWorktreeRoot`) never itself emits a log record, so merely calling `wire()` writes nothing observable either. **Fix:** Either add an exported test-observable accessor to `internal/logger` (naming it and its Context/Edits placement) or rewrite these two assertions to use card 8's force-a-write-and-check-the-file technique, explicitly, in both cards.

### [BLOCKING:design] Card 8 requirement rests on a false claim about the test's own code
**Location:** Batch 3, card 8 **Issue:** Card 8 says to assert against "the derived `stateDir` the test already computes via `standalonestate.Derive(target)`," but `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged` in `internal/webstercli/cli_integration_test.go` (the exact test the card names for extension, lines 70–100) never calls `standalonestate.Derive` anywhere in its body — only the neighboring, different test `TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate` does. The card gives the implementer no instruction to add that call, so as written it points at a variable that does not exist. **Fix:** State explicitly that the extension must add a `stateDir, _, err := standalonestate.Derive(target)` call (mirroring the sibling test), before using it.

### [BLOCKING:design] Card 8's stated residual mechanism (`NotifyExit`) is unreachable from the test it governs
**Location:** Batch 3, card 8 **Issue:** Card 8 tells the implementer to record in the test's doc comment that a pre-`Derive` failure's target-dirtying comes from `logger.NotifyExit` "force-arming the sink." Verified: `logger.NotifyExit` is called only at `cmd/lyx/main.go:45` and `:57` (grepped repo-wide); `internal/clihelp.RunRootCtx`/`Execute`/`ExecuteIn` (the functions `webstercli.RunCLIIn` actually calls) never call it, and neither does anything in `internal/webstercli/cli.go`'s `resolvePersistentPreRun` or `wiring.go`'s `wireStandalone`. Since this integration test drives `RunCLIIn` directly rather than `cmd/lyx`'s `main()`/`run()`, `NotifyExit` never executes in this test's own call path, so the residual as attributed would land an inaccurate mechanism claim in a permanent doc comment. **Fix:** Either verify and name the actual mechanism that dirties the target for a pre-`Derive` failure reached via `RunCLIIn` (if any), or scope the doc comment's claim to "the real `lyx` binary" rather than to this test's own invocation path.

## Verdict

REQUEST_CHANGES
Two cards (5, 6, 8) rest test requirements on unobservable state or false claims about existing test code.
MILL_REVIEW_END
