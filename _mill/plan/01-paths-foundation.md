# Batch: paths-foundation

```yaml
task: 'Harden the Path Invariant: close enforcement hole + fix geometry leaks'
batch: paths-foundation
number: 1
cards: 5
verify: go test ./internal/paths/...
depends-on: []
```

## Batch Scope

Establishes `internal/paths` as the sole owner of the geometry vocabulary. Adds the exported
constants, the pure bootstrap functions, and the reverse parser, then refactors the three
existing weft `Layout` methods to delegate to the new helpers so no inline `"-weft"` literal
remains in `paths.go`. This batch ships the external interface (`WeftSuffix`, `BoardDirName`,
`HubSuffix`, `WeftSiblingPath`, `BoardDir`, `HubPath`, `WeftHostSlug`) that batches 2–4 consume.
No other package is touched. All edits stay inside `internal/paths`, which is the enforcement
allowlist, so the existing `os.Getwd`/`--show-toplevel` guard is unaffected and the new AST ban
does not exist yet.

## Cards

### Card 1: Add exported geometry constants

- **Context:**
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the existing `const (...)` block in `paths.go` (where `LyxDirName = "_lyx"`
  lives), add three exported constants: `WeftSuffix = "-weft"`, `BoardDirName = "_board"`,
  `HubSuffix = "-HUB"`. Add a godoc comment on each (the suffix appended to a slug to form the
  weft sibling dir; the hub board data-dir name; the suffix appended to a repo name to form the
  hub dir). These are the single source of these literals for the whole repo.
- **Commit:** `feat(paths): add exported WeftSuffix/BoardDirName/HubSuffix geometry constants`

### Card 2: Add pure geometry-construction functions

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three package-level pure functions (no `Layout` receiver) for bootstrap
  callers that have no resolved `Layout`:
  `WeftSiblingPath(hub, slug string) string` returns `filepath.Join(hub, slug+WeftSuffix)`;
  `BoardDir(hub string) string` returns `filepath.Join(hub, BoardDirName)`;
  `HubPath(parent, name string) string` returns `filepath.Join(parent, name+HubSuffix)`.
  Godoc each. These are the canonical builders for `<hub>/<slug>-weft`, `<hub>/_board`, and
  `<parent>/<name>-HUB` respectively.
- **Commit:** `feat(paths): add WeftSiblingPath/BoardDir/HubPath pure constructors`

### Card 3: Add WeftHostSlug reverse parser

- **Context:**
  - `internal/paths/paths.go`
  - `internal/warpengine/prune.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `WeftHostSlug(name string) (slug string, ok bool)`. It returns
  `(strings.TrimSuffix(name, WeftSuffix), true)` when `strings.HasSuffix(name, WeftSuffix)` is
  true AND the stripped result is non-empty; otherwise `("", false)`. The non-empty guard
  reproduces `prune.go`'s current `len(name) <= len("-weft")` skip (a bare `"-weft"` name yields
  `ok == false`). `strings` is already imported in `paths.go`. This owns the weft-suffix matching
  used by prune pass-2 (batch 2).
- **Commit:** `feat(paths): add WeftHostSlug reverse parser for weft sibling names`

### Card 4: Delegate the three weft Layout methods to the new helpers

- **Context:**
  - `internal/paths/paths_test.go`
  - `internal/paths/weft_test.go`
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Refactor the three weft `Layout` methods to delegate, removing every inline
  `"-weft"` literal from these methods: `WeftRepoRoot()` returns
  `WeftSiblingPath(l.Hub, l.PrimeName())`; `WeftWorktreePath(slug)` returns
  `WeftSiblingPath(l.Hub, slug)`; `WeftWorktree()` returns
  `WeftSiblingPath(l.Hub, filepath.Base(l.WorktreeRoot))`. Keep return values byte-identical
  (these are the same joins). Leave `"_codeguide"` / `"_lyx"` literals elsewhere in `paths.go`
  as-is (they are inside the allowlisted package).
- **Commit:** `refactor(paths): delegate weft Layout methods to WeftSiblingPath`

### Card 5: Unit tests for the new geometry helpers

- **Context:**
  - `internal/paths/paths.go`
  - `internal/paths/paths_unit_test.go`
  - `internal/paths/weft_test.go`
- **Edits:** none
- **Creates:**
  - `internal/paths/geometry_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add table-driven tests in package `paths` (or `paths_test`, matching the
  existing unit-test package style in `paths_unit_test.go`) covering: `WeftSiblingPath`,
  `BoardDir`, `HubPath` produce the expected joins for a sample hub/parent; `WeftHostSlug`
  returns `(slug, true)` for `"feat-weft"` → `"feat"`, `(\"\", false)` for a non-`-weft` name
  (`"feat"`), and `(\"\", false)` for the bare suffix `"-weft"` (empty-slug guard). Assert the
  refactored `WeftWorktreePath`/`WeftRepoRoot`/`WeftWorktree` still equal the direct
  `WeftSiblingPath` form (parity). Use `filepath.Join` in the wants so the test is
  separator-agnostic.
- **Commit:** `test(paths): cover WeftSiblingPath/BoardDir/HubPath/WeftHostSlug`

## Batch Tests

`verify: go test ./internal/paths/...` runs the full `internal/paths` package, covering the new
`geometry_test.go` plus the existing `paths_test.go`, `weft_test.go`, `paths_unit_test.go`,
`enforcement_test.go`, and `worktreelist_test.go`. The existing weft/path tests are the parity
gate for Card 4; `geometry_test.go` covers Cards 1–3. Scope is the single package this batch
edits.
