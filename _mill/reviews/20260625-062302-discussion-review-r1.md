MILL_REVIEW_BEGIN
# Review: Move config templates home by removing the lyxtest->configreg edge

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\config-test-cleanup\_mill\discussion.md
date: 2026-06-25
```

## Findings

### [GAP] SeedConfig necessity not substantiated by the test
**Section:** Decisions / lyxtest-seed-helper, where-real-config-is-needed
**Issue:** `config.Edit` (internal/config/edit.go:88-104) scaffolds the missing `worktree.yaml` from the template, and TestE2ESyncIntegration's fake editor overwrites it with `validYAML` then asserts that exact editor content (configcli_integration_test.go:56,90) — it never reads the seeded weft-prime config, so a neutral `_lyx/config/` placeholder already makes the test pass without any real seeded config.
**Fix:** State why `SeedConfig`/real config is required despite scaffold-on-missing, or drop the helper and have the (now-only-needs-a-`_lyx/config/`-dir) fixture suffice; if kept, justify on faithfulness grounds explicitly rather than "this test needs real config."

### [NOTE] Placeholder path collides with an existing literal `_lyx/placeholder`
**Section:** Decisions / lyxtest-leaf; Technical context
**Issue:** The neutral placeholder is specified at `paths.ConfigDir(weftPrime)` = `_lyx/config/placeholder`, but weft/weft_integration_test.go:104 writes a raw-literal `filepath.Join(fixture.WeftPrime, "_lyx", "placeholder")` (`_lyx/placeholder`, not under `config/`) — a different path that also violates the path-helper constraint.
**Fix:** Note this consumer in the discussion: confirm the new placeholder location does not collide/confuse it, and flag that line for path-helper cleanup (it qualifies for the tidy/constraint scope).

### [NOTE] configreg internal test already imports weft — confirm post-revert cycle-safety
**Section:** Decisions / configreg-references-features
**Issue:** configreg_test.go is `package configreg` and imports `weft` (line 8); after the revert, production `configreg` also imports `weft`/`worktree`/`board`, so the internal test shares the feature import — fine, but the discussion's cycle analysis only enumerates production importers, not this test edge.
**Fix:** Add a one-line note that `configreg`'s own internal test importing a feature package is harmless (features do not import `lyxtest` from non-test code, and `configreg` is not reached by feature tests).

## Verdict
GAPS_FOUND
The seed-helper's stated necessity is contradicted by the test's scaffold-and-overwrite flow; resolve before planning.
MILL_REVIEW_END
