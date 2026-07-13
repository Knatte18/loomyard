MILL_REVIEW_BEGIN
# Review: Restore the Tier 1 floor: guards + perchengine — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-13
```

## Findings

### [NIT] Cause-section table cross-reference doesn't resolve
**Location:** `docs/benchmarks/test-suite-timing.md:61-64` (Cause, lever a)
**Issue:** The Cause section says the `internal/clihelp` 8.0 s → 0.46 s effect is shown "in this run's isolated Tier 1 elapsed, median-run table below," but the "Tier 1: where the time goes" table (lines ~122-128) never lists `internal/clihelp` as its own row — it's folded into the "everything else, < 1.8 s each" bucket.
**Fix:** Either add an explicit `internal/clihelp` row to the Tier 1 table or drop the "median-run table below" pointer from the Cause bullet.

### [NIT] "new since" attribution is off by one block
**Location:** `docs/benchmarks/test-suite-timing.md:126`
**Issue:** The new "Current best times" Tier 1 table labels `internal/builderengine` as "new since the 2026-07-13 hermetic-git-env block," but that same hermetic-git-env block (now frozen History, lines ~221-224) already reports `internal/builderengine`/`internal/buildercli` as new relative to 2026-07-12 — so it was already present as of 07-13, not new since it.
**Fix:** Reword to "new since 2026-07-12" or drop the "new" qualifier, since the frozen 07-13 block already lists it.

## Verdict

APPROVE
Plan cards fully realised; constraints and shared decisions honoured; two doc-only NITs, no blocking issues.
MILL_REVIEW_END
