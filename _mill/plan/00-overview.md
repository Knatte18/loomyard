# Plan: weft producers: _lyx/config, lyx config, codeguide

```yaml
task: 'weft producers: _lyx/config, lyx config, codeguide'
slug: weft-producers
approved: false
started: 20260623-110523
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches._

```yaml
batches:
  - number: 1
    name: module-config-templates
    file: 01-module-config-templates.md
    depends-on: []
    verify: go test ./internal/board/ ./internal/worktree/ ./internal/weft/
  - number: 2
    name: paths-host-junctions
    file: 02-paths-host-junctions.md
    depends-on: []
    verify: go test -tags integration ./internal/paths/ ./internal/worktree/
  - number: 3
    name: config-edit-machinery
    file: 03-config-edit-machinery.md
    depends-on: []
    verify: go test ./internal/config/
  - number: 4
    name: lyx-config-command
    file: 04-lyx-config-command.md
    depends-on: [1, 2, 3]
    verify: go test -tags integration ./internal/configcli/ ./cmd/lyx/
```

## Shared Decisions

### Decision: module-owned config templates

- **Decision:** Each config module owns a `ConfigTemplate() string` function returning its
  fully-commented default YAML. `board` and `worktree` templates are **relocated** from
  `internal/board/init.go`; `weft`'s is **authored fresh** (no prior generator existed).
- **Rationale:** Single source of truth per module; both `lyx init` and the new `lyx config`
  command consume the same generator. Matches the operator's "modules own their configs" steer.
- **Applies to:** module-config-templates, lyx-config-command

### Decision: config-edit machinery is weft-agnostic; composition lives above

- **Decision:** The load/scaffold/edit/validate machinery lives in `internal/config`
  (`config.Edit`). It must NOT import `internal/weft`. The edit→`weft sync` composition and the
  command surface live in a new `internal/configcli` package that may import both.
- **Rationale:** `internal/weft` already imports `internal/config`; a `weft` call from inside
  `internal/config` would be a circular import. Verified: no import cycle between
  `board`/`worktree`/`weft`/`config` and the new `configcli`.
- **Applies to:** config-edit-machinery, lyx-config-command

### Decision: zero codeguide code

- **Decision:** This task ships NO `_codeguide` junction seeding, NO `_codeguide` pathspec
  entry, NO `HostCodeguideLink`, NO `codeguide.yaml` schema, and NO `codeguide` registry entry.
  The general mechanisms (paths junction list, module registry) are built extensible so a future
  codeguide module is a one-entry drop-in.
- **Rationale:** Codeguide does not exist yet and is a separate later task. Adding any codeguide
  geometry would break the `internal/paths/codeguide_guard_test.go` guard and exceed scope.
- **Applies to:** all batches

### Decision: Go test conventions

- **Decision:** Integration tests that need real git fixtures use `//go:build integration` and
  the `internal/lyxtest` paired host+weft fixtures (`CopyPaired`/`CopyWeft`). Pure-logic tests
  are untagged. `verify:` commands use the native `go test` runner (no `PYTHONPATH=` prefix —
  this is a Go project). Editors are injected in tests via a fake `EditorFunc`.
- **Rationale:** Matches existing repo convention (`internal/worktree/weft_test.go`,
  `internal/board/boardtest/*`); the broken `LyxTestHub` checkout is NOT a test substrate.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/main.go`
- `cmd/lyx/main_test.go`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/board/init.go`
- `internal/board/init_test.go`
- `internal/board/template.go`
- `internal/board/template_test.go`
- `internal/config/edit.go`
- `internal/config/edit_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_test.go`
- `internal/configcli/menu.go`
- `internal/paths/paths.go`
- `internal/paths/weft_test.go`
- `internal/weft/template.go`
- `internal/weft/template_test.go`
- `internal/worktree/template.go`
- `internal/worktree/template_test.go`
- `internal/worktree/weft.go`
- `internal/worktree/weft_test.go`
