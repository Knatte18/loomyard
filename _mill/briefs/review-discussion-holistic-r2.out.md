MILL_REVIEW_BEGIN
# Review: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:decision] roadmap.md:51 atomicity claim has no disposition
**Section:** Scope → `manifest/roadmap.md`; Exact edit sites → `manifest/roadmap.md`
**Issue:** `roadmap.md:51` (inside the same Planned `Shed` bullet) still reads "a producer is always atomic (one mechanical action or one LLM session, never an internal multi-step process of its own)" and defines Input as "artifact(s) consumed" — contradicting both the carve-out at `:57–58` and the new thin-Input carve-out; the discussion names `:110`, `:54`, the phase-enum record and verify-only `:57–61`, but never `:51`, even though it treats the identical wording in `shed.md:8`/`loom.md:44` as the claim the carve-out must scope and declares itself `roadmap.md`'s last owner.
**Fix:** state `:51`'s disposition explicitly — either scope it to the carve-out in the same way as `shed.md:8`/`loom.md:44`, or record why it stays verbatim as the pre-carve-out record.

### [NIT:consistency] Q&A still says pointers hang off the Type column
**Section:** Q&A log (2nd entry)
**Issue:** "loom.md gets one-line pointers from its atomicity sentence and its table's Type column" contradicts `shed-md-is-authoritative-loom-md-points` and the later auto-pick entry, which leave `Type` untouched and add a `Kind` column with a single anchor above the table.
**Fix:** reword that Q&A answer to name the `Kind` column and the single above-table anchor.

### [NIT:scope] Acceptance greps scoped to manifest/ and docs/ only
**Section:** Testing → step 2
**Issue:** `README.md:97` inlines its own producer chain ("Preflight → Discussion → Plan → Webster → Finalize") and is outside the `manifest/`+`docs/` grep scope; it carries no retired phrasing today, but the enumeration method cannot see it.
**Fix:** state that `README.md` was checked and is clean (or add it to the grep roots) so the sweep's completeness claim is verifiable.

## Verdict

REQUEST_CHANGES
One roadmap site in the swept item carries an undisposed contradiction of this task's own decision.
MILL_REVIEW_END
