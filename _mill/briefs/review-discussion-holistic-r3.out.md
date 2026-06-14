`filepath.Dir` is used widely, including legitimately non-cwd cases (e.g. `state.go:53` `filepath.Dir(lockPath)`, `board/init.go` `filepath.Dir(gitignorePath)`). A static scanner that bans "cwd-based `filepath.Dir`" must distinguish these from genuine cwd-derived geometry. The discussion does not specify the detection heuristic — this is a real feasibility ambiguity worth a NOTE.

I have sufficient grounding. Producing the review.

MILL_REVIEW_BEGIN
# Review: mhgo-portals-launchers

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-14
```

## Findings

### [NOTE] Enforcement test must distinguish cwd-based filepath.Dir
**Section:** Decisions → paths-enforcement; Constraints
**Issue:** `filepath.Dir` appears 30x across the tree, most legitimate (e.g. `state.go:53` `filepath.Dir(lockPath)`, `init.go` `filepath.Dir(gitignorePath)`); banning only "cwd-based `filepath.Dir`" needs a detection heuristic the discussion does not pin down, risking false positives or a leaky guard.
**Fix:** State how the scanner separates cwd-derived `filepath.Dir` from path-of-a-known-path uses (e.g. flag only `filepath.Dir(cwd)`/`filepath.Dir(<Getwd result>)`, or rely on the `os.Getwd`/`--show-toplevel` bans alone and drop the `filepath.Dir` clause).

### [NOTE] muxpoc state migration must also move board.AtomicWrite anchor
**Section:** Decisions → cwd-not-worktree-root (muxpoc)
**Issue:** `SaveState` writes via `board.AtomicWrite(cwd, stateRelPath, ...)` (state.go:108) and `socketArg()` does `cwd, _ := os.Getwd()` swallowing the error (cmd.go:289-294); the migration list names the callers but not these two exact anchor points / the discarded error.
**Fix:** Note that `AtomicWrite`'s base arg and `socketArg`'s `os.Getwd` both retarget to the `Layout` worktree root, and that `socketArg`'s currently-ignored error is surfaced once `paths.Resolve` replaces it.

## Verdict

APPROVE
Scope, decisions, constraints, failure modes, and testing are all resolved and source-grounded; only minor notes remain.
MILL_REVIEW_END