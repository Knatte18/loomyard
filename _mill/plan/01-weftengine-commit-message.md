# Batch: weftengine-commit-message

```yaml
task: "Add lyx init --undo / deinit command"
batch: "weftengine-commit-message"
number: 1
cards: 3
verify: go test -tags integration ./internal/weftengine/... ./internal/weftcli/... -count=1
depends-on: []
```

## Batch Scope

`weftengine.Commit` currently hardcodes its commit message to the unexported constant
`commitMessage = "weft sync"` (declared in `internal/weftengine/weft.go`). The `--undo`
feature (batch `initcli-undo`) needs to commit its weft-side deletion under an accurate,
distinct message, so `Commit` must accept a message parameter instead. This batch also
promotes `weftcli`'s package-private `envSyncOptions()` helper (reads
`WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`) into an exported `weftengine.EnvSyncOptions()`, since
the `--undo` path (living in a different package, `initcli`) needs the exact same
env-based test bypass and duplicating the two env-var string literals across packages
would violate DRY.

This batch changes a function signature that `weftcli` calls into, so it must update
**every** call site in the same batch — otherwise the repo would not compile between
batch commits. The external interface the next dependent batch (`initcli-undo`)
consumes is: `weftengine.Commit(weftPath string, pathspec []string, message string,
opts SyncOptions) (bool, error)`, `weftengine.DefaultCommitMessage` (exported string
constant), and `weftengine.EnvSyncOptions() SyncOptions`.

No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 1: Add a message parameter to `weftengine.Commit`; export `DefaultCommitMessage` and `EnvSyncOptions`

- **Context:** none
- **Edits:**
  - `internal/weftengine/weft.go`
  - `internal/weftengine/sync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/weftengine/weft.go`, rename the unexported `commitMessage` constant
    (in the `const (...)` block alongside `lockDirName`, `writeLockFile`,
    `pushLockFile`) to an exported `DefaultCommitMessage`, keeping its value
    `"weft sync"` unchanged. Update its doc comment to state it is the message used by
    every caller that does not need a custom one.
  - In `internal/weftengine/sync.go`, change `Commit`'s signature from
    `func Commit(weftPath string, pathspec []string, opts SyncOptions) (committed bool, err error)`
    to
    `func Commit(weftPath string, pathspec []string, message string, opts SyncOptions) (committed bool, err error)`.
    Use the `message` parameter in the `gitexec.RunGit([]string{"commit", "-m", message}, weftPath)`
    call instead of the old `commitMessage` constant reference. Update `Commit`'s doc
    comment to document the new parameter.
  - In `internal/weftengine/sync.go`, add an exported function
    `func EnvSyncOptions() SyncOptions` that reads the `WEFT_SKIP_GIT` and
    `WEFT_SKIP_PUSH` environment variables (`os.Getenv(...) == "1"`) and returns a
    `SyncOptions{SkipGit: ..., SkipPush: ...}`, with the exact same semantics as the
    `envSyncOptions()` helper currently in `internal/weftcli/cli.go` (this batch does
    not yet touch `weftcli`; that happens in Card 2). Add `"os"` to `sync.go`'s import
    block if not already present.
- **Commit:** `feat(weftengine): add Commit message param, export DefaultCommitMessage and EnvSyncOptions`

### Card 2: Update `weftcli` call sites for the new `Commit` signature and exported helpers

- **Context:**
  - `internal/weftengine/sync.go`
  - `internal/weftengine/weft.go`
- **Edits:**
  - `internal/weftcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Update the three `weftengine.Commit(...)` call sites (in `commitCmd`'s `RunE`,
    `pushCmd`'s `RunE`, and `syncCmd`'s `RunE`) to pass `weftengine.DefaultCommitMessage`
    as the new `message` argument, in the same position as documented in Card 1's new
    signature.
  - Replace all four call sites of the package-private `envSyncOptions()` (in
    `commitCmd`, `pushCmd`, `pullCmd`, `syncCmd`) with `weftengine.EnvSyncOptions()`.
  - Delete the now-unused package-private `envSyncOptions` function and its doc comment
    from `internal/weftcli/cli.go` (the `"os"` import may become unused as a result —
    remove it from the import block if so; check whether `os` is still used elsewhere
    in the file first).
  - Re-read `commitCmd`'s `Long` text ("The commit message is always the fixed string
    \"weft sync\" ... cannot be customized with a flag") and confirm it remains
    accurate: it does, since `weftcli` still always passes the fixed
    `weftengine.DefaultCommitMessage` and no new `weftcli` flag is introduced. No
    wording change is needed — this is a confirm-only step, not an edit.
- **Commit:** `refactor(weftcli): use weftengine.DefaultCommitMessage and EnvSyncOptions`

### Card 3: Update `weftengine` tests for the new `Commit` signature and add message-content coverage

- **Context:** none
- **Edits:**
  - `internal/weftengine/sync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Update every existing `Commit(...)` call site in this file (in `TestCommit`,
    `TestCommit_ScopedPathspec`, `TestPush`, `TestPull_FastForward` — four call sites
    total) to pass `DefaultCommitMessage` as the new `message` argument, matching the
    new signature from Card 1.
  - Add a new test `TestCommit_CustomMessage` that commits a change with a distinct
    custom message string (e.g. `"custom test message"`) and asserts that message (not
    `DefaultCommitMessage`) is what lands in the weft repo's history — read it via
    `git log -1 --format=%s` (`os/exec`, matching the existing `TestCommit`'s use of
    `exec.Command("git", ...)` with `cmd.Dir = weftRepo`).
- **Commit:** `test(weftengine): cover Commit message parameter`

## Batch Tests

`verify` runs `go test -tags integration ./internal/weftengine/... ./internal/weftcli/... -count=1`,
covering both the direct signature change (`weftengine/sync_test.go`) and its only
caller package (`weftcli/cli_test.go`, in particular `TestRunCLI_EnvMapToOption`, which
must keep passing unchanged as a black-box confirmation that the env-to-`SyncOptions`
mapping behavior is preserved after moving it into `weftengine.EnvSyncOptions()`). Both
test files require `-tags integration` (real git worktree fixtures via `lyxtest`).
