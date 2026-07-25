# Batch: boardengine-migration

```yaml
task: 'board: use gitrepo as its git operator'
batch: boardengine-migration
number: 2
cards: 5
verify: go build ./... && go test -tags integration ./internal/boardengine/...
depends-on: [1]
```

## Batch Scope

This batch rewires `internal/boardengine`'s live git plumbing onto the
`gitrepo.Repo` added in batch 1, deletes the dead `git.go` call site, folds the
durable design into the package doc, and finishes the documentation lifecycle
(delete the design doc, move the roadmap item to Done). It depends on batch 1
because `sync.go` calls `gitrepo.StageAllAndCommit`. The boardengine code cards
(4, 5) form an atomic unit: `sync.go` must stop using `BoardPushError` (Card 4)
before `git.go` — which defines it — is deleted (Card 5). Board's public `Sync`
API and its locking behavior are unchanged, so the existing
`boardtest/sync_test.go` is the safety net and needs no edits. Batch-local:
none beyond `## Shared Decisions`.

## Cards

### Card 4: Rewrite sync.go onto gitrepo (Push under board's own push lock)

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/push.go`
- **Edits:**
  - `internal/boardengine/sync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Route all of `sync.go`'s git through one `gitrepo.Repo`.
  Imports: remove `github.com/Knatte18/loomyard/internal/gitexec`; add
  `github.com/Knatte18/loomyard/internal/gitrepo`; keep `os`, `path/filepath`,
  `strings`, `fmt`, and the `flock "github.com/Knatte18/loomyard/internal/lock"`
  alias. In `Sync(boardPath string, skipGit, skipPush bool)`: keep the `skipGit`
  early return, the `ensureLockfilesIgnored(boardPath)` call, and the top-level
  push-lock acquisition `flock.AcquireWriteLock(filepath.Join(boardPath,
  pushLockFile))` held (defer Release) across the whole loop — unchanged.
  Construct `repo := gitrepo.New(boardPath)` before the loop. Loop body:
  `committed, err := commitDirty(repo, boardPath)` (return on err); then
  `if !skipPush { if err := repo.Push(); err != nil { return fmt.Errorf("sync
  push: %w", err) } }`; then `if !committed { return nil }`. Change `commitDirty`
  to `func commitDirty(repo *gitrepo.Repo, boardPath string) (bool, error)`:
  acquire `flock.AcquireWriteLock(filepath.Join(boardPath, writeLockFile))`
  (defer Release), then `_, committed, err := repo.StageAllAndCommit("board
  sync")`; on err return `false, fmt.Errorf("sync commit: %w", err)`; else return
  `committed, nil`. Remove the `git status --porcelain`, `git add -A`, and `git
  commit` `gitexec.RunGit` calls. Delete the `pushUnpushed` and `hasUnpushed`
  functions entirely. Keep `ensureLockfilesIgnored` and the `writeLockFile` /
  `pushLockFile` consts unchanged. No `BoardPushError` reference may remain in
  the file. Update the file-level doc comment at the top of `sync.go` so it no
  longer describes the deleted `hasUnpushed` short-circuit ("pushes all unpushed
  commits, looping until nothing is left"): state that each loop iteration
  commits dirty state via `gitrepo.StageAllAndCommit` and, unless `skipPush`,
  pushes via `gitrepo.Push` (which owns the rebase-retry) unconditionally, with
  the single top-level push lock still serializing pushers.
- **Commit:** `refactor(board): route sync.go git through gitrepo.Repo`

### Card 5: Delete dead git.go plumbing and its test

- **Context:**
  - `internal/boardengine/sync.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/boardengine/git.go`
  - `internal/boardengine/boardtest/git_test.go`
- **Moves:** none
- **Requirements:** Delete `internal/boardengine/git.go` in full — it defines
  `Pull`, `CommitPush`, and the `BoardPushError` type, all now unreferenced
  (`Pull`/`CommitPush` have no production callers; `BoardPushError` is no longer
  used after Card 4's `sync.go` rewrite). Delete
  `internal/boardengine/boardtest/git_test.go`, which only tested `Pull` and
  `CommitPush`. Before deleting, confirm no remaining references to `Pull`,
  `CommitPush`, or `BoardPushError` exist in `internal/boardengine` (grep the
  package).
- **Commit:** `chore(board): delete dead Pull/CommitPush git plumbing`

### Card 6: Fold durable design into boardengine package doc

- **Context:**
  - `internal/boardengine/sync.go`
- **Edits:**
  - `internal/boardengine/board.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the `package boardengine` doc comment at the top of
  `board.go` to fold in the durable design (per the Documentation Lifecycle):
  add one or two sentences stating that the detached sync path commits and
  pushes through a single `gitrepo.Repo` (`StageAllAndCommit` + `Push`) under
  board's own write and push locks, rather than hand-rolled `gitexec` calls. Do
  not restate the full lock protocol — keep it to the durable summary.
- **Commit:** `docs(board): note gitrepo-backed sync in package doc`

### Card 7: Delete the superseded design doc

- **Context:**
  - `internal/boardengine/board.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `manifest/designs/board-use-gitrepo.md`
- **Moves:** none
- **Requirements:** Delete `manifest/designs/board-use-gitrepo.md`. Its own
  header declares it is deleted when the work lands ("Status: Design — not
  built… durable parts fold into `internal/boardengine`'s package doc when this
  lands and this file is deleted"), and Card 6 performed that fold. Required by
  the Documentation Lifecycle constraint.
- **Commit:** `docs(manifest): delete landed board-use-gitrepo design doc`

### Card 8: Move roadmap item from Planned to Done

- **Context:**
  - `internal/boardengine/board.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, remove the `## Planned` entry
  **board: use `gitrepo` as its git operator** (currently the first Planned
  item, whose body links to `designs/board-use-gitrepo.md`) and record it under
  `## Done` as a completed item — board now talks to git through `gitrepo.Repo`,
  folded into `internal/boardengine` (do **not** link the deleted
  `designs/board-use-gitrepo.md`; reference `internal/boardengine` instead, in
  the style of the existing Done `gitrepo` entry). Leave the `git-native-library`
  Planned entry's "Depends on `board-use-gitrepo` landing first" wording intact —
  it cross-references by bold item name, which stays valid.
- **Commit:** `docs(manifest): move board-use-gitrepo to Done`

## Batch Tests

`verify: go build ./... && go test -tags integration ./internal/boardengine/...`.
The `go build ./...` compiles every production package repo-wide, catching any
dangling reference to the deleted `Pull` / `CommitPush` / `BoardPushError`
symbols. The scoped `go test -tags integration ./internal/boardengine/...` runs
the boardengine unit and black-box `boardtest` packages — including the
unchanged `sync_test.go` (commit+push, burst coalescing, skipPush, clean no-op,
lockfile-ignore), which is the behavior-preserving safety net for the sync.go
rewrite — and confirms the `git_test.go` deletion leaves the test build green.
Docs-only cards (7, 8) have no runnable surface; they are covered by the same
build+test gate not regressing.
