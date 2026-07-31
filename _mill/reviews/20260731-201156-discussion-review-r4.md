MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: claude-opus-4 (Opus 4.x)
reviewed_file: _mill/discussion.md (round 4)
date: 2026-07-31
```

## Findings

### [NOTE] `unwire` junction enumeration under repo-wide pathspec
**Section:** Decisions / init-dissolves-to-fabric-verbs; Testing (`lyx fabric unwire`)
**Issue:** unwire inherits undo.go's config-name-set teardown, but stale-removal now introduces on-disk link scanning; whether "full deactivation" removes on-disk junctions absent from `pathspec` (or only the configured set) is not pinned, and its worktree scope (one host vs hub-wide) is only implied by the "no-ops on unpaired host" test.
**Fix:** State that `unwire` is per-host-worktree and whether it enumerates junctions by scanning on-disk entries (minus `HubReservedNames()`) or by the repo-wide config name-set.

### [NOTE] Interaction of unwire with the repo-wide weft:main records
**Section:** Decisions / init-dissolves-to-fabric-verbs
**Issue:** undo.go clears/commits/pushes the per-worktree weft `_lyx`; the discussion does not state whether `unwire` leaves the repo-wide `.fabric-anchor` and `<BoardDir>/_lyx/config/fabric.yaml` (a different worktree) untouched.
**Fix:** Note explicitly that unwire touches only the per-worktree weft `_lyx`, leaving the weft:main repo-wide records intact for later re-wire.

## Verdict

APPROVE
Scope, decisions, constraints, and failure modes are resolved; only minor unwire-semantics notes remain.
MILL_REVIEW_END
