# Batch: paths-weft-geometry

```yaml
task: 'weft engine: paths geometry, paired worktrees, lyx weft'
batch: paths-weft-geometry
number: 1
cards: 4
verify: go test ./internal/paths/
depends-on: []
```

## Batch Scope

This batch lands the host↔weft path math in `internal/paths` — the foundation both the weft module (batch 2) and the paired-spawn changes (batch 3) build on. It adds eight `Layout` methods (six weft-side targets + two host-side junction-link helpers) and updates the geometry method lists in `CONSTRAINTS.md` and `docs/overview.md` so the path-invariant documentation stays accurate. **External interface consumed downstream:** `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`. Pure geometry, no git, no I/O — fully unit-testable. Batch-local decision: the new methods mirror the existing `WorktreePath`/`PortalTarget` RelPath-mirroring convention exactly.

## Cards

### Card 1: Add weft + host-link geometry methods to Layout

- **Context:**
  - `internal/paths/worktreelist.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add eight methods on `*Layout`, each a pure `filepath.Join` expression, placed near the existing `WorktreePath`/`PortalTarget` methods with godoc comments matching the file's style:
  Add a godoc note to the file header or the new methods clarifying that the host link and the weft target for the same `slug` (`HostLyxLink(slug)` ↔ `WeftLyxDirFor(slug)`) are the two ends of the seeded junction.
  - `WeftRepoRoot() string` → `filepath.Join(l.Hub, l.PrimeName()+"-weft")` — the weft Prime worktree (the `git -C` target for weft `worktree add/remove`).
  - `WeftWorktreePath(slug string) string` → `filepath.Join(l.Hub, slug+"-weft")` — parallel to `WorktreePath(slug)`.
  - `WeftWorktree() string` → `filepath.Join(l.Hub, filepath.Base(l.WorktreeRoot)+"-weft")` — the weft worktree paired with the current host worktree.
  - `WeftLyxDir() string` → `filepath.Join(l.WeftWorktree(), l.RelPath, "_lyx")` — the current worktree's weft `_lyx` (junction target for `lyx weft`, pathspec base), RelPath-mirrored like `PortalTarget` (collapses to `<weft>/_lyx` at RelPath ".").
  - `WeftLyxDirFor(slug string) string` → `filepath.Join(l.WeftWorktreePath(slug), l.RelPath, "_lyx")` — a named slug's weft `_lyx` (the junction target paired spawn seeds for `<slug>`). Parallel to `HostLyxLink(slug)`.
  - `WeftCodeguideDir() string` → `filepath.Join(l.WeftWorktree(), l.RelPath, "_codeguide")` — geometry only (no junction this task).
  - `HostLyxLink(slug string) string` → `filepath.Join(l.WorktreePath(slug), l.RelPath, "_lyx")` — host-side junction link in a named slug's host worktree.
  - `HostLyxLinkHere() string` → `filepath.Join(l.WorktreeRoot, l.RelPath, "_lyx")` — host-side junction link in the current host worktree. Document that this is WorktreeRoot+RelPath-based and intentionally distinct from the cwd-based `LyxDir()`.
- **Commit:** `feat(paths): add weft + host-link geometry methods to Layout`

### Card 2: Unit tests for weft geometry methods

- **Context:**
  - `internal/paths/paths.go`
  - `internal/paths/helpers_test.go`
  - `internal/paths/paths_test.go`
- **Edits:** none
- **Creates:**
  - `internal/paths/weft_test.go`
- **Deletes:** none
- **Requirements:** Table-driven white-box (`package paths`) tests constructing a `Layout` literal directly (no git needed) for the RelPath "." case and a subpath case (e.g. `RelPath: "sub/dir"`). Assert each method returns the expected `-weft` sibling path with correct RelPath mirroring: e.g. with `Hub=/h`, `WorktreeRoot=/h/feat`, `Prime=/h/main`, `RelPath="."` → `WeftRepoRoot()==/h/main-weft`, `WeftWorktree()==/h/feat-weft`, `WeftWorktreePath("x")==/h/x-weft`, `WeftLyxDir()==/h/feat-weft/_lyx`, `WeftLyxDirFor("x")==/h/x-weft/_lyx`, `HostLyxLink("x")==/h/x/_lyx`, `HostLyxLinkHere()==/h/feat/_lyx`; with `RelPath="sub"` assert the `sub` segment appears in `WeftLyxDir`, `WeftLyxDirFor("x")`, `WeftCodeguideDir`, `HostLyxLink`, `HostLyxLinkHere`. Assert the junction-end pairing holds: `HostLyxLink("x")` and `WeftLyxDirFor("x")` are siblings differing only by the `-weft` suffix on the worktree dir. Include an assertion that `HostLyxLinkHere()` differs from `LyxDir()` when `Cwd != WorktreeRoot`. Use `filepath.Join` in expected values so the test is OS-agnostic.
- **Commit:** `test(paths): cover weft + host-link geometry methods`

### Card 3: Update CONSTRAINTS.md geometry method list

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the "For New Code" bullet that enumerates `Layout` methods (`LyxDir()`, `WorktreePath(slug)`, … `PrimeName()`), append the eight new methods: `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`. Do not change any other rule text.
- **Commit:** `docs(constraints): list new weft geometry methods`

### Card 4: Update docs/overview.md geometry method list

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In the "## Path Invariants" section, the line beginning "The `Layout` type provides geometry methods:" enumerates the methods — append the same eight new method names (`WeftRepoRoot`, `WeftWorktreePath`, `WeftWorktree`, `WeftLyxDir`, `WeftLyxDirFor`, `WeftCodeguideDir`, `HostLyxLink`, `HostLyxLinkHere`). Do NOT edit the "## Weft overlay model" section or the module list here (those belong to batch 2). Keep the edit limited to the geometry method enumeration.
- **Commit:** `docs(overview): list new weft geometry methods`

## Batch Tests

`verify: go test ./internal/paths/` runs the whole `internal/paths` package, covering the new `internal/paths/weft_test.go` plus the existing `paths_test.go`, `helpers_test.go`, `enforcement_test.go`, and `worktreelist_test.go`. The enforcement test must still pass — the new methods introduce no banned `os.Getwd`/`git rev-parse --show-toplevel` tokens (pure `filepath.Join`). Cards 3 and 4 are doc-only and have no runnable surface; they are covered by the batch landing, not by a test.
