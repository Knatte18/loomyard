MILL_REVIEW_BEGIN
# Review: Extract shared primitives (paths, output) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-11
```

## Findings

### [NIT] config.md error-text framing could mislead implementer
**Location:** Batch 2 / Card 6
**Issue:** Card 6 tells the doc to say `FindBaseDir` "returns the `not initialized` error"; the doc's existing prose quotes the board-level message `not initialized here; run "mhgo init"`, while `FindBaseDir` actually returns `not initialized: _mhgo/ directory not found in %s`.
**Fix:** Have the doc distinguish `FindBaseDir`'s raw message from board's rewrapped one to avoid a wrong literal being copied in.

## Verdict

APPROVE
Plan is accurate, source-grounded, and behaviour-preserving; the single nit is non-blocking.
MILL_REVIEW_END
