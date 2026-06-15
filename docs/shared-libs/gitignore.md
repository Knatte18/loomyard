# `internal/gitignore`

A **shared `.gitignore` block manager** that multiple modules contribute ignore entries to.

Many modules create machine-local directories that should not be committed (e.g.,
`.vscode/` from `ide`, `.mhgo/` from `board`). Instead of each module managing its
own `.gitignore` block or clobbering others' entries, `internal/gitignore` maintains
a single mhgo-managed block (`# === mhgo-managed === … # === end mhgo-managed ===`)
as a **set** that merges entries idempotently.

## Exported API

### `Ensure(repoRoot string, entries ...string) (changed bool, err error)`

Ensures the mhgo-managed block in `.gitignore` contains all given entries, adding
any that are missing.

**Behavior:**

1. Reads `.gitignore` at the repo root (or creates it if absent).
2. Locates or creates the mhgo-managed block delimited by:
   ```
   # === mhgo-managed ===
   <entries>
   # === end mhgo-managed ===
   ```
3. Merges the given entries into the block (idempotent — entries are never duplicated).
4. Writes the file back atomically (temp + rename) if anything changed.

**Returns:** `true` if the file was written (entries were added or the block was
created), `false` if nothing changed. Any write error is returned as a non-nil error.

**Design note:** Entries are merged as a **set**, so the order within the block may
change when a write occurs. The block is human-readable and preserved between calls.

## Usage pattern

Modules register their ignore entries when they first create their directories:

```go
// board/init.go
changed, err := gitignore.Ensure(repoRoot, ".mhgo/")

// ide/spawn.go (when first creating .vscode/)
changed, err := gitignore.Ensure(repoRoot, ".vscode/")
```

Each module calls `Ensure` with only its own entries; `Ensure` merges them into one
shared block, so all active modules' directories coexist without clobbering.

## Machine-local directories

Directories managed by modules (e.g., `.vscode/`, `.mhgo/`) are **not committed**.
Because `.gitignore` itself is committed at the repo's cwd / `relpath`, every
worktree checkout inherits the ignores — so a new worktree's `.gitignore` already
lists `.vscode/` even before `ide` runs there.
