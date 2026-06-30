# Batch: code-rename

```yaml
task: "Rename internal/paths to internal/hubgeometry"
batch: "code-rename"
number: 1
cards: 8
verify: go build ./... && go test ./... && go vet -tags integration ./...
depends-on: []
```

## Rename mechanic

This batch renames the `internal/paths` package directory to `internal/hubgeometry`.
For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
   Moving every file out of `internal/paths/` into `internal/hubgeometry/` effects the
   directory rename (git tracks files, not directories; the empty source dir disappears).
2. Make ONLY surgical edits — touch only the lines that must change after the move: the
   `package` declaration, the package doc comment, and (in the two guard test files) the
   hardcoded allowlist string literals and the `"paths.go"` filename literal. Do NOT
   rewrite any moved file from scratch and do NOT touch geometry logic, the `Layout`
   type's fields/methods, or any exported symbol name.
3. Use a full-file `Creates:` entry only for genuinely new files — there are none here.
4. Never write a relocated file from scratch and delete the original — that breaks git
   rename history and inflates the review diff.

## Batch Scope

This batch performs the entire behaviour-preserving Go-source rename of the `paths`
package to `hubgeometry` and updates **every** importer so the build and full test suite
stay green. It is a single batch by necessity: the rename is atomic — the moment
`package paths` becomes `package hubgeometry` and the directory moves, every importer that
still says `github.com/Knatte18/loomyard/internal/paths` fails to compile, so `go build
./...` only passes once **all** references are updated together. An intermediate
half-renamed state does not build; therefore all code cards live here and `verify` runs
only at batch end. Card 1 moves the package and fixes its two self-referential guards
(the raw-primitive / geometry-literal allowlists in `enforcement_test.go` and the
`"paths.go"` filename skip in `codeguide_guard_test.go`); cards 2–8 retarget the importer
set, grouped by subsystem. No exported API, `Layout` field/method, resolution behaviour,
or symbol name changes — only the package path and the `paths.`/`hubgeometry.` qualifier.
The next batch (docs) consumes nothing from this batch at the code level; it depends on
it only for ordering so the docs describe the already-renamed package.

Batch-local decision: every importer must be retargeted in this one batch because
`go build ./...` compiles only when every reference is updated atomically.

## Cards

