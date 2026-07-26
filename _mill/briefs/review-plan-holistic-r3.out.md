MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft — holistic

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewed_file: plan/
date: 2026-07-26
```

## Findings

### [NIT] Stale warp/weft example in configreg struct comment
**Location:** Batch B, card 9 (`internal/configreg/configreg.go`)
**Issue:** Card 9 sweeps only the package-comment `(board, warp, weft)`, but the `Module.Name` field doc-comment `// Name is the module identifier (e.g., "board", "warp", "weft")` still cites the removed config identifiers; the card already edits this file and the card-27 grep gate won't catch bare `warp`/`weft` words, so it silently survives.
**Fix:** Have card 9 also reword the `Module.Name` field comment to a still-valid example (e.g. `"board", "fabric"`).

## Verdict

APPROVE
Sound, source-grounded, acyclic DAG, sequential numbering; one cosmetic stale-comment NIT.
MILL_REVIEW_END
