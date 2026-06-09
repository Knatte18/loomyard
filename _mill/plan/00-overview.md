# Plan: Cut boardtest concurrency test run time

```yaml
task: "Cut boardtest concurrency test run time"
slug: "boardtest-concurrency-speed"
approved: true
started: "20260609-103009"
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
    name: reduce-writes
    file: 01-reduce-writes.md
    depends-on: []
    verify: go test ./internal/board/boardtest/
```

## Shared Decisions

### Decision: test-only-change

- **Decision:** Change only `internal/board/boardtest/concurrency_test.go`. No production code (`board.go`, `git.go`, `render.go`, `store.go`) is touched, no `testing.Short()` gating is added, and no test-only seam is added to `Board.writeOp`.
- **Rationale:** The slowness is filesystem-bound (each write = 3 XDR-scanned temp-create+rename ops); cutting the writer's iteration count alone reaches the wall-clock target with the smallest possible diff. See discussion.md Decisions § reduce-writes-constant / no-production-seam / no-short-gating.
- **Applies to:** all batches

### Decision: go-native-verify

- **Decision:** This is a Go project; `verify:` uses the native `go test` runner with no `PYTHONPATH=` prefix.
- **Rationale:** The `PYTHONPATH=` prefix rule applies only to Python/mill projects. `go test ./internal/board/boardtest/` runs exactly the two `Test*` functions in the package (benchmarks need `-bench`; integration tests need `-tags integration`), so the scope is already the affected tests.
- **Applies to:** all batches

## All Files Touched

- `internal/board/boardtest/concurrency_test.go`
