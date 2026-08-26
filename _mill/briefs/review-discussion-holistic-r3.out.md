MILL_REVIEW_BEGIN
# Review: Custom-typed plan cards skip path-missing checks

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:consistency] Q&A log entry contradicts the Pairs-ownership decision
**Section:** `## Q&A log`, the `May a Rename group appear on a multi-label card?` entry
**Issue:** It answers "`Pairs` stays a flat card-level field", which is the exact position `model-shape-additive` and `rename-in-multi-label-cards` overturn (group-owned `Pairs`/`RenameRaw`, card-level retained only as the union); the later Q&A entry records the reversal but the earlier one was never restated, so the artefact answers the same question two ways.
**Fix:** Restate that Q&A answer to "group-owned `Pairs`/`RenameRaw`, card-level fields retained as the flat union", or mark it superseded by the round-2 entry.

### [BLOCKING:design] Sweep predicate's greps are case- and phrase-locked, and demonstrably under-enumerate
**Section:** `## Technical context`, "Repo-wide sweep predicate"
**Issue:** The two stated greps are case-sensitive and pinned to four literal phrases, so they miss live one-label statements the change invalidates — `contracts/specs/loom-plan-spec.md:75` ("**Exactly one bold type label** from the set …", capital E, the spec's primary card-fields rule), `contracts/stencils/webster/webster-body-implementer.md:27` ("in exactly the targets its type label names", a shipped implementer stencil no scope bullet covers at all, while Scope/Out declares Webster unchanged), `internal/planparser/validate.go:189`'s `exactly one is required` detail string, `internal/planparser/doc.go:25`, and `internal/planparser/parse_test.go:296-323`'s "two recognized type labels on one card" defect-shape assertions. The Scope list is presented as this predicate's output, so the method's blind spots propagate into the plan's work inventory.
**Fix:** Restate the predicate as case-insensitive over a label-phrase family (any of `exactly one`/`its type label`/`a card's type label`/`one bold type label` near a type-label mention) plus the count tokens, and name `contracts/stencils/webster/` explicitly as a scanned root.

## Verdict

REQUEST_CHANGES
Stale Q&A answer contradicts the settled Pairs decision; sweep predicate under-enumerates one-label sites.
MILL_REVIEW_END
