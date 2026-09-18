MILL_REVIEW_BEGIN
# Review: Worktree spawn/teardown as Shed producers

```yaml
duration_s: 157.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Claude Opus 5, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] LoomRun verdict table omits loom `state: failed`
**Section:** `loom-run-poll-bound`, and "Loom status shape" in Technical context
**Issue:** The table is declared exhaustive over loom's `state` but covers only `done`/`blocked`/`paused`/`running`; `internal/shedengine/status.go:15-21` declares five legal values, and `run.go:226` and `:289` both persist `StateFailed` (producer hard error, unrecognised outcome), so `_lyx/loom/status.json` can legitimately read `failed` — a state that would fall through to the attempt cap and stall the lifecycle for the full twelve-hour default before escalating.
**Fix:** Add a `failed` row to the verdict table (with its disposition and reason content) and correct the "takes the three clean-exit values" claim, which is true of `RunOutcome`, not of the persisted `State`.

### [BLOCKING:design] Spawn `Dir`: worktree root or anchor is undecided
**Section:** "Process spawning" / `loom-run-drives-via-no-attach`
**Issue:** Both places say the child's `Dir` is "the task worktree", and the only derivation given (`fabricengine.WorktreePath`) yields the worktree **root** — but the child `lyx loom run` resolves through `lyxcwd.Resolve`, which gates cwd to equal `Join(worktreeRoot, AnchorRel)` (`internal/lyxcwd/lyxcwd.go:67-79,109-128`), so on any subpath-anchored hub a root `Dir` fails with `ErrCwdOutsideAnchor`; `hubgeom.ReedGeometry` already uses `AnchorPath()` as `PaneCwd` for the same reason.
**Fix:** State that `Spawn`'s `Dir` is the resolved `Location.AnchorPath()` of the task worktree, derived in the same lazy closure, not the bare worktree root.

### [BLOCKING:design] Prime-wide lock's acquire/release points unassigned
**Section:** `cross-slug-concurrency-takes-a-prime-wide-lock`
**Issue:** The lock is assigned to `lyx lifecycle run` yet scoped "held only across the `Worktree-Create` and `Worktree-Teardown` rows" — a scope the CLI cannot express from outside an opaque `shedengine.Shed.Run` call, and the Testing section then demands a case asserting release "before `Loom-Run` begins"; nothing says whether the CLI drives row-by-row via `Shed.Step`, whether the two producers take the lock themselves, or how a resume directly into `Worktree-Teardown` re-takes it.
**Fix:** Name the component that acquires and releases the prime-wide lock and the mechanism that bounds it to the two bookend rows, including the resume-into-teardown path.

### [NIT:decision] Reed `Config` source for `TeardownDeps.Shutdown` unstated
**Section:** `loom-status-paths-derived-lazily-by-lifecyclecli`
**Issue:** The chain given for `Shutdown` is `lyxcwd.ResolveWorktree` + `hubgeom.ReedGeometry`, but `reedengine.New(cfg, geom)` also needs a `Config`, which every shipped caller loads via `reedengine.LoadConfig(location.AnchorPath(), "reed")` (`internal/reedcli/cli.go:89`) — unlike `ResolveStatus`, whose chain is pinned end to end.
**Fix:** Add the `LoadConfig` step (against the task worktree's anchor, inside the same lazy closure) to `Shutdown`'s stated derivation chain.

## Verdict

REQUEST_CHANGES
Three gaps: an unhandled loom `failed` state, an ambiguous child cwd, an unassigned lock scope.
MILL_REVIEW_END
