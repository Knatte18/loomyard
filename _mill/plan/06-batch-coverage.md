# Batch: batch-coverage

```yaml
task: "Centralize glyph ref-shape enumeration"
batch: "batch-coverage"
number: 6
cards: 2
verify: go test ./internal/planparser/ ./internal/planglyph/
depends-on: [4]
```

## Batch Scope

This batch is the family-2 hardening from `_mill/discussion.md`'s Decision: batch-coverage-disposition: add the one missing per-key answer-coverage guard on `drift.go`'s post-repair resolve, and pin the two existing batched-boundary chokepoints (`repo.Resolve` inside `resolveTargets`, `quarry.Name` inside `CanonicalizeHandles`) with an AST test so a future call site cannot bypass their guards.
It is one batch because both cards are the coverage family's disposition and neither overlaps batch 5's files.
This carries the plan's second sanctioned behavior change (the drift guard), landing with its own two-sided regression tests.

## Cards

### Card 12: Per-key answer-coverage guard in drift's post-repair resolve

- **Context:**
  - `internal/planglyph/donecheck.go`
  - `internal/planglyph/repo.go`
  - `internal/planglyph/planglyph.go`
- **Edits:**
  - `internal/planglyph/drift.go`
  - `internal/planglyph/drift_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/planglyph/drift.go`'s post-repair block, bind the requested batch to a variable (`targets := collectGlyphTargets(reloaded, lang)`) before the `resolveTargets` call, then guard answer coverage per key, mirroring `doneCheckVerdicts`' per-key guard (`internal/planglyph/donecheck.go`) and split out for unit-testability the way `ensureResolveCoverage` (`internal/planglyph/repo.go`) is:
  add `ensurePostRepairCoverage(targets []string, results []quarry.ResolveResult) error`, which indexes the results by `Target` and returns an `ErrQuarryUnavailable`-wrapped error naming the first unanswered target; call it right after `resolveTargets` returns, propagating the error from inside the language-gated block so the function returns at the resolve boundary exactly like the sibling transport-error path — before the amendment loop, so no amendment is appended on a coverage miss.
  The guard's scope is answer coverage only: it asserts every target actually requested in the post-repair batch received an answer.
  An introduced glyph absent from the batch entirely (an unlanded substitution, or a `newID` that fails `glyph.Parse` inside `collectGlyphTargets`) keeps today's silent behavior deliberately — a glyph absent from the reloaded plan cannot drift, and whatever is in the plan is governed by ordinary validation; carry that rationale into the guard's comment.
  Amend the existing comment above the amendment loop ("the amendments below are still appended") to say the amendments are appended regardless of findings, never on an infrastructure error, so it stops contradicting the new early return.
  Regression tests in `internal/planglyph/drift_test.go` (new test functions; existing assertions untouched), driving `ensurePostRepairCoverage` directly:
  one asserts a requested-but-unanswered target errors with `errors.Is(err, ErrQuarryUnavailable)` and the target named in the message;
  the other asserts a result set fully covering the requested targets returns nil even when other glyphs exist outside the target list — the introduced-glyph-absent-from-batch case stays a silent pass.
- **Commit:** `planglyph: guard drift's post-repair resolve with per-key answer coverage`

### Card 13: Chokepoint pin test for repo.Resolve and quarry.Name

- **Context:**
  - `internal/planglyph/repo.go`
  - `internal/planglyph/handle.go`
  - `internal/cliwire/bannedecl_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/planglyph/chokepoint_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/planglyph/chokepoint_enforcement_test.go` per the overview's Decision: ast-enforcement-idiom, scanning every production `.go` file in `internal/planglyph` and asserting two pins:
  1. Every `ast.CallExpr` whose selector `Sel` is exactly `Resolve` sits inside the function `resolveTargets` (`internal/planglyph/repo.go`) — the deliberately conservative any-receiver match is documented in the test's comment: `resolveTargets` owns the `ensureResolveCoverage` length guard, and a second `Resolve` call site of any kind is exactly the bypass this pin exists to catch.
  2. Every `ast.CallExpr` whose selector is the package-qualified `quarry.Name` sits inside the function `CanonicalizeHandles` (`internal/planglyph/handle.go`), which owns the length-plus-echo guard.
  The failure message instructs: route new resolve calls through `resolveTargets` and new naming calls through `CanonicalizeHandles`, or move the coverage guard with the call — never add an unguarded batched quarry boundary.
  Include the seeded self-test (synthetic source strings with an out-of-place `x.Resolve(...)` call and an out-of-place `quarry.Name(...)` call; assert the matcher fires on both), then assert zero hits on the real tree.
- **Commit:** `planglyph: pin repo.Resolve and quarry.Name to their guarded chokepoints`

## Batch Tests

`verify: go test ./internal/planparser/ ./internal/planglyph/` — card 12's two new drift tests carry the behavior change's both halves (coverage miss errors, full coverage passes); the chokepoint pin test asserts the two-call state of the real tree; the integration-tagged drift suite is exercised by the configured done gate at Handoff, not by this per-batch verify.
