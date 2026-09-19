# Batch: board-type-field

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: board-type-field
number: 2
cards: 2
verify: go test ./internal/boardengine/...
depends-on: []
```

## Batch Scope

This batch adds the `type` field to the Board task record so a task can carry the recipe its child worktree will run, and makes it actually settable.
It is one small batch of its own because it touches only `internal/boardengine`, depends on nothing, and is consumed by batch 6's `Seed-Child` read seam — keeping it separate lets it land in parallel with the `shedrun` leaf and the batten rename.

The external interface batch 6 consumes: `boardengine.Task.Type`, readable through the existing `Board.GetTask(idOrSlug) (Task, bool, error)`, with the empty string meaning `loom`.

Batch-local decision beyond the overview's: `boardengine` never validates the value against a recipe name and never imports `internal/shedrun`.
The Board's job is to carry the choice; rejecting a choice that names nothing is the seeder's, per the discussion's `board-type-field-defaults-to-loom-and-is-validated-late` decision.
`BriefTask` deliberately does not gain the field — Board rendering and display of `type` are out of scope for this task.

## Cards

### Card 5: Type on the task record and in the upsert allowlist

- **Context:**
  - `internal/boardengine/template.yaml`
  - `internal/boardengine/render.go`
  - `internal/boardengine/layer.go`
- **Edits:**
  - `internal/boardengine/task.go`
  - `internal/boardengine/store.go`
  - `internal/boardengine/task_test.go`
  - `internal/boardengine/store_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the field `Type string` with tag `json:"type,omitempty"` to `boardengine.Task`, positioned after `Status` and before `ShortName` so the struct's optional fields stay grouped.
  Document on the field that an empty value means the `loom` recipe, that the meaning is resolved at the seeding site rather than here, and that `omitempty` is what keeps every existing `tasks.json` record valid with no migration.
  Add `"type": true` to `upsertAllowedKeys` in `internal/boardengine/store.go`.
  This entry is the write path, not bookkeeping: `validateUpsertFields` enforces the set as closed for both `UpsertTask` and `UpsertTasksBatch`, so without it the field could never be set by anything and `Seed-Child` would read the empty string forever.
  `NewTask` needs no change — it round-trips through JSON, so the new field flows automatically once the allowlist accepts the key.
  Confirm while editing that neither `render.go`, `layer.go` nor `template.yaml` enumerates `Task`'s fields in a way the addition breaks; do not add the field to `BriefTask`.
  In `store_test.go` add the load-bearing case: an `UpsertTask` carrying `"type"` **succeeds** and the stored task reads it back — that is the assertion proving the allowlist entry landed, and every other test here passes without it.
  In `task_test.go` cover the round-trip through `Task`, an absent value reading back as the empty string, and `omitempty` keeping a record with no `type` byte-identical to today's.
- **Commit:** `feat(boardengine): add the task type field and accept it on upsert`

### Card 6: carry type through merge and promote

- **Context:**
  - `internal/boardengine/task.go`
  - `internal/boardengine/store.go`
- **Edits:**
  - `internal/boardengine/board.go`
  - `internal/boardengine/board_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `taskToUpsertFields` in `internal/boardengine/board.go` to emit `fields["type"] = t.Type` when `t.Type` is non-empty, following the conditional shape the neighbouring `Status` and `ShortName` entries already use rather than emitting it unconditionally.
  `taskToUpsertFields` is the re-upsert path `PromoteNote` runs a record through, so without this a promoted note silently loses its `type`.
  In `board_test.go` assert that promoting a note carrying a `type` preserves it on the resulting task, and that a note with no `type` still round-trips with the key absent rather than present-and-empty.
- **Commit:** `fix(boardengine): carry the task type field through promote-note`

## Batch Tests

`verify: go test ./internal/boardengine/...` runs the whole `boardengine` package, which is the right scope here rather than a narrower one: the change touches the shared `Task` struct and the closed `upsertAllowedKeys` set, both of which every existing test in that package exercises, so a regression in an untouched path (`UpsertTasksBatch`'s own validation, `MergeTasks`' inner upsert, `Load`'s decode of an existing `tasks.json`) is exactly what the full-package run catches.
The package is Tier 1 — it reads and writes JSON under a temp dir and spawns nothing — and already carries its own `testmain_test.go`.

New coverage lands in three files: `store_test.go` (the `type`-carrying upsert succeeding, which proves the allowlist entry), `task_test.go` (round-trip, empty default, `omitempty` byte-identity) and `board_test.go` (promote-note preserving `type`).
The empty-means-`loom` rule itself is deliberately **not** tested here — it is resolved at the seeding site and is covered in batch 6's `Seed-Child` tests.
