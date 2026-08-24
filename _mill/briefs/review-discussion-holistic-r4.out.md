MILL_REVIEW_BEGIN
# Review: loom: redesign the Discussion format

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [BLOCKING:consistency] review-finding-classification.md:7 is the wrong target
**Section:** Scope (bullet "Point ... intro (line 7)"), Acceptance criteria bullet 3
**Issue:** Line 7 of `manifest/designs/review-finding-classification.md` is a factual sentence about the two stencils and points at nothing; the doc's existing pointer into `loom.md`'s Discussion rubric section is line 45 (Concrete proposal item 1), and the symmetry principle Fix 2 echoes is item 5 (lines 52-56) — so "retargeted pointer at line 7, not at `discussion-format.md`" describes an edit that does not exist and gives no content to write.
**Fix:** Name the real edit site (line 45's existing `loom.md#...` link, and/or item 5) and state what the added/changed sentence must say.

### [BLOCKING:decision] roadmap.md:43-47 restates Fix 2 with no disposition
**Section:** Scope / `manifest-cleanup-bundled-in`
**Issue:** `manifest/roadmap.md`'s `loom: Discussion-Review producer` item already carries Fix 2's full content in prose (accept "belongs in support-log.md" / "doesn't belong in Discussion" findings, plus the completeness-before-relocation test); once `loom.md`'s new subsection becomes the authoritative copy, that entry is a duplicate and also runs against roadmap.md's own Maintenance rule (line 368: entries are short, "never a design writeup", detail belongs in the design doc). The task states no disposition for it while explicitly bundling adjacent manifest cleanup.
**Fix:** Decide and record whether roadmap.md:44's Fix-2 prose is trimmed to a pointer at `loom.md`'s new subsection or deliberately left verbatim, and add the matching acceptance criterion.

### [BLOCKING:scope] Deletion-handoff note has no acceptance criterion
**Section:** Constraints (Documentation Lifecycle) vs. Acceptance criteria
**Issue:** The Constraints section requires `discussion-format.md` to flag, for the deleting Wave-2 task, the two inbound pointers that go stale (`loom.md:35`, the roadmap `## Done` entry link) — but Acceptance criterion 1 enumerates the doc's required parts (header, what's-not-changing, Fix 1, Fix 2, "Open, not decided here") and omits this, so a plan writer can omit the only place that obligation is durably recorded.
**Fix:** Add the stale-pointer/deletion-disposition note to the doc's required section list in the acceptance criteria.

### [NIT:design] New loom.md subsection heading left as "e.g."
**Section:** Scope bullet 5 / `fix2-lives-in-loommd`
**Issue:** The heading text is given only as an example, yet its generated anchor is the machine-enforced link target for two inbound links (`discussion-format.md` and `review-finding-classification.md`) under `TestEnforcement_MarkdownLinks`.
**Fix:** Pin the exact heading string in the discussion so both pointers can be written against a fixed anchor.

## Verdict

REQUEST_CHANGES
Two mis-specified manifest edits and one unenforced doc obligation; Fix 1/Fix 2 design itself is sound.
MILL_REVIEW_END
