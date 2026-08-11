# Discussion fix — orchestrator review pass

Source review: `_mill/orch-discussion-review.md` (verdict APPROVE, 0 BLOCKING, 1 NIT).
Fixed under `mill-receiving-review`'s fix-everything default; the finding met no pushback criterion.

## Fixed

- orch — "`reground-discussion-format-14`'s rationale describes a citation that's already gone" — verified against the live file: `discussion-format.md:14` carries no citation at all, task A's commit `0149776a` having stripped the `builder-contract.md` reference rather than redirecting it.
  Restated the decision as an **addition** of attribution to an already-correct, currently uncited sentence, and flagged `shed-followups.md:286` / `:289–292`'s stale "re-ground the citation" premise inline so an implementer does not start from a false assumption about the file's current state.
  Both target sources reconfirmed: `internal/websterengine/doc.go:155` and `docs/overview.md:60`.

## Pushed Back

None.
