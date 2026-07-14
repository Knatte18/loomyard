MILL_REVIEW_BEGIN
# Review: Investigate the unexplained lyx mux server crash — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-14
```

## Findings

### [MAJOR] Card 6 smoke test targets an ambiguous "fixture hub" dir
**Location:** Batch 2 / Card 6
**Issue:** Card says pre-create fake `tmux-server-*.log` in "the fixture hub's `.lyx/logs/`", but `PairedFixture.Hub` is `<container>/hub` — the *worktree root*, while `layout.HubLogsDir()` = `filepath.Join(l.Hub,".lyx","logs")` with `l.Hub = filepath.Dir(worktreeRoot)` = the *container* (parent of `fixture.Hub`). Placing fakes under `fixture.Hub/.lyx/logs` targets the wrong directory, so the prune/existence assertions never see the engine's real log dir.
**Fix:** Instruct the test to compute the dir as the engine does — the geometry hub (`filepath.Dir(fixture.Hub)`), i.e. `layout.HubLogsDir()` per card 3 — not `fixture.Hub`.

### [MINOR] Card 5 Context omits config.go for `e.cfg.DebugLog`
**Location:** Batch 2 / Card 5
**Issue:** Requirements call `debugLogArgs(e.cfg.DebugLog)`, but `Config.DebugLog` is defined in `internal/muxengine/config.go`, which is in neither Context nor Edits (Context lists only hubgeometry.go, env.go). Card 4 (same batch) adds the field, so harm is small, but the Context-completeness rule wants the defining file listed.
**Fix:** Add `internal/muxengine/config.go` to card 5's Context.

### [NIT] Card 4 template.go godoc classifies debug_log wrongly
**Location:** Batch 2 / Card 4
**Issue:** template.go's godoc splits keys into `${env:...}`-syntax paths (psmux/pwsh) vs "plain literals" (width, top_band_rows, …). `debug_log: ${env:LYX_MUX_DEBUG:-0}` is env-syntax, not a plain literal; folding it into the literal list would make the godoc inaccurate.
**Fix:** When extending the key list, place `debug_log` with the env-resolved group, not the plain-literal one.

### [NIT] Server cwd move vs Windows teardown belt
**Location:** Batch 2 / Card 5
**Issue:** Setting `cmd.Dir = <hub>/.lyx/logs` moves the server cwd off the worktree into the container. `deferHubRelease`/`hubHolders` scan only `fixture.Hub` (the worktree), so on Windows a leaked server now holding `<container>/.lyx/logs` (inside the TempDir) would escape that belt. Down reaps the server synchronously, so this is a leak-path-only concern, and the run is on Linux; worth a note when reconciling the cwd prose card 5 already calls out.
**Fix:** None required; ensure the reconciled lifecycle.go prose notes the server cwd is now the hub logs dir, not the worktree.

## Verdict

REQUEST_CHANGES
Sound, well-grounded plan; clarify card 6's log directory and card 5's Context.
MILL_REVIEW_END
