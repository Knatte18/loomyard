# Plan: board: use gitrepo as its git operator

```yaml
task: 'board: use gitrepo as its git operator'
slug: board-use-gitrepo
approved: true
started: 20260725-053134
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitrepo-stage-all
    file: 01-gitrepo-stage-all.md
    depends-on: []
    verify: go test -tags integration ./internal/gitrepo/...
  - number: 2
    name: boardengine-migration
    file: 02-boardengine-migration.md
    depends-on: [1]
    verify: go build ./... && go test -tags integration ./internal/boardengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: gitrepo is board's only git operator

- **Decision:** Board talks to git exclusively through a single `gitrepo.Repo`
  instance — `gitrepo.New`, `(*Repo).StageAllAndCommit`, and `(*Repo).Push`. No
  direct `gitexec.RunGit` calls remain in `internal/boardengine`. Board uses
  **`Push`**, not `PushCoalesced`, and does **not** call `CurrentSHA`,
  `SHAExists`, `ChangedFilesSince`, or the snapshot surface.
- **Rationale:** gitrepo is the typed layer every git-backed consumer shares;
  board is its first production consumer. Board needs no SHA-racing checks (it
  delegates rebase to gitrepo's `Push` and tracks no snapshot SHA).
- **Applies to:** all batches

### Decision: option-1 locking — keep board's push lock, use Push not PushCoalesced

- **Decision:** `Sync` keeps board's own top-level `pushLockFile`
  (`tasks.json.push.lock`), acquired once and held across the whole commit+push
  loop exactly as today, and calls `repo.Push()` inside it. `PushCoalesced` is
  NOT used — board's own lock is the cross-process single-pusher / coalescing
  mechanism.
- **Rationale:** dropping the top-level lock and pushing via `PushCoalesced`
  (which acquires only its own `.gitrepo-push.lock`) would let a concurrent
  `Sync`'s `commitDirty` (`add -A`) race a `Push`/`pull --rebase` on `.git/index`
  — a race today's shared lock prevents. Keeping board's lock + `Push()`
  preserves today's commit-vs-push mutual exclusion while still routing all raw
  git through gitrepo. See `_mill/discussion.md`
  `push-path-uses-gitrepo-Push-under-board's-own-lock`.
- **Applies to:** boardengine-migration

### Decision: StageAllAndCommit is board's documented wildcard exception

- **Decision:** The new `gitrepo.StageAllAndCommit` (wildcard `add -A`) is a
  separate method that never changes `StageAndCommit`'s explicit-file-list
  contract. `doc.go` documents it as **board's** opt-in escape hatch only —
  `fabric`/`raddle`/`codeintel` keep using explicit-list `StageAndCommit`.
- **Rationale:** keeps "never wildcard-stage" the default; makes the exception
  discoverable so a future reader does not read it as a general relaxation.
- **Applies to:** all batches

### Decision: Go verify shape (no PYTHONPATH prefix)

- **Decision:** This is a Go module; `verify:` commands use the native `go test`
  / `go build` runner directly, with no `PYTHONPATH= ` prefix (that prefix is a
  Python-project rule only).
- **Rationale:** the `verify-not-isolated` validator enforces `PYTHONPATH=` only
  for Python projects.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path
across every batch, sorted alphabetically (Move **source** paths are
excluded — they disappear, like `Deletes:` tokens)._

- `internal/boardengine/board.go`
- `internal/boardengine/sync.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitrepo/gitrepo_test.go`
- `internal/gitrepo/push.go`
- `manifest/roadmap.md`
