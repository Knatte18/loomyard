MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:scope] removeWeftWorktree has two callers, not one
**Section:** Technical context → "`Topology.Remove` shape" (and Scope → In, 4th bullet)
**Issue:** The discussion states "`removeWeftWorktree`'s only caller is `Remove`, so this is a contained change" — but `internal/fabricengine/add.go:264` also calls it, from `rollbackAdd`, with `alsoDeleteBranch = !weftBranchAdopted`, i.e. the very `alsoDeleteBranch` path this task wires remote deletion into; the only other caller is `remove.go:132`.
**Fix:** State the third site explicitly and decide what `rollbackAdd` passes for `remote` (presumably a hardcoded false) so the signature/return-shape change and the "exactly two call sites" scope bullet are both truthful.

### [NIT:decision] Rollback carve-out's "never pushed" premise is weaker than stated
**Section:** Scope → Out, 2nd bullet
**Issue:** The carve-out justifies excluding the rollback sites on "the branch was never pushed", but `Add` pushes the weft branch at step (12) (`add.go:224`, `AddResult.Pushed`), so a rollback reached after that step can face a pushed weft branch.
**Fix:** Keep the exclusion but restate the reason as "a rollback never makes a network-visible destructive change", rather than a factual claim about pushes.

### [NIT:design] removeWeftWorktree reporting shape left as a two-way fork
**Section:** Technical context → "`Topology.Remove` shape"
**Issue:** "either an out-parameter struct or a changed return shape" is an explicit unresolved alternative; with the extra `rollbackAdd` caller it is less contained than assumed.
**Fix:** Name one of the two so the plan writer does not choose, given both callers must now compile against it.

## Verdict

REQUEST_CHANGES
Caller enumeration for `removeWeftWorktree` is wrong; `rollbackAdd`'s remote argument is undecided.
MILL_REVIEW_END
