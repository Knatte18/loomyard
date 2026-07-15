MILL_REVIEW_BEGIN
# Review: Reconsider whether lyx mux needs anchor:top at all — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-15
```

## Findings

### [BLOCKING] Missed consumer: shuttlecli/run.go --anchor help
**Location:** whole plan (All Files Touched; batch 1)
**Issue:** `internal/shuttlecli/run.go:149` registers `--anchor` with usage `"placement: top|below-parent|hidden"` and is a live CLI consumer whose strand flows through the (card-3) engine `validateAnchor` that will now reject `top`; the file appears in no card and is absent from All Files Touched, so `top` survives as a stale, self-contradicting help string (CLI/Cobra "stale help is review-blocking" + discussion "no dead surface survives").
**Fix:** Add a batch-1 card editing `internal/shuttlecli/run.go` to change the `--anchor` usage to `placement: below-parent|hidden`, and extend batch 1's `verify:` to also run `go test ./internal/shuttlecli/...`.

### [NIT] Residual top_band_rows in mux review prompt
**Location:** batch 3 / card 14 (`docs/reviews/mux-review-prompt.md`)
**Issue:** Card 14 sweeps lines 109-111, 243-245, 256, 279 but leaves the `top_band_rows` config-key mention at line 155 ("Config is honored (`top_band_rows`/`collapsed_strip_rows` scale the layout)"), a now-nonexistent key.
**Fix:** Have card 14 also drop `top_band_rows` from line 155 (or note it as retained historical narrative and justify).

## Verdict

REQUEST_CHANGES
A live shuttle CLI consumer still advertises removed `--anchor top`; plan omits the file entirely.
MILL_REVIEW_END
