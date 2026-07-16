MILL_REVIEW_BEGIN
# Review: Built-in operator console pane in mux — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-16
```

## Findings

### [NIT] clampHeaderHeight can clamp the header to zero rows
**Location:** `internal/muxengine/render/height.go:23-39`, `internal/muxengine/render/layout.go:65-78`
**Issue:** In a pathological config (`height_rows` large relative to a tiny `mux.yaml` `height`), `clampHeaderHeight` can legally return 0, and `bandHeader` would then emit a `WxH,...` header cell with `H=0` into the `window_layout` string — untested against a real tmux/psmux `select-layout` call for whether a zero-height cell is accepted.
**Fix:** Either floor the header cell at 1 row when a header pane id is present (never fully starving it), or add a real-tmux contract assertion (`contract_integration_test.go`) pinning that a zero-height cell is accepted/rejected, so the documented "clamped to zero" behavior (height.go's doc comment, `TestClampHeaderHeight`'s `WindowTooShortForBoth` case) is verified against the actual multiplexer, not just the pure unit test.

## Verdict

APPROVE
All four batches match their plan cards, shared decisions, and CONSTRAINTS.md exactly; no blocking issues found.
MILL_REVIEW_END
