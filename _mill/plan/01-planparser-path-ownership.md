# Batch: planparser-path-ownership

```yaml
task: "planparser owns the plan-directory path"
batch: "planparser-path-ownership"
number: 1
cards: 2
verify: go test ./internal/planparser/...
depends-on: []
```

## Batch Scope

This batch makes `internal/planparser` the declarer of the plan directory's absolute path: it adds `PlanDir(anchorPath string) string` and `PlanOverview(anchorPath string) string` beside the package's existing plan-location symbols, covers them with a new untagged unit test, and records the widened package contract in `internal/planparser/doc.go`.
It is purely additive — nothing is deleted, no caller is repointed, and `internal/loomengine`'s twins keep working untouched — so the tree stays green at every commit and batch 2 can repoint callers onto a surface that already exists and is already tested.
The external interface batch 2 consumes is exactly those two exported functions.

**Batch-local decision — the TDD ordering happens inside card 1, not across two cards.**
`_mill/discussion.md`'s Testing section names the `planparser` path tests as the natural TDD entry (write the test, watch it fail to compile, add the functions).
That ordering is kept, but as two steps inside one card rather than two cards: a card that adds only the test would leave its own commit referencing undefined symbols, so `go test ./internal/planparser/...` could not pass at that commit.
Card 1 therefore carries both the test file and the functions, and its Requirements state the write-test-first order explicitly.

