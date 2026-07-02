# Batch: hubgeometry-dotlyx

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'hubgeometry-dotlyx'
number: 1
cards: 1
verify: go test ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

Adds ownership of the **ephemeral `.lyx`** directory to `internal/hubgeometry`, so mux
resolves `.lyx/mux.json` and `.lyx/mux.lock` through a geometry accessor rather than a
hardcoded literal (Hub Geometry Invariant). This is the single external interface later
batches consume: `(*Layout).DotLyxDir()`. `.lyx` (dot, ephemeral, machine-bound) is deliberately **distinct** from the existing `_lyx`
(underscore, durable/weft-synced).

**Git-ignore is already handled — no card needed (GAP C).** `.lyx/` is already git-ignored:
the root `.gitignore` carries `.lyx/`, and `lyx init` maintains that managed block via
`gitignore.Ensure(cwd, ".lyx/")` (`internal/initengine/init.go:101`; `lyx init --undo` reverts
it). Since `lyx init` is the precondition for every lyx module (mux's `LoadConfig` errors with
`run "lyx init"` when uninitialized), mux's `.lyx/mux.json` + `.lyx/mux.lock` are covered with
zero new wiring. Do **not** add a git-ignore card. Batch-local decision: the accessor is added **without** registering
`.lyx` as a machine-enforced geometry token (YAGNI — the accessor is the single source of
the path, and no other package constructs `.lyx` paths; if a future reviewer wants
enforcement, adding `".lyx"` to `enforcement_test.go`'s `geometryToken` switch is the
localized follow-up).

## Cards

### Card 1: Add `.lyx` (ephemeral dir) accessor to hubgeometry

- **Context:**
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_unit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/hubgeometry/hubgeometry.go`, add an unexported package
  constant `dotLyxDirName = ".lyx"` alongside the existing `LyxDirName = "_lyx"` const, and
  add a `*Layout` method `func (l *Layout) DotLyxDir() string { return filepath.Join(l.Cwd,
  dotLyxDirName) }`, mirroring the existing `func (l *Layout) LyxDir() string` accessor. The
  method must return `<Cwd>/.lyx`. Do **not** modify `LyxDir` or `LyxDirName`. In
  `internal/hubgeometry/hubgeometry_unit_test.go`, add a unit test asserting `DotLyxDir()`
  returns `filepath.Join(l.Cwd, ".lyx")` for a `Layout` with a known `Cwd`, and that it is
  distinct from `LyxDir()` (`_lyx` vs `.lyx`). Do not touch `enforcement_test.go`.
- **Commit:** `feat(hubgeometry): add DotLyxDir accessor for ephemeral .lyx`

## Batch Tests

`verify: go test ./internal/hubgeometry/...` runs the whole hubgeometry package, including
the existing `enforcement_test.go` / `geometry_test.go` guards (confirming the new accessor
does not trip the geometry-literal enforcement) plus the new `DotLyxDir` unit test.
