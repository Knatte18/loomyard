# Plan: Adopt internal/state in board and muxpoc

```yaml
task: "Adopt internal/state in board and muxpoc"
slug: "adopt-internal-state"
approved: true
started: "20260622-180236"
parent: "main"
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
    name: state-explicit-lock-path
    file: 01-state-explicit-lock-path.md
    depends-on: []
    verify: go test ./internal/state/
  - number: 2
    name: board-adopt-state
    file: 02-board-adopt-state.md
    depends-on: [1]
    verify: go test ./internal/board/...
  - number: 3
    name: muxpoc-adopt-state
    file: 03-muxpoc-adopt-state.md
    depends-on: [1]
    verify: go test ./internal/muxpoc/...
```

## Shared Decisions

### Decision: explicit lock path is the enabling change

- **Decision:** `state.WriteJSON` / `state.ReadJSON` gain an explicit `lockPath string`
  parameter and use it verbatim instead of deriving `path + ".lock"`. Every adopting
  call site names the lock file it wants. Batch 1 ships this; batches 2 and 3 consume it.
- **Rationale:** board MUST lock on a file distinct from `<path>.lock` — its coarse
  write lock is literally `tasks.json.lock`, held by `board.writeOp` across
  Load → mutate → Save. If `state` hardcoded `<path>.lock`, board's `Save` would
  re-acquire the coarse lock the caller already holds → self-deadlock, and routing
  `Load` through it would force readers onto the coarse lock. An explicit lock path
  lets board pass its swap lock (`tasks.json.swaplock`) and lets muxpoc pass
  `<statePath>.lock`. `state` has no production callers yet, so the signature change
  only touches its own tests.
- **Applies to:** all batches

### Decision: corruption surfaces as an error (no swallowing)

- **Decision:** a corrupt / unparseable JSON file is returned as an `error` at every
  read site. `board.Load` no longer falls back to an empty task list on a JSON parse
  error; `muxpoc.LoadState` no longer returns a warning string for a corrupt file.
  This matches `state.ReadJSON`, which already surfaces unmarshal errors.
- **Rationale:** explicit operator instruction — swallowing errors is not acceptable.
  A silent-empty board or an ignored-corrupt session file hides real data loss; failing
  loudly forces the corrupt file to be dealt with. Genuine read errors (non-NotExist)
  already surfaced at both sites and continue to.
- **Applies to:** board-adopt-state, muxpoc-adopt-state

### Decision: a site's Save and Load must lock the same file

- **Decision:** within each adopting package, the write path and the read path pass the
  **same** lock path to `state`. board: both use `<tasks.json>.swaplock`. muxpoc: both
  use `<muxpoc-state.json>.lock`.
- **Rationale:** the read/write fence only works if readers and writers contend on one
  lock file. Changing one without the other silently breaks mutual exclusion.
- **Applies to:** board-adopt-state, muxpoc-adopt-state

### Decision: Go test runner, behaviour-preserving except corruption

- **Decision:** verify with the native Go runner (`go test ./internal/<pkg>/...`), no
  `PYTHONPATH=` prefix (this is a Go project). Existing tests are the guardrail: the
  persistence round-trips, the DependsOn nil→`[]` normalization, and the board
  concurrency tests must keep passing unchanged. Only the corruption-handling tests
  change, intentionally, to assert errors.
- **Rationale:** the task is a behaviour-preserving cleanup apart from the deliberate
  corruption-as-error change. A regression in the board concurrency tests would mean the
  swap-lock / coarse-lock split was broken (the deadlock).
- **Applies to:** all batches

## All Files Touched

- `internal/board/board.go`
- `internal/board/store.go`
- `internal/board/store_test.go`
- `internal/muxpoc/attach.go`
- `internal/muxpoc/daemon.go`
- `internal/muxpoc/down.go`
- `internal/muxpoc/muxpoc_smoke_test.go`
- `internal/muxpoc/review.go`
- `internal/muxpoc/state.go`
- `internal/muxpoc/state_test.go`
- `internal/muxpoc/status.go`
- `internal/muxpoc/up.go`
- `internal/state/state.go`
- `internal/state/state_test.go`
