MILL_REVIEW_BEGIN
# Review: loom: Webster-Review producer

```yaml
duration_s: 232.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (best-effort; environment reports model id claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [NIT:scope] "git grep -n sixteen" not-touch list is incomplete
**Section:** Technical context → Row-count knock-on
**Issue:** The list is framed as exhaustive ("finds every site"), but the must-not-touch half omits `manifest/designs/plan-card-format.md:91` and `manifest/designs/loom.md:155` (both check-ID counts) and `internal/planparser/validate_test.go`; only the stated *criterion* covers them.
**Fix:** Add those three to the must-not-touch enumeration, or drop the exhaustiveness claim and rely on the criterion alone.

### [NIT:scope] loom.md:78 task list not in the doc-update inventory
**Section:** Scope → In (doc updates)
**Issue:** `manifest/designs/loom.md:78` names `loom: Webster-Review producer` among the deliberately-last per-producer tasks, with `loom: Plan-Review producer (shipped)` beside it; the inventory names only rows 13/16/17/51 and the rubric section, so line 78 keeps a stale non-shipped marker.
**Fix:** Either add line 78 to the loom.md edit list, or state that the `(shipped)` marker convention there is deliberately left alone.

### [NIT:consistency] "nothing but Webster commits warp content" is stronger than the mechanism
**Section:** Decisions → diff-derivation-lives-in-the-rubric-not-in-go, Rationale
**Issue:** `Discussion-Burler` ships `fix-scope: source`, whose prompt authorises working-tree commits beyond the target paths (`internal/burlerengine/prompt.go:144-149`), so a pre-Webster warp commit is possible in-system, not only via an operator hand-commit as the open-risk bullet frames it.
**Fix:** Soften the premise to "normally carries no commits" and fold the `fix-scope: source` sibling into the existing merge-base open risk, whose "reviews everything the branch introduces" disposition already covers it.

### [NIT:consistency] Derivation recipe duplicated across recipe and stencil with no pin
**Section:** Decisions → diff-derivation… and bouncer-artifact-paths-names-the-plan
**Issue:** The same status.json/merge-base derivation is stated both in `profile.target.instructions` (recipe yaml) and in the rubric stencil, with no test asserting agreement; editing one leaves the round with two conflicting recipes.
**Fix:** Name one as canonical (the rubric, which both rows already read) and reduce `target.instructions` to a pointer at it, or record the duplication as an accepted risk with the sibling-row precedent cited.

## Verdict

APPROVE
Decisions verified against source; every mechanism claim checked holds. Only doc-inventory and framing nits remain.
MILL_REVIEW_END
