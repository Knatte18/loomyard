MILL_REVIEW_BEGIN
# Review: Worktree spawn/teardown as Shed producers — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:decision] Roadmap's "optional VS Code embedding" has no stated disposition
**Location:** 00-overview.md Shared Decisions / batch 6 card 33
**Issue:** `manifest/roadmap.md`'s own Planned entry (the one this task closes) defines scope as folding "`fabric create`, reed's self-healing bootstrap, optional VS Code embedding, and `loom`'s own producer list" into one Shed run. The delivered batches cover the first (WorktreeCreate), third (LoomRun, via `loom run --no-attach`'s own bootstrap which already brings reed up), and fourth (WorktreeTeardown = reed down + fabric remove) ingredients, but no card, batch, or Shared Decision ever mentions VS Code embedding — it is neither implemented nor explicitly dropped.
**Fix:** Add a Shared Decision (or extend card 33's Done-entry rewrite) stating VS Code embedding's disposition — e.g. deferred to the Someday "VS Code as opt-in per worktree" item — so the rewritten Done entry doesn't silently claim more scope shipped than it did.

### [BLOCKING:consistency] lifecyclecli's naming deviation is never added to CONSTRAINTS.md
**Location:** batch 6 / card 31 (CONSTRAINTS.md edits); batch 5 / card 24 (`internal/lifecyclecli/cli.go`)
**Issue:** The CLI/Cobra Invariant's package-naming rule ("`<module>cli` imports `<module>engine`") tracks every exception by name in a closed "Deviations:" list — today `stencilcli → internal/stencilstore` and `quarrycli → internal/planglyph`. Card 24 has `lifecyclecli` import `internal/lifecycleshed` and `internal/lifecyclerecipe`; no `lifecycleengine` package exists anywhere in this plan, which is the identical deviation shape those two entries record. Card 31, the card that edits this exact invariant section, only bumps the module-count line ("eleven of twelve" → "twelve of thirteen") and never adds the third deviation entry.
**Fix:** Have card 31 also add `lifecyclecli → internal/lifecycleshed, internal/lifecyclerecipe` to the Deviations list.

### [NIT:consistency] Next Up cross-reference will call a Done item "Planned"
**Location:** batch 6 / card 33
**Issue:** `manifest/roadmap.md`'s Next Up entry "reed: per-hub daemon reaps orphaned sessions" reads "...when the Planned `worktree spawn/teardown as Shed producers` item's deliberate teardown sequencing doesn't run." Card 33 explicitly leaves this sentence untouched, reasoning only that "it references by name, not by link, so it still resolves" — but the word "Planned" becomes factually wrong the instant this same card moves the item to Done, independent of link integrity.
**Fix:** Card 33 should change "Planned" to "Done" (or drop the qualifier) in that cross-reference too.

### [NIT:consistency] docs/overview.md's shed-registry count goes further stale
**Location:** batch 2 (registry growth to seventeen) / batch 6 card 32 (`docs/overview.md` edit)
**Issue:** `docs/overview.md`'s `shed` bullet already understates `internal/shedrecipe`'s registry as "it registers twelve engine names" (card 10 itself says the pre-task table is fourteen, so the doc is already two generations stale). Batch 2 raises the true count to seventeen, and card 32 — which is already editing this same Modules section for the new lifecycle bullet — doesn't fold in the one-word count correction.
**Fix:** Update "twelve engine names" to "seventeen engine names" in card 32 while that section is already being touched.

## Verdict

REQUEST_CHANGES
Two BLOCKING gaps (undisposed VS Code scope item, unrecorded CLI naming deviation) plus two doc-staleness NITs.
MILL_REVIEW_END
