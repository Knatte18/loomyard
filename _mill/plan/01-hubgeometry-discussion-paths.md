# Batch: hubgeometry-discussion-paths

```yaml
task: 'loom: Discussion producer (interactive interview, auto-mode capable)'
batch: hubgeometry-discussion-paths
number: 1
cards: 2
verify: go test ./internal/hubgeometry/
depends-on: []
```

## Batch Scope

Add the `_lyx/discussion/` path accessors to `internal/hubgeometry` — the sole
owner of all `_lyx`/geometry paths (Hub Geometry Invariant). This is a small,
isolated leaf change with no dependency on the rest of the task. It delivers the
external interface batch 3's `DiscussionSpec` factory consumes: three
`Layout` methods returning the discussion directory and its two files. The
methods are `WorktreeRoot`-anchored (not `Cwd`-anchored), mirroring the existing
`LoomStatusFile()` — the discussion artifact is the one true per-worktree
artifact, so a caller invoked from a subdirectory must still resolve the single
`_lyx/discussion/` at the worktree root.

## Cards

### Card 1: Add discussion-path Layout accessors to hubgeometry

- **Context:**
  - `docs/reference/discussion-format.md`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three exported methods on `*Layout`, placed immediately
  after the existing `LoomStatusLock()` method, each with a godoc comment
  mirroring `LoomStatusFile()`'s style (including the "per the Hub Geometry
  Invariant, no other package may construct this path" and the WorktreeRoot-vs-Cwd
  anchoring note):
  - `func (l *Layout) DiscussionDir() string` returns
    `filepath.Join(l.WorktreeRoot, LyxDirName, "discussion")`.
  - `func (l *Layout) DiscussionDecisionRecord() string` returns
    `filepath.Join(l.DiscussionDir(), "decision-record.md")`.
  - `func (l *Layout) DiscussionSupportLog() string` returns
    `filepath.Join(l.DiscussionDir(), "support-log.md")`.
  Use the literal directory name `"discussion"` and the literal filenames
  `"decision-record.md"` / `"support-log.md"` (matching the pinned contract);
  reuse the existing `LyxDirName` constant. Do not add a package-level
  `DiscussionDir(baseDir string)` helper — the WorktreeRoot-anchored `Layout`
  method form is correct here (the `PlanDir(baseDir)` free-function form is for
  the Cwd/baseDir-parameterized callers; the discussion artifact follows
  `LoomStatusFile`'s method form).
- **Commit:** `feat(hubgeometry): add _lyx/discussion path accessors`

### Card 2: Unit-test the discussion-path accessors

- **Context:**
  - `internal/hubgeometry/loomstatus_test.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/hubgeometry/discussionpath_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New test file in `package hubgeometry` mirroring
  `loomstatus_test.go`'s construction of a `Layout` with a known `WorktreeRoot`
  (and a `Cwd` set to a subdirectory distinct from `WorktreeRoot`). Assert:
  `DiscussionDir()` returns `<WorktreeRoot>/_lyx/discussion`;
  `DiscussionDecisionRecord()` returns
  `<WorktreeRoot>/_lyx/discussion/decision-record.md`; `DiscussionSupportLog()`
  returns `<WorktreeRoot>/_lyx/discussion/support-log.md`. Include an assertion
  that the paths are anchored on `WorktreeRoot`, NOT `Cwd` — i.e. with
  `Cwd != WorktreeRoot`, the returned paths still resolve under `WorktreeRoot`
  (the same property `loomstatus_test.go` asserts for `LoomStatusFile`). Build
  expected paths with `filepath.Join` (not hard-coded slashes) so the test is
  OS-agnostic.
- **Commit:** `test(hubgeometry): cover _lyx/discussion path accessors`

## Batch Tests

`verify: go test ./internal/hubgeometry/` runs the whole `hubgeometry` package
test binary, which includes the new `discussionpath_test.go` plus the
`enforcement_test.go` invariant walk (confirming the new methods introduce no
banned `os.Getwd` / `git rev-parse` token). Scoped to the single package this
batch touches.
