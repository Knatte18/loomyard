MILL_REVIEW_BEGIN
# Review: Speed up git-fixture tests: bench, analyse, hardlink

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-13
```

## Findings

### [GAP] lyxtest's own package trips the hermetic guard
**Section:** Decisions › hermetic-guard-and-constraints-entry; Technical context (line 323)
**Issue:** `internal/lyxtest/lyxtest_test.go` is `package lyxtest` and spawns git directly (verified: many `exec.Command("git",…)`), so it needs the hermetic env — but its TestMain would call the helper *unqualified* (`HermeticEnv()`), while the guard's presence check is on the qualified `lyxtest.<Helper>` token; allowlisting lyxtest is wrong since it is the most git-heavy fixture builder and genuinely needs hermeticity.
**Fix:** State that the guard's helper token is the bare function name (matching both `HermeticEnv` and `lyxtest.HermeticEnv`), or explicitly carve lyxtest's self-reference case, so the most git-heavy package is not silently allowlisted out of hermeticity.

### [NOTE] hubgeometry TestMain must sit in the external test package
**Section:** Decisions › two-layer-hermetic-mechanism; Technical context (line 323)
**Issue:** lyxtest imports hubgeometry (Leaf Invariant), so a TestMain calling the lyxtest helper cannot live in an internal `package hubgeometry` test file without a cycle; hubgeometry's git-spawning tests are already `package hubgeometry_test` (verified), which is what makes this work.
**Fix:** Note in the plan that for hubgeometry (and any leaf lyxtest depends on) the hermetic TestMain goes in the external `_test` package to avoid the import cycle.

## Verdict

GAPS_FOUND
One guard-mechanism gap: lyxtest's own git-spawning package needs the helper-token check resolved.
MILL_REVIEW_END
