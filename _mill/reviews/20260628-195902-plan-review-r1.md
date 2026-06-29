I have reviewed the overview, the single batch, and all referenced source files. The plan is well-grounded against the source. Two minor findings.

MILL_REVIEW_BEGIN
# Review: Sandbox test-suite launcher and task harvester — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-28
```

## Findings

### [NIT] Cards 2 and 3 are mutually compile-dependent
**Location:** Batch 1, Cards 2 and 3
**Issue:** Card 2's `run` calls `runSuite` (created in Card 3) while Card 3's `runSuite`/`suite.go` uses `hostDirName` (the const Card 2 adds to `main.go`), so neither card compiles standalone in commit order; only the batch as a whole builds.
**Fix:** Sequence Card 3 (create `suite.go`) before Card 2 and move the `hostDirName` const addition into Card 3, so each commit leaves `go build ./tools/sandbox` green; batch verify is unaffected either way.

### [NIT] `//go:embed` into a string var needs the `embed` import
**Location:** Batch 1, Card 3
**Issue:** `//go:embed test-scheme.md` bound to `var testSchemeMD string` fails to compile unless the file blank-imports `embed`; the card text omits this.
**Fix:** Note in Card 3 Requirements that `suite.go` must add `import _ "embed"`.

## Verdict

APPROVE
Plan is constraint-clean, complete, and source-grounded; both findings are minor NITs.
MILL_REVIEW_END
