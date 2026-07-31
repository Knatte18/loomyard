MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 5 (self-assessed; exact build unknown)
reviewed_file: C:\Code\loomyard\wts\trace-logging\_mill\discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Scout daemon strip buys nothing — the child is gopls
**Section:** Decisions → `long-lived-child-env`
**Issue:** `ensureserver.go:520` spawns `supervisedArgv(command, socketPath)` = the language-server binary (`gopls serve -listen=unix;…`, pinned by `TestSupervisedArgv_IncludesServeListenAndIdleTimeout`), not the lyx binary; gopls never reads `LYX_TRACE_ID` and never emits a lyx log line, so the stated harm ("stamp every later run's lines with a foreign trace") cannot occur there, yet the decision commits to a new `cmd.Env` assignment plus a new scout-side filter helper.
**Fix:** State the actual justification (env hygiene/consistency for a detached child, or symmetry with reed) or drop the scout half from scope and keep only reed's `CleanClaudeEnv` edit.

### [GAP] Integration test contradicts `no-arming-under-test`
**Section:** Testing → "One `//go:build integration` test"
**Issue:** `cmd/lyx`'s existing integration tests drive `run()` **in-process** (`cmd/lyx/main_integration_test.go:52`), where `testing.Testing()` is true and the root hook skips mint/export/arming — so the described test cannot prove "the root-command wiring" at all; `LYX_TRACE=1` activates only logger's own lazy mint site, bypassing the hook.
**Fix:** Say explicitly that this test builds and spawns a real `lyx` binary as a child (precedent: `internal/reedcli/smoke_test.go:779`), or narrow its claim to geometry + file path and drop root-hook wiring from what it proves.

### [GAP] Span records have no assigned level
**Section:** Decisions → `explicit-span-parenting`, `level-policy`, `dual-handler-fan-out`
**Issue:** Nothing states at what level `StartSpan`, `End(nil)` and `End(err)` emit; `level-policy` puts "the step trace" at Debug and Debug is excluded from the durable sink, so on the plainest reading no span open/close ever reaches the trace file, which undercuts `no-reader-cli`'s forward-compat property (2) ("every line carries `trace=` and `span=`").
**Fix:** Decide and record the level of span open/close records (and whether `End(err)` with a non-nil error is Warn), and state which of them are expected in the Info+ durable file.

### [GAP] Count sweep can unlink a live sibling's open file on POSIX
**Section:** Decisions → `retention`
**Issue:** The gotcha covers only Windows ("held open cannot be deleted"); on Linux/macOS the newest-50 pass will happily unlink a file a long-running sibling (`lyx perch`/`reed` loop) still has open — its subsequent writes vanish silently, losing exactly the longest-running trace, and the age pass has the same hazard past 14 days.
**Fix:** State the liveness rule — e.g. never delete a file whose `<pid>` segment is still alive (`internal/proc.IsAlive` is already available), or never delete this process's own file — and say whether the age bound reads the filename timestamp or mtime.

### [GAP] configengine stderr site is operator UX, not a diagnostic
**Section:** Scope → "Retiring the three remaining ad-hoc stderr sites"
**Issue:** `internal/configengine/edit.go:166` prints the YAML parse error to the operator immediately before re-opening their editor (the loop at `edit.go:123-172`); routing it through `logger.Warn` reformats visible interactive feedback as a timestamped slog line and couples it to the log threshold.
**Fix:** Either exclude that site with a one-line reason (interactive feedback, like the CLI/Cobra interactive-handoff carve-out) or state that the reformatted text is the accepted new operator-facing output.

### [NOTE] Scoutengine allowlist prose lives in three places
**Section:** Constraints → Scoutengine Leaf Invariant
**Issue:** Besides the `allowedImports` map, `internal/scoutengine/leaf_enforcement_test.go` enumerates the allowlist in its file header (lines 1-8) and in the failure message at line 106 — which is already stale (omits `internal/proc`).
**Fix:** Name both prose sites alongside the map edit, mirroring the treadle bullet's own "amend the comments too" discipline.

### [NOTE] Reed's log dir can never actually be the trace dir
**Section:** Decisions → `retention` (Sweep scope)
**Issue:** `HubLogsDir()` is `<Hub>/.lyx/logs` and `Hub = filepath.Dir(WorktreeRoot)`, so reed's `tmux-server-<pid>.log` family never shares the worktree-anchored trace directory; the "when the directory is shared" premise is false (the prefix-scoped rule is still right).
**Fix:** Reword to "any other file an operator or future consumer drops there" so the plan does not chase a nonexistent collision.

### [NOTE] New geometry accessor unnamed and untested
**Section:** Decisions → `sink-location`, Testing
**Issue:** The accessor is left as "e.g. `Layout.WorktreeLogsDir()`" and no test for it appears in the Testing section, though every comparable accessor has one (`loomstatus_test.go`, `scoutdaemon_test.go`).
**Fix:** Fix the name and add a one-line hubgeometry unit test to the test list.

## Verdict

GAPS_FOUND
Five gaps: scout strip rationale, integration-test wiring, span levels, POSIX sweep, configengine UX.
MILL_REVIEW_END
