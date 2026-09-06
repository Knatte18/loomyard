MILL_REVIEW_BEGIN
# Review: webster standalone mode: run refuses to start Master; logs write untracked into target repo

```yaml
duration_s: 203.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-5 (best-effort self-assessment)
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [BLOCKING:design] Sink API options are not interchangeable
**Section:** `logger-production-sink-api` **Issue:** The decision defers "second parameter, sibling setter, or options struct" as equally satisfying, but `sink.go:196-208` zeroes `header = sinkHeader{}` and `headerOnce`, so a sibling setter called *before* `SetDurableSinkDir` is silently wiped, while a second parameter churns the ~20 existing call sites the same decision's rationale says it wants to avoid. **Fix:** Decide the shape here, given the reset semantics, or state explicitly that any shape must survive `SetDurableSinkDir`'s header reset.

### [BLOCKING:design] The cmd/lyx ordering test cannot observe what it claims
**Section:** Testing → `cmd/lyx`; `redirect-call-site-and-ordering` **Issue:** `stencilseed.go:39-41` returns before resolving anything under `testing.Testing()`, and `main.go:82` skips `MintOrAdoptAndExport`/`Arm` entirely under test — `stencilseed.go:62-63` says outright "a test can never observe the gate through it" — so "root pre-run emits no Info+ record in a standalone invocation" passes vacuously via the test guard, not via the standalone `ok == false` gate. **Fix:** Name the observable seam instead (`stencilSeedTarget`/`seedStencilsAt` directly, or a tagged subprocess invocation), or drop the claim that the dependency is pinned executably.

### [BLOCKING:design] `NotifyExit` still arms a cwd-derived sink in the target repo
**Section:** `log-sink-redirect-not-gitignore` ("Nothing is written into the target repository at all") **Issue:** `main.go:57` calls `logger.NotifyExit(code)`, which force-runs `ensureDurableSink()` on any non-zero exit (`sink.go:211-216`); on every standalone failure that occurs *before* the redirect (root pre-run, `resolveStandaloneTarget`, `Derive`) the override is still empty, so the cwd fallback writes `<target>/.lyx/logs`. **Fix:** State the disposition of the pre-redirect failure window — accepted residual, or covered — since the integration assertion "target repository is clean" does not hold for those exits.

### [BLOCKING:design] Production call to a process-global setter, in tier-1 tests
**Section:** `redirect-call-site-and-ordering`; Testing → `webstercli`/`burlercli` **Issue:** `wireStandalone` calling `SetDurableSinkDir` makes every tier-1 wiring test mutate process-global sink state with no restoration, and a non-empty override bypasses the `testing.Testing() && LYX_TRACE != "1"` suppression at `sink.go:77-84`, so later tests in the same binary write real trace files into a stale directory. **Fix:** Say whether wiring tests must restore the override (and how a test that drives `wireStandalone` avoids arming a live sink), since the discussion invokes Test Tier Purity but only for resolution/spawn.

### [NIT:consistency] "sole construction site" for standalone logs is already false
**Section:** `logs-dir-single-declarer` **Issue:** `standalonegeom/reedgeom.go:29` already constructs a standalone logs directory, `filepath.Join(stateDir, "logs")`, as `reedengine.Geometry.LogsDir`; the new `standalonegeom.LogsDir(stateDir)` would be a third same-named path helper beside it and `logger.LogsDir`. **Fix:** Reword the claim to "the standalone *trace* logs directory" and state that reed's `<stateDir>/logs` is deliberately unchanged.

### [NIT:decision] Existing sink test pinning the empty worktree root
**Section:** `redirected-sink-header-worktree-root`; "Existing tests to extend rather than duplicate" **Issue:** `internal/logger/sink_test.go:108-117` (`TestEnsureDurableSink_SeamPathLeavesWorktreeRootEmpty`) pins today's empty-header behaviour on exactly the seam path this decision changes, and the discussion's test inventory never names it. **Fix:** State its disposition — rewritten, or kept as the no-worktree-root-supplied case.

## Verdict

REQUEST_CHANGES
Four decisions rest on premises the source falsifies; scope and rationale otherwise sound.
MILL_REVIEW_END
