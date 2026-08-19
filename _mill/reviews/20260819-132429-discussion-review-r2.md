MILL_REVIEW_BEGIN
# Review: loom: session bootstrap

```yaml
duration_s: 262.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] Seeding dirties weft; Preflight then blocks the run
**Section:** `self-seeding`, `missing-record-refusal`
**Issue:** `fabricengine.Clean` runs `git status --porcelain` on the **weft** sibling too (`internal/fabricengine/warpclean.go:27-41`, untracked included), and `preflight.Check` reports `CheckWorktreeClean` on it — so `lyx loom run`'s own `Seed` write of `_lyx/loom/status.json`, and the `--parent` write of `_lyx/fabric/origin.json`, leave the weft dirty and make row 1 (Preflight) Stuck with `OnStuck: ""` ⇒ `StateBlocked` on the very first run.
**Fix:** State how each loom-side `_lyx` write becomes committed (or excluded) before Preflight runs, or move the seed to a point where it is committed as `origin.json` is.

### [BLOCKING:design] No named mechanism for Add's weft-side commit
**Section:** `origin-record-is-committed-and-is-a-new-class`
**Issue:** Technical context points at `Fabric.Commit` as the shape, but `Commit` is bound to one `warpPath`/`weftPath` pair, resolves routing via `lyxcwd.ResolveWorktree(f.warpPath)`, and unconditionally fires `spawnDetachedPushFn` when anything lands (`commit.go:109-166`) — contradicting "the existing push carries it with no new push call"; the only other weft commit path, `Bolt`/`commitWeftAt`, is a stage-all barred by the Fabric Git Invariant's positive-only pathspec rule.
**Fix:** Name the concrete commit call `Add` makes for the *new pair's* weft worktree and state how it avoids both the extra detached push and the stage-all.

### [NIT:consistency] Contradicts the pinned loom-status-spec
**Demoted-from:** BLOCKING
**Section:** `self-seeding`
**Issue:** `contracts/specs/loom-status-spec.md` is a pinned Contract doc stating the seed is written "by a lyx Go command at spawn time" and is the t=0 state "before any `lyx loom run` has executed" (lines 3, 10, 26-28, 89); the discussion moves seeding into `lyx loom run` and gives that doc no disposition — Scope's doc list names only `designs/loom.md`, `docs/overview.md`, `roadmap.md`.
**Fix:** Either add the spec to the same-commit doc updates with the corrected binding, or state why `lyx loom run` counts as the spec's "spawn-time lyx command".

### [NIT:design] Run-lock probe leaves the double-spawn window open
**Section:** `reentrancy-ensure-and-attach`
**Issue:** The probe acquires, releases, then spawns, so two near-simultaneous `lyx loom run`s both see the lock free and both spawn; the loser dies on `Shed.Run`'s `ErrShedBusy` (`shedengine/run.go:56-62`) with the failure only in `driver.log` — exactly the invisible-failure mode "always spawning" was rejected for.
**Fix:** Say the window is accepted (and why it is harmless), or name the serialisation.

### [NIT:scope] Sandbox scenario has no way to obtain a seeded status file
**Section:** Testing → Sandbox
**Issue:** The scenario exercises `lyx loom status`/`pause` "against a seeded status file", but no shipped verb seeds one without going through `lyx loom run`'s tmux bootstrap, and `pause` on an absent file is specified to error.
**Fix:** State how the scenario reaches a seeded state (hand-written fixture, or a seeding side effect of a tmux-free verb).

### [NIT:decision] RefMatcher value for loom's RunDeps unstated
**Section:** `webster-deps-wired-for-real`
**Issue:** `RefMatcher` is listed among the mirrored fields but no value is named; the Fabric Git Invariant requires `fabricengine.RefScanner` in hub-mode webster runs and `NeverMatches` only in standalone, and loom is hub-only.
**Fix:** Pin `RefMatcher` to `fabricengine.RefScanner` explicitly.

## Verdict

REQUEST_CHANGES
Seed/record writes collide with loom's own cleanliness gate; commit mechanism and spec disposition unstated.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
