MILL_REVIEW_BEGIN
# Review: Move config templates home by removing the lyxtest->configreg edge — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [NIT] Neutral fixture placeholder path diverges from test literal
**Location:** Batch 1 Card 1 / Batch 3 Card 12
**Issue:** Card 1 writes the placeholder at `paths.ConfigDir(weftPrime)/placeholder` (i.e. `_lyx/config/placeholder`), but `TestRunCLI_EnvMapToOption` (line 104) and Card 12 reference `_lyx/placeholder` (bare, not under `config/`); these are two different files. It works only because the test creates the file it writes, so the latent inconsistency is invisible until someone reads the fixture.
**Fix:** Note in Card 1 (or Card 12) that the fixture placeholder and the test's `_lyx/placeholder` literal are intentionally distinct paths, or align them.

### [NIT] Card 4 says `worktree.ConfigTemplate()` for in-package test files
**Location:** Batch 1 Card 4
**Issue:** add_test.go / remove_test.go / weft_test.go are `package worktree` (internal), so the call is bare `ConfigTemplate()`, not `worktree.ConfigTemplate()`; the qualified name would not compile from inside the package.
**Fix:** Clarify the call is unqualified `ConfigTemplate()` for the `package worktree` test files (the qualified form applies only to external-test or cross-package sites).

### [NIT] Card 5 godoc/placeholder source not in Context
**Location:** Batch 1 Card 5
**Issue:** Card 5 asserts the neutral fixture contains `paths.ConfigDir(...)/placeholder`, a fact defined in `lyxtest.go` (Card 1), but Context lists only `paths.go`. Same-package implicit read covers it, so this is non-blocking.
**Fix:** Optionally add `internal/lyxtest/lyxtest.go` to Card 5 Context for explicitness.

## Verdict

APPROVE
Cycle ordering, leaf invariant, paths-helper discipline, DAG, and numbering are all sound; findings are cosmetic.
MILL_REVIEW_END
