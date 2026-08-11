# Discussion fixer report — orchestrator review (pre-round)

Source review: `_mill/reviews/20260811-orch-review-r1.md` (verdict APPROVE, 0 BLOCKING, 2 NIT).
Target: `_mill/discussion.md`.

## Fixed

- `20260811-orch-review-r1.md` — "[NIT:consistency] Section E's span is off by one line".
  Verified: `## E — shed-model-contradiction-sweep` is at `shed-followups.md:409` and `## F — batcher-standalone-split` at `:552`, so `:409–550` was wrong.
  Restated against `## F`'s heading rather than as a hard end-line, per the finding's own suggested form, so the citation survives a line shift inside F.
- `20260811-orch-review-r1.md` — "[NIT:consistency] `loom.md`'s open-questions residue is cited as two different ranges in the same document".
  Verified: Scope said `:78–83`, Technical context said `:76–83`, with nothing explaining the difference.
  Aligned both to `:76–83` and added a clause stating that `:76–77` is the already-resolved first question included for context and `:78–83` is the residue this task owns.

## Pushed Back

None.