**Batch-local decision — no new import.** `path/filepath` and `internal/lyxdirs` are both already in `internal/planparser/parse.go`'s import block, so the two new functions add no import to the package.
Adding `internal/lyxcwd` is prohibited (see the overview's plain-string Shared Decision).

## Cards

### Card 1: add `PlanDir` and `PlanOverview` to `internal/planparser/parse.go`, test-first

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/planpath_test.go`
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/planparser/parse.go`
- **Creates:**
  - `internal/planparser/planpath_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Work in two steps, in this order, so the test is seen failing before the implementation exists.

  **Step 1 — write `internal/planparser/planpath_test.go` first.**
  It is an in-package test (`package planparser`), untagged (pure `filepath.Join` arithmetic, no spawning, no fixture tree, per the Test Tier Purity Invariant), importing only `path/filepath`, `testing`, and `github.com/Knatte18/loomyard/internal/lyxdirs`.
  It must NOT import `github.com/Knatte18/loomyard/internal/lyxcwd`.
  Give the file a header comment, above the `package` line, stating that it tests the told-anchor `PlanDir`/`PlanOverview` path constructors, that it is the ported successor of `internal/loomengine/planpath_test.go` (deleted in batch 2), and that it is untagged pure path arithmetic.
  Two test functions, both driving a nested-directory argument rather than a bare root — reuse the ported fixture's argument shape, `filepath.Join("home", "user", "repo", "sub", "dir")`:
  - `TestPlanDir` asserts `PlanDir(anchor)` equals `filepath.Join(anchor, lyxdirs.LyxDirName, "plan")`, deriving `_lyx` from `lyxdirs.LyxDirName` rather than a literal.
  - `TestPlanOverview` asserts `PlanOverview(anchor)` equals `filepath.Join(PlanDir(anchor), "00-overview.md")` AND that it equals the exact path `ParsePlan` reads for its overview, i.e. `filepath.Join(PlanDir(anchor), overviewFileName)` using the package's own unexported `overviewFileName` constant.
    Asserting both forms is the point of routing `PlanOverview` through `overviewFileName`: the literal-vs-constant agreement is what can never silently diverge again.

  The ported file's third case, `TestLocationPlanDir_UnanchoredEqualsWorktreePath`, is deliberately NOT ported — with a told string there is no unanchored-vs-anchored distinction left to assert inside this package.
  Do not add a replacement for it here;
  the anchoring proof lives at the call sites, added in batch 2.

  **Step 2 — add the two functions to `internal/planparser/parse.go`.**
  Insert them immediately after the existing `PlanDirRel` function and before the `cardIndexHeading` constant, so `overviewFileName`, `PlanDirName`, `PlanDirRel`, `PlanDir` and `PlanOverview` form one contiguous plan-location block.

  `PlanDir` is exactly `filepath.Join(anchorPath, lyxdirs.LyxDirName, PlanDirName)` — a character-for-character copy of `loomengine.PlanDir`'s current body with `l.AnchorPath()` replaced by the `anchorPath` parameter.
  Do not implement it as `filepath.Join(anchorPath, PlanDirRel())`: `PlanDirRel` is built with `path.Join` and is forward-slash by contract because it stamps `Card.SourcePath`, and building an OS path on top of it would couple two functions with deliberately different separator contracts.

  `PlanOverview` is exactly `filepath.Join(PlanDir(anchorPath), overviewFileName)`, reusing the existing unexported constant.
  Do not introduce a second declaration of the overview filename and do not export `overviewFileName`.

  Neither function validates its argument: no guard on an empty or relative `anchorPath`, no error return, both stay `func(string) string`.

  Doc comments start with the function name per `golang:golang-comments`, and each must state the anchor-always contract in its own words — that the caller supplies the absolute directory lyx is anchored at, which in a lyx worktree is `lyxcwd.Location.AnchorPath()` and never `WorktreePath()`, and that `planparser` is the sole declarer of this path.
  Carry forward the substance of the deleted twins' *"no other package may construct this path"* sentence, reworded for the told-anchor form.
- **Commit:** `feat(planparser): add told-anchor PlanDir and PlanOverview`

### Card 2: record path ownership in `internal/planparser/doc.go`

- **Context:**
  - `internal/planparser/parse.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/planparser/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one short `# `-headed section to the package doc comment stating that `planparser` owns *where* the plan directory is, not only what is inside it.
  Place it after the opening paragraph and before the existing `# Type model` section, so the ownership claim sits with the sole-parser claim it extends rather than buried among the format sections.

  It must say: `PlanDirName` and `PlanDirRel` declare the worktree-relative token (`_lyx/plan`, forward-slash, a document token used for `Card.SourcePath`); `PlanDir` and `PlanOverview` declare the absolute form; the package never resolves cwd and never imports `internal/lyxcwd`; and the caller supplies the anchor path — `lyxcwd.Location.AnchorPath()` in a lyx worktree, never `WorktreePath()`.
  Keep it to a few sentences — this is a pointer to the contract, not a restatement of the Cwd Resolution Invariant.

  Do not touch the existing `# Type model`, `# The root:/// resolution rule`, `# The none sentinel`, or `# Validation lives in validate.go` sections.
- **Commit:** `docs(planparser): record plan-path ownership in the package doc`

## Batch Tests

`verify: go test ./internal/planparser/...` runs the package's whole untagged suite: the new `planpath_test.go` plus the existing `parse_test.go`, `normalize_test.go`, `sections_test.go` and `validate_test.go`.
The batch is scoped to one package and adds no cross-package surface, so the package-scoped run is the right gate — the two new functions have no consumer until batch 2, and nothing in the package's existing behaviour is touched.

The new `planpath_test.go` is the batch's own coverage: it pins the join arithmetic (`_lyx` from `lyxdirs.LyxDirName`, `plan` from `PlanDirName`), pins `PlanOverview` as `PlanDir` plus the overview filename, and pins the agreement between `PlanOverview`'s filename and the `overviewFileName` constant `ParsePlan` reads.
It deliberately does not attempt anchoring coverage — with a told string there is nothing inside this package that can distinguish an anchor path from a worktree path.
That proof is added at the call sites in batch 2 (the subpath-anchored `loomengine.PlanSpec` case and the subpath-anchored `webstercli` `PersistentPreRunE` case).

`internal/planparser/doc.go` carries no runnable surface;
card 2's correctness is a review obligation, and `go vet` (via the task-wide done gate) catches a malformed doc comment.
