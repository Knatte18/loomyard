# Batch: plan-path-helpers

```yaml
task: 'loom: Planner producer'
batch: 'plan-path-helpers'
number: 1
cards: 1
verify: go test ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

Adds the two WorktreeRoot-anchored `Layout` accessors the Planner producer's `PlanSpec`
factory needs — `PlanDir()` and `PlanOverview()` — to `internal/hubgeometry`, mirroring the
existing `DiscussionDir()` / `DiscussionDecisionRecord()` accessors, and fixes the stale
"plan-format v1" wording in the existing free `PlanDir(baseDir)` function's godoc. This is a
separate batch (and the DAG root) because it lives in a different module than the producer and
`PlanSpec` (batch 2) calls `layout.PlanOverview()`, so the accessor must exist first. The
external interface batch 2 consumes: `Layout.PlanOverview()` returns
`<WorktreeRoot>/_lyx/plan/00-overview.md` and `Layout.PlanDir()` returns
`<WorktreeRoot>/_lyx/plan`. All decisions inherit from `## Shared Decisions`
(`hubgeometry-owns-plan-paths`); no batch-local decisions.

## Cards

### Card 1: Layout.PlanDir / PlanOverview accessors + PlanDir godoc fix

- **Context:**
  - `internal/hubgeometry/discussionpath_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/hubgeometry/planpath_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/hubgeometry/hubgeometry.go`, add two `*Layout` methods
  immediately after the existing free `func PlanDir(baseDir string) string` (around line 229),
  mirroring the `DiscussionDir()` / `DiscussionDecisionRecord()` pattern (around lines 401–415):
  (1) `func (l *Layout) PlanDir() string` returning `PlanDir(l.WorktreeRoot)` — delegate to the
  existing free function so the `_lyx/plan` path has one definition; (2)
  `func (l *Layout) PlanOverview() string` returning
  `filepath.Join(l.PlanDir(), "00-overview.md")`. Both are WorktreeRoot-anchored (never `Cwd`),
  matching `DiscussionDir`'s doc rationale, and each carries a godoc comment stating the return
  value and the WorktreeRoot anchoring, and noting per the Hub Geometry Invariant that no other
  package may construct this path. The `00-overview.md` literal must live here, not in
  `loomengine`. Also fix the existing free `PlanDir(baseDir string)` godoc: it currently says
  "builder's plan-format **v1** artifacts" — replace the version-pinned wording with
  format-agnostic wording (the directory holds the plan's `00-overview.md` + per-card files and
  is shared across plan-format versions), keeping the rest of the comment and the
  `Returns filepath.Join(baseDir, LyxDirName, "plan").` line intact. Do NOT change the free
  function's signature or return value. Then create
  `internal/hubgeometry/planpath_test.go` mirroring `discussionpath_test.go` exactly — same
  `package hubgeometry`, same imports (`path/filepath`, `testing`), same hand-built `Layout`
  with `Cwd` deliberately differing from `WorktreeRoot` to prove Cwd is ignored — with:
  `TestLayoutPlanDir` asserting `l.PlanDir() == filepath.Join(l.WorktreeRoot, LyxDirName, "plan")`;
  `TestLayoutPlanOverview` asserting
  `l.PlanOverview() == filepath.Join(l.WorktreeRoot, LyxDirName, "plan", "00-overview.md")`; and
  a `TestLayoutPlanDir_CwdEqualsWorktreeRoot` variant like `discussionpath_test.go`'s. The test
  must be pure path arithmetic — no git spawn, no `exec.Command`, no fixture copy (Test Tier
  Purity Invariant); it is untagged Tier-1 and needs no `TestMain` (hubgeometry already has one).
  Follow the repo's godoc conventions (see `golang-comments` skill).
- **Commit:** `feat(hubgeometry): add Layout.PlanDir/PlanOverview accessors`

## Batch Tests

`go test ./internal/hubgeometry/...` compiles the package and runs the new pure-path tests in
`planpath_test.go` plus the existing enforcement/geometry tests (including
`TestEnforcement_GeometryLiterals`, which confirms the new methods introduce no illegal geometry
literal outside the allowed sites). Scope is the single module the batch touches.
