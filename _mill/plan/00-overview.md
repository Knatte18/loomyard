# Plan: Rename internal/config to internal/configengine

```yaml
task: "Rename internal/config to internal/configengine"
slug: "config-engine-rename"
approved: false
started: "20260628-192924"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: rename
    file: 01-rename.md
    depends-on: []
    verify: go build ./... && go vet -tags integration ./... && go test ./...
```

## Shared Decisions

### Decision: single atomic batch

- **Decision:** The entire rename — package directory move, package clauses, every
  importer, every comment reference, all docs, and the `CONSTRAINTS.md` convention — lands
  in **one batch** (`rename`). Cards within it are logical groupings, but the tree is only
  guaranteed to compile once *all* cards are applied.
- **Rationale:** Renaming a Go package is not separable: the moment `package config`
  becomes `package configengine` and the import path changes, every importer must be
  updated in the same compile unit or `go build ./...` fails. A behaviour-preserving rename
  must keep `go test ./...` green, so partial states are invalid. Splitting into multiple
  batches would produce an intermediate batch whose `verify` cannot pass.
- **Applies to:** all batches.

### Decision: git mv preserves history

- **Decision:** Use `git mv` for the directory rename (`internal/config` →
  `internal/configengine`) and the doc rename (`docs/shared-libs/config.md` →
  `docs/shared-libs/configengine.md`). In the plan these are modelled as `Deletes:` (old
  path) + `Creates:` (new path) for validator purposes, but the implementer MUST achieve
  them via `git mv`, never delete-and-recreate.
- **Rationale:** Keeps `git blame` / history attached to the moved files; the rename is
  pure so history continuity is free. The Deletes+Creates modelling is only how the plan
  validator represents a move (old path exists at plan time → Deletes; new path is a
  Creates target → existence check suppressed).
- **Applies to:** all batches.

### Decision: behaviour-preserving — exported API frozen

- **Decision:** No exported symbol, signature, or behaviour of the engine changes. Only
  the package clause / qualifier moves (`config.Load` → `configengine.Load`). The exported
  surface stays exactly: `Load`, `Edit`, `FindBaseDir`, `EditorFunc`, `ErrAborted`,
  `DefaultEditor`. There is no CLI change — the engine has no `Command()`; `lyx config`
  is owned by `internal/configcli` and is untouched.
- **Rationale:** This is a mechanical rename and the precursor to the separate
  `internal/module/` restructure. Green existing tests are the proof of preservation.
- **Applies to:** all batches.

### Decision: completeness via word-boundary grep

- **Decision:** After the edits, completeness is verified by a **word-boundary** grep —
  `grep -rn "internal/config\b"` over `*.go` and `docs/`, plus a check for the bare
  `config` package token — excluding the `config{cli,reg,sync,engine}` tokens. It must
  return nothing referring to the renamed engine.
- **Rationale:** Comment forms like `internal/config;` and `internal/config)` are not
  caught by a quoted-import (`internal/config"`) or bare-qualifier (`config.`) grep, so a
  word-boundary pattern is required to prove "zero stale references."
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `docs/benchmarks/test-suite-timing.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/configengine.md`
- `docs/shared-libs/paths.md`
- `internal/board/config.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_integration_test.go`
- `internal/configcli/configcli_test.go`
- `internal/configcli/menu.go`
- `internal/configengine/config.go`
- `internal/configengine/config_test.go`
- `internal/configengine/edit.go`
- `internal/configengine/edit_test.go`
- `internal/paths/paths.go`
- `internal/warp/config.go`
- `internal/warp/worktreelifecycle.go`
- `internal/weft/config.go`
