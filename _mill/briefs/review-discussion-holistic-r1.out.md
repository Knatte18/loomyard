MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Scoutengine Leaf Invariant blocks the stderr retirement
**Section:** Scope → In (stderr sites); `adoption-scope`; Constraints
**Issue:** Routing `internal/scoutengine/ensureserver.go` and `lspclient.go` through `internal/logger` adds an import outside `internal/scoutengine/leaf_enforcement_test.go`'s allowlist (`hubgeometry`, `lock`, `proc`, yaml only — verified lines 24-27), so `TestLeafInvariant_AllowlistOnly` fails; the Constraints section never names this invariant.
**Fix:** Decide and record: widen the scoutengine allowlist (with a CONSTRAINTS.md amendment in the same commit) or drop the two scoutengine sites from scope.

### [GAP] Long-lived shared children bake in a stale trace-ID
**Section:** `trace-id-mint-and-propagate`
**Issue:** `reedengine/lifecycle.go:355` sets `cmd.Env = clean` at *server boot* on a per-hub shared, long-lived tmux server, and `scoutengine/ensureserver.go:520-524` spawns a `DetachBreakaway` daemon meant to outlive the call — both freeze the booting invocation's `LYX_TRACE_ID`, so every later run's panes/daemon lines carry a foreign trace, which is worse than no trace.
**Fix:** State the policy for env inherited by processes that outlive the minting invocation (e.g. strip `LYX_TRACE_ID` at those two spawn sites, or re-inject per pane/session at pane creation).

### [GAP] Root-hook arming also fires inside cmd/lyx's own test binaries
**Section:** `test-entry-activation`; `lazy-sink-open`
**Issue:** The `LYX_TRACE=1` opt-in is scoped to "entry points that never reach `cmd/lyx/main.go`", but `cmd/lyx`'s untagged tests drive `run()` → `newRoot()` (20 call sites across 6 test files), so the mint, the `os.Setenv` of `LYX_TRACE_ID`, and the sink arming all execute under plain `go test` — an inherited/leaked `LYX_TRACE_ID` then makes the "unset mints fresh" test order-dependent, and any Warn triggers `hubgeometry.Resolve` (a git spawn) from an untagged test.
**Fix:** Specify the behaviour of the root hook under `go test` (a `testing.Testing()` suppression, following the `headerLaunchLine` precedent named in CONSTRAINTS, or gating arming on `LYX_TRACE` there too).

### [GAP] Concurrency safety of the new logger globals unspecified
**Section:** `dual-handler-fan-out`; `lazy-sink-open`; `retention`; `explicit-span-parenting`
**Issue:** The lazy sink (open-once), the byte counter behind the 8 MB cap, and the "exactly one truncation marker" rule are mutable package globals with no stated locking, and the premise that production has "exactly one `go func`" is wrong — `scoutengine/lspclient.go:224` and `:241` run `go c.readLoop()`, and the same package's error paths are in adoption scope.
**Fix:** State the synchronisation contract for sink open, byte accounting, and the truncation marker (e.g. `sync.Once` + mutex), and correct the concurrency premise.

### [NOTE] Adoption baseline and spawn-site enumeration are inaccurate
**Section:** Technical context → Adoption baseline; Spawn sites
**Issue:** Actual count is 31 `logger.Debug/Info/Warn` calls across 9 files, with **zero** in `internal/perchengine` and `cmd/lyx` (both listed as having them); and "all non-test `exec.Command` call sites are enumerated" omits `scoutengine/ensureserver.go:520`, `scoutengine/lspclient.go:202`, `reedcli/attach.go:68`, `websterengine/integration.go:190`, `configengine/edit.go:48`.
**Fix:** Correct both lists so the plan sizes the perchengine adoption work against a real baseline.

### [NOTE] Retention sweep scope over the logs directory undefined
**Section:** `retention`
**Issue:** The age/count bounds are stated over "files", not over `trace-`-prefixed files; the cited reed precedent (`pruneServerLogsLocked`, lifecycle.go:322-333) is deliberately prefix-scoped per filename shape.
**Fix:** State that the sweep matches only the `trace-<UTC>-<16-hex>.log` grammar and ignores foreign files in `.lyx/logs/`.

### [NOTE] Treadle seam test's own doc comment repeats the amended claim
**Section:** `logger-imports-hubgeometry`; Constraints
**Issue:** `internal/treadleengine/seam_enforcement_test.go` states "never `internal/hubgeometry`" and "the engine is geometry-blind" in its header and at lines 24-27; only CONSTRAINTS.md is slated for amendment.
**Fix:** Include that file's comment in the same-commit amendment list.

## Verdict

GAPS_FOUND
Four gaps: scoutengine leaf invariant, stale trace-ID in long-lived children, test-binary arming, concurrency.
MILL_REVIEW_END
