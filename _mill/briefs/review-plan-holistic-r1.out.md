Confirmed: there is a canonical verify-shape convention. Every non-null `verify:` MUST start with the literal token `PYTHONPATH=` followed by a space. The plan's verify commands are `go test ./internal/worktree/` — missing the mandated `PYTHONPATH= ` prefix. This is a constraint/convention violation enforced by `_plan_validate.verify-not-isolated`.

Additionally, the integration-tag reachability issue remains: `go test ./internal/worktree/` does not pass `-tags integration`, so the new Card 3 tests (in integration-tagged files) won't run.

Let me confirm the existing test files truly require the tag and that there are no non-tagged test files in the package that the verify would otherwise compile. All 7 test files in the package are integration-tagged (from earlier grep, all matched `go:build integration`). So `go test ./internal/worktree/` with no tags compiles zero test files — the entire batch's test surface is unreachable under the stated verify. This is unambiguously BLOCKING per the "Integration test reachability" criterion.

I have enough to produce the review.

MILL_REVIEW_BEGIN
# Review: Ensure weft branches are orphan branches — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [BLOCKING] Verify never runs the integration-tagged tests
**Location:** 00-overview.md batch index `verify:`; 01 frontmatter + Batch Tests
**Issue:** Every test file in `internal/worktree` (incl. `weft_test.go`, `add_test.go` where Card 3 adds tests) carries `//go:build integration`, but `verify: go test ./internal/worktree/` passes no `-tags integration`, so it compiles zero test files — Card 3's entire surface is unreachable.
**Fix:** Set `verify` to run with `-tags integration` (e.g. `go test -tags integration ./internal/worktree/`).

### [BLOCKING] verify command violates canonical PYTHONPATH= shape
**Location:** 00-overview.md batch index; 01 frontmatter
**Issue:** `mill-config.yaml` (lines 196-208) mandates every non-null `verify:` start with the literal token `PYTHONPATH=` then a space, enforced by `_plan_validate.verify-not-isolated`; both `verify:` values omit it.
**Fix:** Prefix both with `PYTHONPATH= ` (e.g. `PYTHONPATH= go test -tags integration ./internal/worktree/`).

### [NIT] Card 3 subtask test must seed weft branch Y before start-point use
**Location:** 01 Card 3, test #2
**Issue:** `createWeftWorktree` will run `git worktree add -b <new> <path> Y` in `WeftRepoRoot()`; start-point `Y` only resolves if weft branch `Y` already exists. The card does describe creating weft-`Y` and advancing its tip, but does not state the missing-parent-weft-branch failure path that `discussion.md` Testing calls out.
**Fix:** Add the "missing parent weft branch ⇒ clear error + full rollback" case (discussion Testing item) as a Card 3 sub-case, or note its deferral.

### [NIT] Card 4 docs anchor uses a stale subsection heading style
**Location:** 01 Card 4
**Issue:** `docs/overview.md` `## Weft overlay model` exists (line 87) but already contains many `###` subsections; the new `### Branch model` fits, yet the card should state placement relative to `### Junction model` / `### Weft suffix convention` to avoid an ambiguous insert point.
**Fix:** Specify the new `### Branch model` is added under `## Weft overlay model`, after `### Junction model`.

## Verdict

REQUEST_CHANGES
Verify command runs no tests and breaks the mandated PYTHONPATH= shape.
MILL_REVIEW_END