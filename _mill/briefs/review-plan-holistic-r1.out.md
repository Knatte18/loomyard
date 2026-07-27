MILL_REVIEW_BEGIN
# Review: Crucible review spawn as effort-selectable Agent profiles — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-27
```

## Findings

### [MEDIUM] Batch 1 Card 3 doc-consistency grep for "rotate"/"next model" is under-scoped
**Location:** Batch 1, "Batch Tests" item 4 (mirrors `_mill/discussion.md` Testing §4)
**Issue:** The check says "grep `crucible/` for `rotate` and `next model`" and names only two legitimate exception locations (`README.md`'s "Why rotate the model" section and the historical reed-campaign rows ~110/120). A literal grep of the whole `crucible/` directory also hits `orchestrator-prompt.md:56`, and — more importantly — several unrelated, unedited per-module instance files (`builder-review-prompt.md`, `fabric-review-prompt.md`, `board-review-prompt.md`, `gitrepo-review-prompt.md`, `loom-planner-review-prompt.md`, `reed-review-prompt.md`, `webster-review-prompt.md`), all of which describe model-only rotation for their own historical campaigns and are entirely out of this batch's `Edits:` scope.
**Fix:** Scope the grep instruction to the two edited files only (`orchestrator-prompt.md`, `README.md`), or explicitly state that hits in any other `crucible/*-review-prompt.md` instance file are out of scope and must not be touched.

### [NIT] Overview Decision title undercounts substituted tokens
**Location:** `00-overview.md`, "Decision: the five bodies stay byte-identical except three tokens"
**Issue:** The decision body itself lists four varying points — `name:`, the effort word in `description:`, `effort:`, and the H1 heading — but the title says "three tokens." Card 1's own requirements and the Batch Tests §1 diff-check correctly account for all four.
**Fix:** Retitle to "except four tokens" (or similar) so the heading matches its own body.

## Verdict

APPROVE
Plan is complete, internally consistent, and faithfully implements every Shared Decision and discussion.md resolution; only minor non-blocking findings remain.
MILL_REVIEW_END
