MILL_REVIEW_BEGIN
# Review: finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:scope] README.md's own Raddle-as-separate-phase residue has no owner anywhere in the six-task set
**Location:** overview / Shared Decisions ("untouchable files") and discussion.md's Scope
**Issue:** `README.md:93` reads "the phased orchestrator (Preflight → Discussion → Plan → Webster → Raddle → Finalize)" — the exact stale phase-chain residue this whole `shed-producer-model-scoping` effort exists to remove, listing Raddle as a distinct phase between Webster and Finalize. `manifest/designs/shed-followups.md:420` explicitly assigns task E to fix the *same pattern* at `docs/overview.md:272` ("stale chain 'Preflight → Discussion → Plan → Builder → Raddle → Finalize'"), but no task body — not A (whose README.md list is `:25`,`:86`,`:87`,`:94`,`:115`, never `:93`), not D (this task; README.md is absent from its "Files this task edits" table and its `## All Files Touched`), not E (Part four names only `docs/overview.md:272`) — names README.md line 93. It is prose, not a markdown link, so the new `TestEnforcement_MarkdownLinks` checker (scoped to `manifest/`/`docs/`, link-grammar only) will never catch it either. It will ship unfixed and un-flagged through the entire follow-up set.
**Fix:** Either fold README.md into this task's "safe unowned files" set (parallel to `semantic-index.md`/`webster-parallel-execution.md`/`docs/shared-libs/README.md`) and add a card repairing `:93`'s phase-chain prose to match `finalize-shared-by-reference`/the fold, or explicitly add "`README.md:93`" beside `docs/overview.md:272` in `shed-followups.md`'s task-E Part four so the gap is recorded rather than silently dropped.

## Verdict

REQUEST_CHANGES
One BLOCKING scope gap: README.md's Raddle-slot residue is orphaned across the whole six-task set.
MILL_REVIEW_END