### Card 1: Move the package internal/paths → internal/hubgeometry and fix its self-referential guards

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/paths/paths.go` -> `internal/hubgeometry/hubgeometry.go`
  - `internal/paths/paths_test.go` -> `internal/hubgeometry/hubgeometry_test.go`
  - `internal/paths/paths_unit_test.go` -> `internal/hubgeometry/hubgeometry_unit_test.go`
  - `internal/paths/worktreelist.go` -> `internal/hubgeometry/worktreelist.go`
  - `internal/paths/worktreelist_test.go` -> `internal/hubgeometry/worktreelist_test.go`
  - `internal/paths/geometry_test.go` -> `internal/hubgeometry/geometry_test.go`
  - `internal/paths/weft_test.go` -> `internal/hubgeometry/weft_test.go`
  - `internal/paths/enforcement_test.go` -> `internal/hubgeometry/enforcement_test.go`
  - `internal/paths/codeguide_guard_test.go` -> `internal/hubgeometry/codeguide_guard_test.go`
- **Requirements:** After `git mv` of all nine files (which moves the directory):
  - Change the package clause in the white-box files to `package hubgeometry`:
    `hubgeometry.go`, `worktreelist.go`, `enforcement_test.go`, `codeguide_guard_test.go`.
  - Change the package clause in the black-box test files to `package hubgeometry_test`:
    `hubgeometry_test.go`, `hubgeometry_unit_test.go`, `geometry_test.go`, `weft_test.go`,
    `worktreelist_test.go`. (Do not assume one package name — confirm each file's clause.)
  - In `hubgeometry.go`: update the leading package doc comment "Package paths is the
    single owner of Loomyard worktree and container geometry…" to name `hubgeometry`.
  - In `enforcement_test.go`: change both hardcoded allowlist string literals
    `pkgDir == "internal/paths"` (the raw-primitive allowlist, ~line 69) and
    `filepath.ToSlash(filepath.Dir(relPath)) == "internal/paths"` (the geometry-literal
    allowlist, ~line 347) to `"internal/hubgeometry"`, and update every `internal/paths`
    mention in its comments (e.g. "Two levels up from internal/paths/enforcement_test.go",
    "Allowlist: internal/paths is the sole permitted owner") to `internal/hubgeometry`.
  - In `codeguide_guard_test.go`: change the filename skip `if d.Name() == "paths.go"`
    (~line 48) to `if d.Name() == "hubgeometry.go"` (this skip is **filename-based**, not
    package-name-based — `hubgeometry.go` legitimately contains the `_codeguide` literal
    via `WeftCodeguideDir`, so a missed update makes `TestCodeguideGuard/tree-scan` fail),
    and update the comment at lines 1-5 and 45-47 that name `paths.go`/`internal/paths`.
  - Do NOT rename, add, or remove any exported or unexported identifier, `Layout` field
    or method, constant (`WeftSuffix`, `BoardDirName`, `HubSuffix`, `LyxDirName`), or
    change any resolution logic. Pure rename.
- **Commit:** `refactor(hubgeometry): move internal/paths package to internal/hubgeometry`

### Card 2: Retarget cmd/lyx importers

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_test.go`
  - `cmd/lyx/registration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the import
  `github.com/Knatte18/loomyard/internal/paths` → `.../internal/hubgeometry` and every
  `paths.` package-selector qualifier → `hubgeometry.` (e.g. `paths.Resolve` →
  `hubgeometry.Resolve`). In `registration_test.go`, also update the comment at ~line 66
  that names `internal/paths/enforcement_test.go` → `internal/hubgeometry/enforcement_test.go`.
  Do not touch any local variable also named `paths` (replace only the package selector).
- **Commit:** `refactor(hubgeometry): retarget cmd/lyx to internal/hubgeometry`

### Card 3: Retarget board importers

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/boardcli/cli.go`
  - `internal/boardcli/cli_test.go`
  - `internal/boardengine/boardtest/bench_test.go`
  - `internal/boardengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the import
  `github.com/Knatte18/loomyard/internal/paths` → `.../internal/hubgeometry` and every
  `paths.` package-selector qualifier → `hubgeometry.`. Update any code comment naming
  `internal/paths` to `internal/hubgeometry`. Pure mechanical retarget; no logic change.
- **Commit:** `refactor(hubgeometry): retarget board to internal/hubgeometry`

### Card 4: Retarget config family importers

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/menu.go`
  - `internal/configcli/reconcile_test.go`
  - `internal/configengine/config.go`
  - `internal/configengine/config_test.go`
  - `internal/configengine/edit.go`
  - `internal/configengine/edit_test.go`
  - `internal/configsync/configsync.go`
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the import
  `github.com/Knatte18/loomyard/internal/paths` → `.../internal/hubgeometry` and every
  `paths.` package-selector qualifier → `hubgeometry.`. Update any code comment naming
  `internal/paths` to `internal/hubgeometry`. Pure mechanical retarget; no logic change.
- **Commit:** `refactor(hubgeometry): retarget config family to internal/hubgeometry`

### Card 5: Retarget ide importers

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/idecli/cli.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/menu_test.go`
  - `internal/ideengine/spawn.go`
  - `internal/ideengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the import
  `github.com/Knatte18/loomyard/internal/paths` → `.../internal/hubgeometry` and every
  `paths.` package-selector qualifier → `hubgeometry.`. Update any code comment naming
  `internal/paths` to `internal/hubgeometry`. Pure mechanical retarget; no logic change.
- **Commit:** `refactor(hubgeometry): retarget ide to internal/hubgeometry`

### Card 6: Retarget warp family importers

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/warpcli/clone.go`
  - `internal/warpcli/warp.go`
  - `internal/warpcli/warp_test.go`
  - `internal/warpengine/add.go`
  - `internal/warpengine/checkout.go`
  - `internal/warpengine/cleanup.go`
  - `internal/warpengine/clone.go`
  - `internal/warpengine/clone_integration_test.go`
  - `internal/warpengine/config_test.go`
  - `internal/warpengine/drift.go`
  - `internal/warpengine/drift_test.go`
  - `internal/warpengine/hook.go`
  - `internal/warpengine/hook_test.go`
  - `internal/warpengine/junction.go`
  - `internal/warpengine/launchers.go`
  - `internal/warpengine/launchers_test.go`
  - `internal/warpengine/list.go`
  - `internal/warpengine/portals.go`
  - `internal/warpengine/portals_test.go`
  - `internal/warpengine/prune.go`
  - `internal/warpengine/reconcile.go`
  - `internal/warpengine/reconcile_test.go`
  - `internal/warpengine/remove.go`
  - `internal/warpengine/remove_test.go`
  - `internal/warpengine/status.go`
  - `internal/warpengine/status_test.go`
  - `internal/warpengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the import
  `github.com/Knatte18/loomyard/internal/paths` → `.../internal/hubgeometry` and every
  `paths.` package-selector qualifier → `hubgeometry.`. Update any code comment naming
  `internal/paths` to `internal/hubgeometry`. Pure mechanical retarget; no logic change.
