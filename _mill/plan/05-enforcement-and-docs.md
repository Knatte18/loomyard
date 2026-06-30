# Batch: enforcement-and-docs

```yaml
task: 'Harden the Path Invariant: close enforcement hole + fix geometry leaks'
batch: enforcement-and-docs
number: 5
cards: 4
verify: go build ./... && go test ./...
depends-on: [2, 3, 4]
```

## Batch Scope

The integration batch. Tightens `internal/paths/enforcement_test.go` with an AST scan that bans
geometry-literal *construction* outside `internal/paths`, then updates the docs that the change
makes inaccurate (`CONSTRAINTS.md`, `docs/shared-libs/paths.md`, and the two `${env:}` examples
that referenced the now-dead `LYX_BOARD_PATH`). It depends on batches 2, 3, and 4 because the new
ban fails the build the instant any unconverted geometry site remains — every warp/board/lyxtest
conversion must already be merged. Its repo-wide verify is the cross-package backstop and the
de-facto done gate.

## Cards

### Card 18: Add AST geometry-literal scan to enforcement_test.go

- **Context:**
  - `cmd/lyx/registration_test.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/paths/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Keep the existing `TestEnforcement` substring guard (the `os.Getwd` /
  `--show-toplevel` tree-scan and its predicate sub-test; allowlist `internal/paths` +
  `cmd/lyx/main.go`; skips `*_test.go`) unchanged. Add a new test (e.g.
  `TestEnforcement_GeometryLiterals`) that walks the repo tree like `registration_test.go`
  (`runtime.Caller(0)` → repoRoot, `filepath.WalkDir`,
  `parser.ParseFile(..., parser.SkipObjectResolution)`, `ast.Inspect`). For every non-`*_test.go`
  `.go` file outside `internal/paths` (allowlist = `internal/paths` only), flag a string literal
  whose unquoted value **equals** (whole-token, not substring) one of the tokens `_board`,
  `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`, `_lyx` when it appears in a
  path-construction context: (a) an argument to a `filepath.Join(...)` call, (b) an operand of a
  binary `+` (`token.ADD`) expression, or (c) the value of a string `const` declaration. Collect
  failures as `relPath` and fail with the list. Add a `discovered`/`scanned-non-empty`-style
  sanity sub-test so a misconfigured walk cannot pass vacuously. Add a predicate/AST-fixture
  sub-test that parses synthetic snippets with `go/parser`: positives that MUST flag
  (`filepath.Join(x, "_board")`, `slug + "-weft"`, `const s = "-HUB"`); negatives that MUST pass
  (a doc comment containing `-weft`, a `Long: "...-weft..."` struct-field literal,
  a plain non-token string, and the compound near-tokens `slug + "-weft-bare"` and
  `filepath.Join(x, "_boardroom")` to pin whole-token matching). The real tree-scan must pass on
  the fully-converted tree (batches 2–4).
- **Commit:** `test(paths): ban geometry-literal construction via AST enforcement scan`

### Card 19: Update CONSTRAINTS.md Path Invariant

- **Context:**
  - `internal/paths/paths.go`
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the Path Invariant section: (1) record that geometry-dir literals
  (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`, `_lyx`) outside
  `internal/paths` are now machine-enforced in path-construction context (`filepath.Join` arg,
  `+` operand, string-const decl) by the extended `enforcement_test.go`, via whole-token matching,
  production-only (`*_test.go` excluded — test geometry stays a review rule); (2) add the new API
  to the helper inventory: consts `WeftSuffix`/`BoardDirName`/`HubSuffix`, pure funcs
  `WeftSiblingPath`/`BoardDir`/`HubPath`, reverse parser `WeftHostSlug`; (3) record the principle
  that geometry is paths-owned and not config/env-overridable (board data dir = `--board-path`
  flag > `paths.BoardDir`), while non-geometry config keeps `${env:NAME:-default}`; (4) note the
  legitimately-allowed bypasses: git pathspecs / parse comparisons (`status.go` `_lyx`/`_codeguide`),
  pure filenames (`home`/`sidebar`/`proposal_prefix`), user-supplied paths (clone URLs/dests).
- **Commit:** `docs(constraints): record machine-enforced geometry-literal ban and new paths API`

### Card 20: Update paths shared-lib doc

- **Context:**
  - `internal/paths/paths.go`
  - `internal/paths/enforcement_test.go`
- **Edits:**
  - `docs/shared-libs/paths.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Document the new exported API (consts `WeftSuffix`/`BoardDirName`/`HubSuffix`;
  pure funcs `WeftSiblingPath(hub, slug)`, `BoardDir(hub)`, `HubPath(parent, name)`; reverse parser
  `WeftHostSlug(name) (slug, ok)`) and the extended enforcement guard (whole-token,
  construction-context, production-only, allowlist `internal/paths`). Keep the doc consistent with
  the actual signatures shipped in batch 1.
- **Commit:** `docs(paths): document geometry vocabulary API and enforcement guard`

### Card 21: Refresh dead LYX_BOARD_PATH examples in env docs

- **Context:**
  - `internal/boardengine/template.yaml`
- **Edits:**
  - `docs/shared-libs/yamlengine.md`
  - `docs/shared-libs/configengine.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Both docs use `path: ${env:LYX_BOARD_PATH:-../_board}/sub` as the canonical
  `${env:}` illustration (`yamlengine.md:34`, `configengine.md:60`). `LYX_BOARD_PATH` no longer
  exists after batch 3, so swap the sample variable for a generic placeholder (e.g.
  `${env:LYX_EXAMPLE_PATH:-../_board}/sub`). The `${env:}` mechanism description is unchanged —
  this is a doc-accuracy fix to the example var name only.
- **Commit:** `docs(env): replace dead LYX_BOARD_PATH example var in env-substitution docs`

## Batch Tests

`verify: go build ./... && go test ./...` is intentionally repo-wide. Justification: the
deliverable of this batch — the tightened `enforcement_test.go` — is itself a tree-wide AST scan,
so it can only be validated against the entire converted source tree, and the ban only becomes
active here (after batches 2–4). The repo-wide run also serves as the cross-package backstop for
the scoped verifies in batches 2–4 (e.g. confirming no consumer of `lyxtest` or the board template
regressed) and as the de-facto done gate (`pipeline.done_gate` is left null since this batch
already runs the full suite). Cards 19–21 are docs-only and have no runnable surface beyond the
build.
