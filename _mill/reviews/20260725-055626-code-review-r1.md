MILL_REVIEW_BEGIN
# Review: board: use gitrepo as its git operator — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-25
```

## Findings

### [BLOCKING] doc.go still carries the dangling board's-sync.go cross-reference Card 2 required removed
**Location:** `internal/gitrepo/doc.go:112-113`
**Issue:** Card 2 explicitly requires rewording the `PushCoalesced` comment "This is the board sync.go push-loop replacement" so it no longer cites board's sync.go (the deleted `pushUnpushed`/`hasUnpushed` functions batch 2 removes). The literal phrase "This is the coalescing engine behind board's sync.go push-loop replacement." still appears verbatim in doc.go's Push-surface section — the reword was not applied (push.go's own PushCoalesced comment was correctly cleaned, but the same sentence survives in doc.go).
**Fix:** Reword doc.go's Push-surface closing sentence to describe `PushCoalesced`'s coalescing purpose on its own merit, without naming board or sync.go, per Card 2's requirement.

## Verdict

REQUEST_CHANGES
doc.go retains the exact dangling board-sync.go cross-reference Card 2 explicitly required removed.
MILL_REVIEW_END
