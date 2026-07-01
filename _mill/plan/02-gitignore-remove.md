# Batch: gitignore-remove

```yaml
task: "Add lyx init --undo / deinit command"
batch: "gitignore-remove"
number: 2
cards: 2
verify: go test ./internal/gitignore/... -count=1
depends-on: []
```

## Batch Scope

`internal/gitignore/gitignore.go`'s `Ensure` maintains a shared managed block in
`.gitignore` delimited by `# === lyx-managed ===` / `# === end lyx-managed ===`,
treating entries as a set (multiple modules — currently `lyx init` via `".lyx/"` and
`internal/vscode/config.go`'s `WriteConfig` via `".vscode/"` — contribute to the same
block). This batch adds the mirror-image `Remove` function so `lyx init --undo`
(batch `initcli-undo`) can revert only the `.lyx/` entry it added, without disturbing
other modules' entries or the block itself if others remain. This batch is fully
independent of the other batches — it touches only the `gitignore` package.

The external interface the next dependent batch (`initcli-undo`) consumes is:
`gitignore.Remove(repoRoot string, entries ...string) (changed bool, err error)`.

No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 4: Add `gitignore.Remove`

- **Context:** none
- **Edits:**
  - `internal/gitignore/gitignore.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add `func Remove(repoRoot string, entries ...string) (changed bool, err error)`,
    structured as `Ensure`'s mirror image: parse `<repoRoot>/.gitignore` into the same
    before-block / block-interior / after-block shape `Ensure` already uses (reuse the
    `startMarker`/`endMarker` constants).
  - If the file does not exist, or the managed block does not exist, or none of the
    given `entries` are present in the block: return `(false, nil)` (no file write,
    matching `Ensure`'s idempotency pattern).
  - Otherwise, remove the given `entries` from the tracked set of block entries (the
    remaining entries stay sorted deterministically, same as `Ensure`). If the
    resulting set is non-empty: rewrite the file with the block containing only the
    remaining entries (before-block and after-block content preserved verbatim, same
    formatting rules `Ensure` uses for the blank line before the block). If the
    resulting set is empty: rewrite the file with the entire managed block (both
    `startMarker` and `endMarker` lines, and the blank line `Ensure` inserts before the
    block if any) removed entirely — restoring the file to what it would look like had
    `Ensure` never been called with those entries, not merely leaving an empty block
    shell.
  - Return `(true, nil)` whenever the file was actually rewritten; `(false, nil)`
    otherwise (idempotent no-op, matching `Ensure`'s `changed` semantics).
  - Preserve `Ensure`'s existing helpers (`getOldSortedEntries`, `entriesEqual`) if
    reusable as-is; do not duplicate parsing logic that can be shared.
- **Commit:** `feat(gitignore): add Remove to reverse Ensure's managed block`

### Card 5: Test `gitignore.Remove`

- **Context:**
  - `internal/gitignore/gitignore.go`
- **Edits:**
  - `internal/gitignore/gitignore_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add a test asserting: managed block with only `.lyx/` present →
    `Remove(dir, ".lyx/")` returns `(true, nil)` and the entire block (both markers) is
    gone from the resulting `.gitignore` content, with any surrounding content
    preserved.
  - Add a test asserting: managed block with `.lyx/` and `.vscode/` both present (seed
    via `gitignore.Ensure(dir, ".lyx/", ".vscode/")` first) → `Remove(dir, ".lyx/")`
    returns `(true, nil)`, the block survives with `.vscode/` still present, and
    `.lyx/` is gone.
  - Add a test asserting: `.gitignore` exists with a managed block, but the entry being
    removed was never in it → `Remove(dir, ".lyx/")` returns `(false, nil)` and the
    file content is byte-for-byte unchanged.
  - Add a test asserting: no `.gitignore` file exists at all → `Remove(dir, ".lyx/")`
    returns `(false, nil)` and no file is created.
- **Commit:** `test(gitignore): cover Remove`

## Batch Tests

`verify` runs `go test ./internal/gitignore/... -count=1` — this package's test file
carries no `//go:build integration` tag (pure filesystem operations, no git
subprocesses), so no `-tags integration` flag is needed.
