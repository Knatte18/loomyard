MILL_REVIEW_BEGIN
# Review: Make producer engines runnable without a lyx worktree

```yaml
duration_s: 152.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md → manifest/designs/producers-standalone.md, manifest/roadmap.md
date: 2026-08-17
```

## Findings

### [BLOCKING:design] Standalone trigger misses the "downloaded repo" case
**Section:** design doc T6 ("when `lyxcwd.Resolve(cwd)` fails … build from told values")
**Issue:** The doc's own tier-1 analysis — verified against `internal/lyxcwd/lyxcwd.go:109-131,171-184` — is that `Resolve` succeeds in *any* git repo run from its root, yet the goal statement at line 5 names "a downloaded repo" as a target; in that case `Resolve` succeeds, the standalone branch never fires, `HubPath` = the parent dir and `RepoName` = its basename, so the run gets the exact fictional-hub tmux-socket hazard the told-geometry decision rejects the synthetic `Location` for, plus a `.lyx` tree written into the reviewed repo.
**Fix:** Pin the standalone trigger as something stronger than "`Resolve` errored" — an explicit opt-in flag, or a tier-2 (`fabricengine.Ready`-class) check — and state which of hub-mode/standalone wins when a target is a plain git repo.

### [BLOCKING:design] `<state>` has no Windows answer despite the "everything pinned" claim
**Section:** design doc T6, `<state>` definition and pinned-values table
**Issue:** T6 asserts "Every told value is pinned here, not left to the implementer", but `<state>` is pinned only as `$XDG_STATE_HOME/lyx/<hash8>/` with a `~/.local/state/lyx/` fallback — a POSIX-only convention in a repo that explicitly ships Windows paths (`internal/fslink` junctions, `shell.ForGOOS`'s `pwsh`), and the same section pins Windows-specific case-insensitive hashing, so Windows is in scope.
**Fix:** Pin the Windows `<state>` location (e.g. `LOCALAPPDATA`-derived) explicitly, or state that standalone is POSIX-only for now and why.

### [NIT:decision] `--target-dir`'s in-hub disposition unstated
**Section:** design doc T6, "`--target-dir` is a resolution base, not a review target"
**Issue:** `--stencils-dir` gets an explicit both-modes ruling ("honoured in both — refusing it in-hub would forbid…"), but `--target-dir` says only that the flag "is unnecessary" in a real worktree — honoured, ignored, or refused is left open.
**Fix:** Add the one-sentence in-hub ruling for `--target-dir` matching the treatment `--stencils-dir` already gets.

### [NIT:consistency] Standalone stencils default sits outside the `_lyx` pair
**Section:** design doc T6, pinned-values table ("defaults to `<state>/stencils`")
**Issue:** T6 pins config at `<state>/_lyx/config/` and argues the Durable-vs-Ephemeral pair are "ordinary siblings under `<state>`", yet stencils — which are `_lyx`-resident in a hub (`<hub>/_board/_lyx/stencils`, `fabricengine.StencilsDir`) — default to a third top-level `<state>/stencils`, with no rationale for the asymmetry.
**Fix:** Either default to `<state>/_lyx/stencils` for hub parity, or state in one line why stencils deliberately sit outside the pair.

## Verdict

REQUEST_CHANGES
Trigger condition and Windows state path must be pinned before plan writing.
MILL_REVIEW_END
