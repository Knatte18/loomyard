MILL_REVIEW_BEGIN
# Review: Prune and consolidate the test suite (board first) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [BLOCKING] preserve-names-as-subtests violated in TestLayoutChecksum

**Location:** `internal/muxpoc/cmd_test.go:15-18`
**Issue:** The table rows use `"TestLayoutChecksumMatchesPsmux_case1"`, `"TestLayoutChecksumMatchesPsmux_case2"`, and `"arbitrary"` as subtest names; neither original top-level name (`TestLayoutChecksumMatchesPsmux`, `TestLayoutChecksumIsFourHexDigits`) appears as a subtest name, violating the shared `preserve-names-as-subtests` decision and Card 14's explicit requirement ("named after `TestLayoutChecksumMatchesPsmux`").
**Fix:** Rename the two pinned rows to `"TestLayoutChecksumMatchesPsmux"` (or a single row named `TestLayoutChecksumMatchesPsmux` with both pinned values) and name the format row `"TestLayoutChecksumIsFourHexDigits"`, so that `-list '.*'` produces both original names as surviving subtests.

### [BLOCKING] Equivalence guardrail name-map contains incorrect subtest paths

**Location:** `docs/benchmarks/test-suite-timing.md:124,171-200`
**Issue:** Multiple entries strip the `Test` prefix from the actual subtest name (e.g. `TestLoadConfig/AbsolutePathPassthrough` when the code runs as `TestLoadConfig/TestAbsolutePathPassthrough`; `TestLoadConfig/DefaultWhenNoYAML` when the actual path is `TestLoadConfig/TestLoadConfig_DefaultWhenNoYAML`; `TestPickColor/NeverReturnsGreen` when code uses `"TestPickColorNeverReturnsGreen"`; `TestEnvFiltering/SanitizeEnv` when code uses `t.Run("TestSanitizeEnv",…)`; same pattern for `TestSpawn/*`, `TestRunCLIErrors/*`, `TestPushIntegration/CommitLandsOnBare`, etc.). The guardrail is meant to be auditable by diffing `-list` output; wrong paths make it non-auditable.
**Fix:** Correct every mapped path to match the actual `t.Run` string literal in the source: the subtest path is `<ParentFunc>/<name-field-verbatim>`.

### [NIT] TestRenderSpecialBucketTask absent from the doc name-map

**Location:** `docs/benchmarks/test-suite-timing.md:123-156`
**Issue:** `TestRenderSpecialBucketTask` is in the board baseline and is folded into `TestRenderProposalAndShapesHomepage` (row `name: "TestRenderSpecialBucketTask"`), but the doc's board folded-names list omits it.
**Fix:** Add `TestRenderSpecialBucketTask → TestRenderProposalAndShapesHomepage/TestRenderSpecialBucketTask` to the board folded-names section.

## Verdict

REQUEST_CHANGES
Two blocking issues: preserve-names-as-subtests violated in muxpoc cmd_test.go; doc equivalence guardrail contains systematically wrong subtest paths across five packages.
MILL_REVIEW_END