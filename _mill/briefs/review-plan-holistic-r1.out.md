MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-28
```

## Findings

### [BLOCKING] Batch 2 verify omits `-tags integration` for the integration-tagged file it edits
**Location:** Batch 2 (`02-hubgeometry-pattern-surface.md`), cards 4 and 5
**Issue:** `internal/hubgeometry/hubgeometry_test.go` begins with `//go:build integration` (confirmed in source), but batch 2's `verify: go test ./internal/hubgeometry/... ./cmd/lyx/...` (both in this batch file and in `00-overview.md`'s Batch Index) carries no `-tags integration`. A plain `go test` excludes that file from the build entirely, so card 4's new `HostJunctionsHere()` assertions and card 5's new `IsReservedHubName("_pattern")` assertion — both added to that exact file — never compile or run under this batch's own verify command; a bug or compile error in either would go undetected until batch 5's wider `-tags integration` run, three batches later, by which point it is entangled with the two-junction flip's much larger diff. The batch's own "Batch Tests" section compounds this by asserting the opposite: "`verify: ...` covers the two test files this batch edits (`internal/hubgeometry/hubgeometry_test.go`, ...)" — false, since that file is excluded from the build in that mode. `cmd/lyx`'s guards (`tierpurity_test.go`/`hermeticenv_test.go`) don't help either: they text-scan file bytes, never compile them.
**Fix:** Add `-tags integration` to batch 2's `verify:` in both `02-hubgeometry-pattern-surface.md` and `00-overview.md`'s Batch Index (Go's tags are additive, so this still runs the untagged tier too), or move cards 4/5's new assertions into a fresh untagged test file if the intent was to keep this batch spawn-free.

### [NIT] Card 28's "drive it through PlanSpec the way the existing tests there do" undersells the fixture change needed
**Location:** Batch 7 (`07-prompt-wiring.md`), card 28
**Issue:** `internal/loomengine/plan_test.go`'s existing tests build `&hubgeometry.Layout{WorktreeRoot: filepath.Join("home", "user", "repo")}` — a path that never exists on disk — purely for string-shape assertions. `pattern.Directive`'s active check is a real `os.Stat` on `_pattern/PATTERN.md`, so reusing that same fake Layout for the "non-empty directive" positive case would always render empty, not prove the placement assertion. This differs from cards 24/25/26/27's sibling tests, which inject `pattern_directive` directly as a stencil value and never touch a real filesystem.
**Fix:** State explicitly that the non-empty-directive case needs a `t.TempDir()`-backed Layout with a real `_pattern/PATTERN.md` seeded on disk, unlike this file's other existing tests.

## Verdict

REQUEST_CHANGES
Fix batch 2's verify/build-tag mismatch (BLOCKING); the rest of the plan is unusually well source-grounded.
MILL_REVIEW_END
