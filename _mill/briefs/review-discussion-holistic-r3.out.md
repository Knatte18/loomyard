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

### [BLOCKING:design] Fix 2 content dies before its consumer exists
**Section:** Constraints (Documentation Lifecycle bullet) vs. `fix2-principle-only`
**Issue:** The stated disposition deletes `discussion-format.md` when the Wave-2 `loom: Discussion-Write producer` task lands, folding its content into `loom-template-discussion.md` — but Fix 2 is rubric-side content that cannot fold into the *writer's* stencil, and its consumer (`loom: Discussion-Review producer`, roadmap.md:43-47) is sequenced in the later "loom: real LLM producers" group (roadmap.md:41), so the doc's only home for Fix 2 is deleted before the task that needs it runs.
**Fix:** State a separate disposition for Fix 2's half — e.g. the doc survives until the Discussion-Review producer lands, or Fix 2's principle folds into `loom.md`'s "Discussion-Review rubric" section at Wave 2 — rather than one deletion trigger for both fixes.

### [BLOCKING:decision] Four new pointers into a doc slated for deletion
**Section:** Scope (`loom.md` x2, `review-finding-classification.md`, roadmap Done entry) + Constraints (Markdown Link Integrity)
**Issue:** The task mandates adding links to `discussion-format.md` from four `manifest/` files that `TestEnforcement_MarkdownLinks` scans, while simultaneously declaring the target deletable at Wave 2, with no stated disposition for the pointers at that moment — the deletion then breaks the very go-test gate this discussion names as its only test surface.
**Fix:** Name the pointers' disposition at deletion time (retarget to the stencil / to `loom.md`, or remove them) as an explicit note in the new doc's own status header or lifecycle line.

### [BLOCKING:consistency] Supersession scope stated two different widths
**Section:** Scope (bullet 6) vs. Acceptance criteria (bullet 1)
**Issue:** Scope scopes the claim to "Step 3's `Architecture` category specifically — not Step 3's other five categories", while the acceptance criterion says "Step 2 + Step 3 ... only — not Step 5", i.e. all of Step 3; additionally the cited range "stencil lines ~37-43" for the five surviving categories actually spans line 39, the `Architecture` category being replaced (verified against `contracts/stencils/loom/loom-template-discussion.md`).
**Fix:** Restate the acceptance criterion as "Step 2 (bounded) + Step 3's `Architecture` category only", and correct the surviving-category line range to exclude line 39.

## Verdict

REQUEST_CHANGES
Lifecycle of the new doc's two halves and its inbound pointers is unresolved; supersession scope self-contradicts.
MILL_REVIEW_END
