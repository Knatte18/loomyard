# Batch: gitignore-lib

```yaml
task: 'Extend worktree module: portals and launchers'
batch: 'gitignore-lib'
number: 2
cards: 1
verify: go test ./internal/gitignore/...
depends-on: []
```

## Batch Scope

This batch adds `internal/gitignore`, the shared "portal into `.gitignore`" lib
that manages a single mhgo-managed block as a **set** of entries, so multiple
modules (`board init` → `.mhgo/`, `ide` → `.vscode/`) can contribute without
clobbering each other. It is purely additive — `internal/board/init.go` is NOT
touched here (its refactor to call this lib happens in batch 4, keeping this
batch a clean root with no dependencies). **External interface consumed by
batches 4 and 6:** `gitignore.Ensure(repoRoot string, entries ...string)
(changed bool, err error)`.

## Cards

### Card 4: internal/gitignore managed-block set API

- **Context:**
  - `internal/board/init.go`
  - `internal/board/init_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitignore/gitignore.go`
  - `internal/gitignore/gitignore_test.go`
- **Deletes:** none
- **Requirements:** In a new `package gitignore`, add `Ensure(repoRoot string,
  entries ...string) (changed bool, err error)` that maintains the block
  delimited by `# === mhgo-managed ===` and `# === end mhgo-managed ===` inside
  `<repoRoot>/.gitignore`. Port the parse/merge logic from
  `internal/board/init.go`'s `updateGitignoreBlock` (capture before-block /
  interior / after-block; preserve content outside the block; create the file if
  absent; ensure a trailing newline; insert a blank line before the block when
  preceding content exists), but generalize the interior from a single hardcoded
  `.mhgo/` line to a **set** of `entries`: union the requested entries with any
  already present in the block, sort deterministically, dedupe, and write.
  Return `changed=true` when the file is created or the block interior changes,
  `false` when unchanged (idempotent). `gitignore_test.go` (table-driven):
  new-file creation writes both delimiters and the entry; adding an entry to an
  existing block merges (both old and new entries present, sorted); idempotent
  re-add of the same entry returns `changed=false` and identical bytes; a
  two-module set merge (`Ensure(root, ".mhgo/")` then `Ensure(root, ".vscode/")`)
  leaves both entries coexisting in one block; content outside the block is
  preserved verbatim; the delimiter markers are exact. These mirror the
  `.gitignore` cases currently asserted indirectly in
  `internal/board/init_test.go`.
- **Commit:** `feat(gitignore): add shared managed-block set API`

## Batch Tests

`verify: go test ./internal/gitignore/...` runs `gitignore_test.go`, covering
new-file creation, set-merge across modules, idempotency, outside-block
preservation, and delimiter correctness. Pure logic, single package — no
cross-cutting scope.
