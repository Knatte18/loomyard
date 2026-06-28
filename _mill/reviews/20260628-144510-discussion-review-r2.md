MILL_REVIEW_BEGIN
# Review: CLI help & error ergonomics from sandbox run

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-28
```

## Findings

### [GAP] Aggregate `config --print` missing-file behavior undefined
**Section:** Decisions › W12
**Issue:** `lyx config --print` (no arg) "prints all known modules' files," but config files are created lazily per-module (`config.Edit` writes from template on first edit), so in a real repo some modules' `_lyx/config/<module>.yaml` will be absent — the aggregate path's partial-failure semantics (error on first missing vs. skip vs. emit a "not initialized" header) are unspecified, yet the test "dumps all modules" needs deterministic behavior.
**Fix:** State whether the aggregate dump skips absent files (with a delimiter note) or errors, distinct from the single-module case which already errors (exit 1) on a missing file.

### [NOTE] Root path needs `SilenceErrors=true` for W14, not just the seam
**Section:** Decisions › W14
**Issue:** `main()`/`run()` execute via `root.ExecuteContext` directly, not `clihelp.Execute`, and `newRoot()` explicitly sets `SilenceErrors: false` (`cmd/lyx/main.go:85`); the W14 decision flips it in the seam but does not call out flipping it on the root, so the root error path would double-emit (cobra plain text + JSON wrapper).
**Fix:** Note that `newRoot()`'s `SilenceErrors` must become `true` alongside the shared root-wrapping helper.

## Verdict

GAPS_FOUND
One undefined failure mode in the W12 aggregate-print path blocks plan writing.
MILL_REVIEW_END
