MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [GAP] v2-coexistence prose survives A+B with no owner
**Section:** A (doc retirement) / B / E
**Issue:** Only `loom.md:29` is named as prose that B's find/replace leaves self-contradicting, but the same v2-vs-v3 argument survives verbatim elsewhere and, once A deletes v2 and B reuses the filename, every "v2" link silently re-targets v3 content: `docs/reference/builder-contract.md:7,:53,:243` (`[plan-format v2](plan-format.md)` and two more), `docs/reference/model-spec.md:3` ("Pinned alongside [plan-format v2](plan-format.md) and the emerging [v3]"), `manifest/roadmap.md:207` ("Coexists with the still-live plan-format v2 — still used by the frozen `builder`"), `docs/reference/status-schema.md:3,:73`, `manifest/designs/review-finding-classification.md:7,:47` (B's sweep turns `:47`'s pair into "plan-format.md / plan-format.md"), and `docs/reference/plan-format-v3.md:5`'s own "Coexistence, not replacement" section, which the renamed file carries forward asserting it does not retire v2.
**Fix:** Name a single owner for the v2-coexistence prose class (A for what A falsifies, C for the renamed doc's own `:5` section, E for `roadmap.md:207`) rather than leaving it to each task's implicit re-read.

### [GAP] B's acceptance grep does not match its own stated intent
**Section:** plan-format-v3-renamed-to-plan-format-mechanically; Testing (B)
**Issue:** The decision rejects "leaving in-text 'v3' as a historical label", but the completion criterion is zero hits on exactly `plan-format-v3`, `plan_format_v3`, `plan-format v3` — which leaves `plan-v3` (`loom.md:58` "plan-v3's card contract", `:94` "Webster/plan-v3 equivalent"), bare `v3` prose (`planparser/doc.go:32`, `validate.go:11`, `validate_test.go:240`, `parse_test.go:36,:53`), and any capitalized spelling (`doc.go:32` "Plan-format v3") passing. Case sensitivity of the sweep is unspecified.
**Fix:** State the full pattern set (including `plan-v3` and the capitalized/`Plan-format v3` forms), say whether the sweep is case-insensitive, and explicitly exclude the `gopkg.in/yaml.v3` import token so the script cannot corrupt `internal/planparser/parse.go:21`.

### [GAP] Surfaced open question 3 has no owner
**Section:** Surfaced open questions
**Issue:** Questions 1 and 2 carry explicit **Owner:** lines and question 4 is explicitly noted-only, but question 3 (`shed` overloaded — `docs/overview.md:289`'s "Earlier drafts split reed into separate `shed`/`glance` modules") proposes an action ("worth an explicit disambiguating note") with no task assigned, while `docs/overview.md` is edited by A, B and E.
**Fix:** Assign question 3 to E (the last `overview.md` owner) with the same explicit **Owner:** line format used for 1 and 2, or state that it is note-only like question 4.

### [NOTE] `discussion-format.md:14`'s builder reference unassigned
**Section:** A (doc retirement)
**Issue:** A owns `discussion-format.md:3` and `:30`, but `:14` also grounds the two-file split in "Builder's 'distilled digest, never raw prose' rule (see `builder-contract.md`'s digest contract)" — not dangling, since the doc is retired-not-deleted, but it justifies a live contract by pointing at retired design.
**Fix:** Add `:14` to A's or C's `discussion-format.md` line inventory so the rationale is restated in producer-model terms.

## Verdict

GAPS_FOUND
Three ownership/criterion gaps around the v2→v3 transition and one unowned open question.
MILL_REVIEW_END
