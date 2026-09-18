MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Standalone reed sessions lose the watchdog
**Section:** `watchdog-is-one-detached-process-per-hub`, `daemon-single-instance-via-hub-lockfile`
**Issue:** Standalone reed sessions boot through `reedEngine.Up()` from `internal/burlercli/wiring.go:186` and `internal/webstercli/wiring.go` (not `lyx reed up`, which is hub-only per that comment at wiring.go:181), so today they get the watchdog only via the header pane's `eng.Watch`; a hub-only daemon spawned from `reedcli`'s up/resume/attach never covers them, and `fabricengine.HubScratchDir(hub)` is meaningless when `Geometry.HubPath` is a standalone `stateDir`.
**Fix:** State the standalone disposition explicitly — watchdog off in standalone, a per-target daemon with a `standalonestate`-anchored lock, or an in-process loop — as a named decision.

### [BLOCKING:design] Daemon spawn's binary and under-test suppression undecided
**Section:** `daemon-invocation-contract`, Gotchas
**Issue:** The discussion says the daemon "must never re-exec `os.Executable()` under `go test`" but never decides how — while simultaneously deleting `Engine.suppressHeaderLaunch` (`lock.go:38-55`), the existing mechanism for exactly this hazard — and never says which executable path the spawn uses at all.
**Fix:** Decide the binary resolution (`os.Executable()` or other) and the test-time suppression seam, noting that `gitkit.refuseCLIReexec` is a backstop, not the gate.

### [BLOCKING:design] Which layer owns the daemon spawn is ambiguous
**Section:** `daemon-spawn-attempted-by-up-resume-and-attach` vs `daemon-single-instance-via-hub-lockfile`
**Issue:** The spawn is described as firing "exactly as they each already call `pinGeometryOptionsLocked`" — which is an engine-internal call inside `Engine.Up`/`Resume`/`AttachArgv` (`windowsize.go:117`) — while the lock path is "computed in `internal/reedcli` … and told"; a plan writer could put the spawn in either layer, and the engine variant would need `fabricengine` and an exe path it is not told.
**Fix:** Name the owning layer and, if `reedcli`, say how it knows a boot happened on each of the three verbs.

### [BLOCKING:consistency] Discovery cadence contradicts its own caching claim
**Section:** `daemon-discovers-worktrees-by-scanning-the-hub`
**Issue:** "Each cycle the daemon lists live sessions … [and scans] the hub directory's immediate subdirectories, calling `lyxcwd.ResolveWorktree`" contradicts "the mapping is cached and rebuilt when the live session set changes"; `ResolveWorktree` shells out to `git rev-parse --show-toplevel` per candidate (`lyxcwd.go:149`), so the two readings differ by N real process spawns every 5s, and Live-Substrate Spawn Observability's Debug-level probe-spawn rule is never applied to them.
**Fix:** Pick one cadence and state the logging level for the per-candidate git spawns.

### [BLOCKING:scope] docs/overview.md omitted from the doc obligations
**Section:** Scope "Docs in the same commit", Constraints
**Issue:** `docs/overview.md:301` lists `header` in reed's verb line and names `reed header --blocking` as one of reed's "two registered interactive-handoff exceptions", and lines 408/462 describe `tokenvocab` as the `repo`/`hub` registry — all falsified by this task, yet only the design doc, roadmap, and `doc.go` are in scope.
**Fix:** Add `docs/overview.md` to the same-commit doc list.

### [NIT:design] `status-left-length` value unquantified
**Section:** `status-line-content-and-pins`
**Issue:** "large enough for the rendered text" leaves the value and its unit (bytes vs display cells, pre- or post-`#`-escaping) to the plan writer.
**Fix:** State the computed value and which string it is measured over.

### [NIT:consistency] Standalone `{{.worktree}}` == `{{.repo}}` claim is not exact
**Section:** `worktree-token-joins-the-vocabulary`, standalone disposition
**Issue:** `standalonegeom.ReedGeometry` sets `RepoName: filepath.Base(target)` RAW (reedgeom.go:57) while the proposed `worktree` value is `filepath.Base(standalonestate.Normalize(target))`, so a symlinked spelling renders two different names, not "the same directory name".
**Fix:** Qualify the claim, or say both tokens take the normalized spelling.

### [NIT:decision] `HeaderText`/`ValidateHeader` disposition unstated
**Section:** Technical context, `statusline-verb-replaces-header-verb`
**Issue:** Both are exported `*Engine` methods enumerated in `manifest/designs/reed-fabric-standalone-api.md:124`, and `ValidateHeader` is an eager boot gate (`doc.go:47`), but no decision says whether they are renamed, kept, or retired.
**Fix:** State their disposition and whether the standalone-API doc's method inventory moves with them.

## Verdict

REQUEST_CHANGES
Standalone watchdog coverage, spawn mechanism, and doc scope are unresolved.
MILL_REVIEW_END
