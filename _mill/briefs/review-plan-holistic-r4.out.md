MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), invoked as "Sonnet 5" per harness metadata
reviewed_file: plan/
date: 2026-08-10
```

## Findings

### [BLOCKING:scope] Card 8's Context omits clone.go, source of the `RemoveAll` seam it must call
**Location:** batch 2 (the-gate), card 8 (the executors)
**Issue:** Card 8's Requirements name `removePath`'s call to "the package's `RemoveAll` seam" by identifier, but `RemoveAll` is declared only in `internal/fabricengine/clone.go` (`var RemoveAll = os.RemoveAll`) at the time card 8 executes — card 10, which relocates it into `destroy.go`, runs later in the same batch. Card 8's `Context:` list is `remove.go, weftwiring.go, cleanup.go, portals.go, _mill/discussion.md`; none of these, nor its `Edits: destroy.go` (which does not yet declare `RemoveAll`), shows the seam's declaration or call shape. This matches the rubric's own example for the `scope` class verbatim ("a batch's `Context:` list omits a file the card's own `Requirements:` names").
**Fix:** Add `internal/fabricengine/clone.go` to card 8's `Context:` list.

## Verdict

REQUEST_CHANGES
One Context-completeness gap (card 8 omits clone.go for the `RemoveAll` seam it must reference).
MILL_REVIEW_END
