MILL_REVIEW_BEGIN
# Review: loom: redesign the Discussion format

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model, 1M-context variant
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:consistency] Fix 1: stencil points vs. stencil absorbs
**Section:** Constraints (Producer Pointer-Rule bullet vs. Documentation Lifecycle bullet)
**Issue:** The pointer-rule bullet says the rewritten `loom-template-discussion.md` "must point at `discussion-format.md`'s Fix 1 content rather than copy it", while the lifecycle bullet says at Wave 2 "Fix 1's content folds into the stencil itself ... and the design doc is then deleted" — a stencil cannot point at a doc that the same event deletes; verified against CONSTRAINTS.md:567 (the invariant binds an instruction file against duplicating *another producer's format-contract* content, which a design doc is not) and `docs/overview.md`:98.
**Fix:** State one disposition for Fix 1's Wave-2 destiny — content folds into the stencil and the doc is deleted, with the pointer-rule bullet scoped to the Fix 2 / `loom.md` half only.

### [BLOCKING:decision] roadmap.md:18's own Fix 2 restatement undisposed
**Section:** Scope (move-to-Done bullet) / `fix2-lives-in-loommd` / Acceptance criteria
**Issue:** `manifest/roadmap.md:18` — this task's own Planned entry — restates the completeness-before-leanness test and the "relocation is a legitimate finding" principle in full prose, i.e. the exact duplication the task removes from `roadmap.md:43-47` on the Maintenance-rule grounds at line 368; the discussion never says whether the Done entry drops that prose or carries it over, and the shipped Done entries (e.g. `Shed recipe: engine registry`, lines 172-178) run five-plus lines, so "existing Done-entry detail level" does not settle it.
**Fix:** State explicitly that the Done entry condenses to name + one or two sentences + the `discussion-format.md` link, carrying no Fix 1/Fix 2 prose restatement.

### [NIT:scope] Stale-mention enumeration is stated as exhaustive but is not
**Section:** Technical context (last "Exactly two stale Supersedes claims" bullet)
**Issue:** The bullet enumerates `loom.md`'s two mentions and "`review-finding-classification.md`'s mention" (singular), but that file mentions the stencil at both line 7 and line 53, and `docs/overview.md`:320 — inside the stated grep scope — is omitted entirely; none is a supersession claim, so the conclusion holds, but the enumeration presented as grep-confirmed is incomplete.
**Fix:** Correct the mention list, or drop the per-file enumeration and keep only the "exactly two supersession claims" conclusion.

## Verdict

REQUEST_CHANGES
Fix 1's Wave-2 disposition contradicts itself; roadmap.md:18's duplicated Fix 2 prose has no stated disposition.
MILL_REVIEW_END
