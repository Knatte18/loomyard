# Plan: Board fixes from sandbox run — payload keys, help, rerender

```yaml
task: "Board fixes from sandbox run — payload keys, help, rerender"
slug: "board-sandbox-fixes"
approved: false
started: "20260628-150626"
parent: "main"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: payload-contract
    file: 01-payload-contract.md
    depends-on: []
    verify: go test ./internal/board/ ./cmd/lyx/
  - number: 2
    name: cli-help
    file: 02-cli-help.md
    depends-on: [1]
    verify: go test ./internal/board/
  - number: 3
    name: rerender-manifest
    file: 03-rerender-manifest.md
    depends-on: []
    verify: go test ./internal/board/
```

## Shared Decisions

### Decision: one name — `status`, no back-compat

- **Decision:** The task lifecycle field has exactly one operator-facing name: `status`.
  The command is `set-status` (renamed from `set-phase`); the payload key is `status`
  (renamed from `phase`). The on-disk `Task` JSON field is already `status` and does NOT
  change. The old `set-phase` command, the `phase` key, and the `id_or_slug` key are
  removed outright — no hidden or deprecated aliases.
- **Rationale:** Board values (`active`/`done`/`pr-pending`/`ready-to-merge`/`abandoned`)
  are lifecycle states; `status` already appears on disk, in render, and in the wiki task
  record. lyx is pre-release with no internal Go callers of these payloads (verified:
  only tests reference `id_or_slug`/`set-phase`), so a clean break is safe.
- **Applies to:** all batches.

### Decision: no silent drops — strict keys on every payload

- **Decision:** Every board write and lookup payload rejects unknown keys with a clear
  error naming the offending key. Upsert-field validation lives at the store chokepoint
  (`UpsertTask`/`UpsertTasksBatch`/`MergeTasks`, before the `NewTask`/`ApplyPatch` JSON
  round-trip); CLI-shape validation (single-target, `set-deps`, `merge` top-level + inner
  `set_status`, `upsert-batch` wrapper) lives at the cli.go RunE boundary. Decode into a
  `map[string]any` first to detect unknown keys and key presence (Go's typed-struct
  decode silently ignores unknowns and cannot distinguish absent from zero-value).
- **Rationale:** W11 "no silent drop" generalizes; the most dangerous case is a stale
  `phase`/`set_phase` on the renamed commands re-arming the silent no-op the task kills.
- **Applies to:** batch 1 (logic), batch 2 (help documents the resulting schema).

### Decision: lookups accept `slug` or `id`; mutations error on missing; `get` stays a query

- **Decision:** `get`/`set-status`/`remove` accept exactly one of `slug` (non-empty
  string) or `id` (integer). `id=0` is valid (`store.nextID()` starts at 0), so presence
  is detected via map-key membership, never the int zero-value. The resolved value (a Go
  `string` for slug or numeric for id) is passed to the existing store type-switch
  resolvers, which already match on `string`/`int`/`float64`. `set-status`/`remove`/
  `merge`'s status step error with "task not found" when the target is absent; `get`
  remains a query and returns `task:null` for a valid-but-absent target (malformed
  payloads still error).
- **Rationale:** Closes B1/B2 root cause + the masking silent no-op without dropping the
  numeric-lookup ergonomic the operator wants.
- **Applies to:** batch 1.

### Decision: Go-native test commands, no PYTHONPATH prefix

- **Decision:** This is a Go module. `verify:` commands use `go test ./<pkg>/...`
  directly — no `PYTHONPATH=` prefix (that rule is Python-only).
- **Rationale:** `verify-not-isolated` validator is conditional on project language.
- **Applies to:** all batches.

## All Files Touched

- `cmd/lyx/helptree_test.go`
- `internal/board/board.go`
- `internal/board/board_test.go`
- `internal/board/cli.go`
- `internal/board/cli_test.go`
- `internal/board/help_test.go`
- `internal/board/render.go`
- `internal/board/render_test.go`
- `internal/board/store.go`
- `internal/board/store_test.go`
- `internal/board/sync.go`
- `internal/board/task.go`
- `internal/board/task_test.go`
