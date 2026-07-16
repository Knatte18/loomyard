# Batch: hubgeometry-repo

```yaml
task: "Built-in operator console pane in mux"
batch: hubgeometry-repo
number: 1
cards: 2
verify: go test ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

Adds a git-derived `Repo` name to `hubgeometry.Layout` so the downstream `repo` token
(batch 2) is always resolvable without an extra git spawn. `Repo` is derived spawn-free from
the already-computed `Prime` (main worktree path), with a fallback to `WorktreeRoot` when
`Prime` is empty. This is the foundation batch — `internal/tokenvocab` (batch 2) reads
`layout.Repo`. External interface consumed next: the new `Layout.Repo` field.

## Cards

### Card 1: Add `Layout.Repo` field and spawn-free derivation

- **Context:**
  - `internal/hubgeometry/hubgeometry_unit_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `Repo string` field to the `Layout` struct (documented in the struct's
  godoc block as "the repository name, `filepath.Base(Prime)`, or `filepath.Base(WorktreeRoot)` when
  no main worktree is resolved"). Add a pure package-level helper
  `func deriveRepo(prime, worktreeRoot string) string` that returns `filepath.Base(prime)` when
  `prime != ""`, else `filepath.Base(worktreeRoot)` — never returns `"."` for a real worktree
  because `worktreeRoot` is the non-empty git toplevel. In `Resolve`, after `prime` is computed
  (the `for _, entry := range entries` loop that sets `prime`), set
  `Repo: deriveRepo(prime, workTreeRoot)` in the returned `&Layout{...}` literal. Do not add any new
  `git`/`exec` call — reuse the existing `prime` value.
- **Commit:** `feat(hubgeometry): add git-derived Repo field to Layout`

### Card 2: Unit + integration coverage for `Repo`

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/testmain_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry_unit_test.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `hubgeometry_unit_test.go` add a table-driven test for `deriveRepo` covering:
  non-empty `prime` yields `filepath.Base(prime)`; empty `prime` falls back to
  `filepath.Base(worktreeRoot)`; a trailing-slash path is handled by `filepath.Base`. Build inputs as
  plain strings — do NOT call `Resolve` (Test Tier Purity: untagged tests spawn nothing). In
  `hubgeometry_test.go` (the git-spawning, hermetic-`TestMain` package tests) extend an existing
  `Resolve` success assertion to also assert `layout.Repo == filepath.Base(layout.Prime)` for a
  resolved real worktree.
- **Commit:** `test(hubgeometry): cover Repo derivation and empty-Prime fallback`

## Batch Tests

`verify: go test ./internal/hubgeometry/...` runs the whole hubgeometry package. Card 2's
`deriveRepo` unit test is untagged/fast (no spawn); the `Resolve`-sets-`Repo` assertion runs in the
existing hermetic-`TestMain` test path. No new build tags introduced.
