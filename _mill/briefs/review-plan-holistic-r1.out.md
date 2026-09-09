MILL_REVIEW_BEGIN
# Review: Centralize glyph ref-shape enumeration — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (Sonnet 5 per harness label)
reviewed_file: plan/
date: 2026-09-09
```

## Findings

None.

Verification performed: read all 7 batches + overview against `_mill/discussion.md` (every `### Decision:`) and the full source set (`classify.go`, `validate.go`, `handle.go`, `containment.go`, `normalize.go`, `rewrite.go`, `parse.go`, `doc.go`, `glyphref.go`, `plan.go` in planparser; `handle.go`, `planglyph.go`, `resolve.go`, `create.go`, `containment.go`, `drift.go`, `scope.go`, `repo.go`, `donecheck.go` in planglyph; `internal/cliwire/bannedecl_enforcement_test.go`; `manifest/designs/quarry-glyph-plan-alphabet.md`; `CONSTRAINTS.md`).

- Batch Index DAG: 7 batches, no cycle, all `file:` entries present, dependencies match actual cross-file needs (batch 3→[1,2] because `rewrite.go` needs batch-2's `IsHandleRef`; batch 4→[2] only, no shape.go dependency; batch 7→[3,4]).
- All Files Touched union recomputed from every batch's Edits/Creates and matches the overview's list exactly (33 entries, both directions).
- Per-gate disposition tables in card 1 (13 policies) verified line-by-line against the actual dispatch logic in `validate.go`/`containment.go`/`handle.go`/`normalize.go` — every listed Keep/Skip/Finding assignment matches current behavior exactly, including the two negation-form Rename arms and the third self-glyph/handle-new arm.
- Card 5/6/7 wrapper-deletion sequencing checked against `grep`: after card 5 migrates `validate.go:1040` and card 6 migrates `normalize.go:112/178`, `isPathRef`'s caller set is empty, matching card 7's premise exactly; the comment-only edit sites in card 7 (`doc.go:56`, `normalize.go:7/70/107`, `parse.go:160`, `parse_test.go:651`, `validate_test.go:1775`) match `grep` results exactly.
- Card 10/11 `.Status` allowlist (5 functions) matches every production `.Status` selector in `internal/planglyph` found by `grep`, with no omission and no extra entry; `quarrycli`'s two call sites confirmed present and correctly cited as intentionally out of scan-scope.
- Card 13 chokepoint claims (`repo.Resolve` single call site at `repo.go:97`, `quarry.Name` single call site at `handle.go:224`) confirmed via `grep` — no other call sites exist today.
- Card 12's drift fix location, variable binding, and "before the amendment loop" placement verified against `drift.go`'s actual control flow.
- Context completeness spot-checked across all 14 cards; every function/constant a card's Requirements names is either in that card's own Edits (implicitly read) or explicitly listed in Context, including citation-only mentions (e.g. card 4's `resolveLanguage`/`cardIDOf` locations, card 8's read-only `drift.go`/`scope.go` call-site verification).
- Moves are `none` on every card (no Rename-mechanic requirement applies); global step numbering 1–14 is sequential with no gaps.

No constraint violations, no over-engineering, no vague Requirements, no missing regression coverage for the two sanctioned behavior changes (both get dedicated two-sided tests per the plan's own text, matching discussion's TDD-candidate list).

## Verdict

APPROVE
Plan is exhaustively grounded in source and the discussion; no defect found across all 7 batches.
MILL_REVIEW_END
