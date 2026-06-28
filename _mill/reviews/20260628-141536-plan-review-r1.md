I have verified the plan against all referenced source files. The plan is well-grounded: the seam pattern matches `gitexec.RunGit`/`warp.go`, the white-box test precedent matches `board/skipenv_internal_test.go`, the registration-test AST approach mirrors `enforcement_test.go`, and all 8 existing `Command()` packages are already registered in `newRoot()` (so the new registration guard passes on existing code). Context fields are complete; the DAG is acyclic with both batch files present.

A few minor issues below.

MILL_REVIEW_BEGIN
# Review: ghissues module — file LoomYard bugs as GitHub issues — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-28
```

## Findings

### [NIT] JSON number decodes as float64, not int
**Location:** Batch 1 / Card 3 (number-parsing assertions)
**Issue:** `output.Ok` emits `number` as an int, but decoding the envelope into `map[string]any` yields `float64`; asserting `== 123` against a Go `int` will fail.
**Fix:** State that the test compares against `float64(123)` (or decodes via a typed struct), so the implementer does not assert an int literal.

### [NIT] gh-not-found branch also catches generic exec failures
**Location:** Batch 1 / Card 1 (`createIssue` error handling)
**Issue:** `realRunGH` returns `exitCode == -1` for both the `LookPath` miss and any non-`ExitError` run failure (mirroring `gitexec`), so a rare exec failure after a successful `LookPath` is mislabeled "gh not found on PATH".
**Fix:** Note this is acceptable, or have `realRunGH` distinguish the LookPath error from a Run error so `createIssue` can word the two cases separately.

### [NIT] overview.md "Module dispatch" is stale (switch, not cobra)
**Location:** Batch 2 / Card 8 (docs/overview.md edit)
**Issue:** `docs/overview.md` lines 180-209 still document the pre-cobra `switch module` dispatch and `RunCLI` routing, which no longer reflects `newRoot()`; Card 8 only adds `ghissues` to the module enumeration.
**Fix:** Out of strict scope, but consider having Card 8 also note/refresh that the dispatch is the cobra root, since the implementer is already editing this file's module list.

## Verdict

APPROVE
Plan is constraint-clean, well-sequenced, and source-grounded; only minor test/doc nits remain.
MILL_REVIEW_END