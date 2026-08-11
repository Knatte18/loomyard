MILL_REVIEW_BEGIN
# Review: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:consistency] Finalize listed as simple inside the carve-out
**Demoted-from:** BLOCKING
**Section:** `### producer-typology-carve-out` (first bullet) **Issue:** the simple-producer bullet names "`Preflight`, `Discussion-Validate`, `Plan-Sweep`, `Plan-Validate`, `Batchifier` and `Finalize`" as the current mechanical examples, directly contradicting `### finalize-is-bespoke` and `shed-md-is-authoritative-loom-md-points`'s "rows 4, 8, 10, 11 **and 12** … are `bespoke`; the other seven are `simple`" (the seven exclude `Finalize`). **Fix:** drop `Finalize` from the simple-mechanical example list and state that the mechanical examples are the five gate/step producers, since this bullet is the text that lands verbatim in `shed.md`'s contract section.

### [BLOCKING:design] Adapter section left asserting Finalize needs no adapter
**Section:** `### two-axes-cross-reference` vs `### finalize-is-bespoke` **Issue:** `shed.md:41` lists `Finalize` among "Mechanical Go-function producers … need no translation adapter at all", and this decision declares `:31–46` unchanged, while `finalize-is-bespoke` grounds the classification on `finalize.md:9`'s fresh-LLM spawn and raddle's parallel leaf forks — the two-axes sentence's worked examples name only `Discussion-Validate` and `perch`, never the one row where the two axes now visibly clash in the same doc. **Fix:** state whether `Finalize` remains adapter-free under the engine axis and name it explicitly as the sharpest non-alignment case in the cross-reference sentence, so the plan writer does not have to decide.

### [NIT:consistency] finalize.md line citations off by one
**Section:** `### finalize-is-bespoke` **Issue:** the cited lines do not match the tree at `c3af3c9c` — the raddle-in-critical-section text is `finalize.md:36–37` (not `:37–38`), the "never released and re-acquired partway through" quote is on `:37` (not `:38`), and the "alternative giving Raddle its own `Shed` producer" candidate is on `:39` (not `:40`); `:8`/`:9` are correct. **Fix:** correct the three numbers, since the discussion elsewhere claims every line number was verified at branch point.

## Verdict

REQUEST_CHANGES
Two decisions contradict each other on `Finalize`; the rest verifies clean against source.
MILL_REVIEW_END