- **Commit:** `refactor(hubgeometry): retarget warp family to internal/hubgeometry`

### Card 7: Retarget weft importers

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/weftcli/cli.go`
  - `internal/weftcli/cli_test.go`
  - `internal/weftengine/config_test.go`
  - `internal/weftengine/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the import
  `github.com/Knatte18/loomyard/internal/paths` → `.../internal/hubgeometry` and every
  `paths.` package-selector qualifier → `hubgeometry.`. Update any code comment naming
  `internal/paths` to `internal/hubgeometry`. Pure mechanical retarget; no logic change.
- **Commit:** `refactor(hubgeometry): retarget weft to internal/hubgeometry`

### Card 8: Retarget leaf consumers (lyxtest, vscode, envsource, initcli, muxpoccli)

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxtest/doc.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/lyxtest_test.go`
  - `internal/vscode/color.go`
  - `internal/vscode/color_test.go`
  - `internal/envsource/envsource.go`
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
  - `internal/muxpoccli/cli.go`

### Card 8a (holistic-r1): Fix stale paths.X prose comments in out-of-manifest files

- **Context:**
  - holistic review finding: stale `paths.X` qualifiers in comments of six files absent from original manifest
- **Edits:**
  - `internal/boardengine/config.go`
  - `internal/boardengine/template_test.go`
  - `internal/warpcli/clone_cli_test.go`
  - `internal/idecli/cli_test.go`
  - `cmd/lyx/unknown_subcommand_test.go`
  - `internal/muxpoccli/muxpoc_smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, change the import
  `github.com/Knatte18/loomyard/internal/paths` → `.../internal/hubgeometry` and every
  `paths.` package-selector qualifier → `hubgeometry.`. In `leaf_enforcement_test.go`,
  also update the doc comment at ~line 19 "imports only stdlib and internal/paths" →
  "…and internal/hubgeometry"; do NOT add `internal/hubgeometry` to the `bannedImports`
  list (it is an allowed import, and the list is a banlist, not an allowlist — `paths`
  was never in it). The lyxtest Leaf Invariant is preserved: lyxtest still imports only
  stdlib and the geometry package. Pure mechanical retarget elsewhere; no logic change.
- **Commit:** `refactor(hubgeometry): retarget leaf consumers to internal/hubgeometry`

## Batch Tests

`verify: go build ./... && go test ./... && go vet -tags integration ./...` — the rename
touches packages across the whole tree, so the verify is intentionally repo-wide (this is
the cross-cutting case the per-batch-scoping rule explicitly allows). `go build ./...`
proves every importer was retargeted (a missed `internal/paths` import fails to compile).
`go test ./...` runs the load-bearing guards. The trailing `go vet -tags integration ./...`
is required because three edited files carry `//go:build integration`
(`internal/configcli/configcli_integration_test.go`,
`internal/warpengine/clone_integration_test.go`,
`internal/weftengine/weft_integration_test.go`) — `go build` skips all `_test.go` and
untagged `go test ./...` excludes them, so without the tagged vet a botched
`paths.`→`hubgeometry.` retarget in those three would ship green. `go vet -tags
integration` type-checks (compiles) them without running the heavy integration suites.

`go test ./...` runs the load-bearing guards that the rename must keep green:

- `internal/hubgeometry/enforcement_test.go` — `TestEnforcement_*` (raw-primitive ban)
  and `TestEnforcement_GeometryLiterals` (geometry-literal ban). Fail loudly if the two
  allowlist string literals were not updated to `"internal/hubgeometry"`. Primary signal.
- `internal/hubgeometry/codeguide_guard_test.go` — `TestCodeguideGuard/tree-scan`; fails
  if the `"paths.go"` filename skip was not updated to `"hubgeometry.go"`.
- `internal/lyxtest/leaf_enforcement_test.go` — confirms lyxtest's import set stays
  {stdlib, internal/hubgeometry}.
- `cmd/lyx/*` guards (`drift_test.go`, `helptree_test.go`, `registration_test.go`,
  `longlist_test.go`) — expected to stay green unchanged.

The implementer should run `gofmt`/`goimports` on touched files so import blocks regroup
after the path change.
