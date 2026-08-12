# Batch: state-updatejson

```yaml
task: 'fabric: close the corrindex two-phase read-modify-write race (slice 15)'
batch: 'state-updatejson'
number: 1
cards: 3
verify: go test ./internal/state/...
depends-on: []
```

## Batch Scope

This batch adds the read-modify-write primitive the whole task rests on, entirely inside `internal/state`, and touches no consumer.
It is one batch because the three cards are inseparable: the lock-free core extraction exists only so `UpdateJSON` can be written at all, `UpdateJSON`'s tests are the specification of its four error/creation dispositions plus its concurrency property, and the package doc states the rule the primitive exists to enforce.
The external interface batch 2 consumes is exactly one new exported function:

`func UpdateJSON[T any](path, lockPath string, mutate func(cur T, found bool) (T, error)) error`

Batch-local decision beyond `## Shared Decisions`: `UpdateJSON` follows `ReadJSON`/`WriteJSON`'s precedent of `os.MkdirAll(filepath.Dir(path))` **before** acquiring the lock, because `flock.New(lockPath).Lock()` fails outright when the parent directory is absent.
It inherits that precedent's limit unchanged — it creates the parent of `path`, never the parent of a `lockPath` living in a sibling tree.
`ReadJSONStrict` deliberately does not `MkdirAll` and is not touched by this batch.

## Cards

### Card 1: extract lock-free cores from `ReadJSON` and `WriteJSON`

- **Context:**
  - `internal/lock/lock.go`
  - `internal/fsx/fsx.go`
  - `internal/state/state_test.go`
  - `internal/state/strict_test.go`
- **Edits:**
  - `internal/state/state.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/state/state.go`, extract two unexported, lock-free helpers and re-express the existing exported functions on top of them, with no observable behaviour change to either.
  Add `readJSONUnlocked[T any](path string) (T, bool, error)` carrying exactly the body `ReadJSON` runs after `defer l.Release()` — the `os.ReadFile`, the `os.IsNotExist` branch returning `(zero, false, nil)`, the `json.Unmarshal`, and the `fmt.Errorf("unmarshal state: %w", err)` / `fmt.Errorf("read state: %w", err)` wrappings verbatim.
  Add `writeJSONUnlocked[T any](path string, v T) error` carrying exactly the body `WriteJSON` runs after `defer l.Release()` — the `json.MarshalIndent(v, "", "  ")` with its `fmt.Errorf("marshal state: %w", err)` wrapping, then `fsx.AtomicWriteBytes(path, data)`.
  Rewrite `ReadJSON` to keep its own `os.MkdirAll` and `lock.AcquireReadLock` prologue and then `return readJSONUnlocked[T](path)`, and `WriteJSON` to keep its own `os.MkdirAll` and `lock.AcquireWriteLock` prologue and then `return writeJSONUnlocked(path, v)`.
  Do not change `ReadJSONStrict`, the `ErrRead`/`ErrDecode` sentinels, or any error string.
  The existing tests in `internal/state/state_test.go` and `internal/state/strict_test.go` must pass unchanged — they are the regression cover for this extraction, and no test file is edited by this card.
- **Commit:** `refactor(state): extract lock-free read/write cores from ReadJSON and WriteJSON`

### Card 2: add `state.UpdateJSON` and its unit cover

- **Context:**
  - `internal/lock/lock.go`
  - `internal/state/state_test.go`
- **Edits:**
  - `internal/state/state.go`
- **Creates:**
  - `internal/state/update_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func UpdateJSON[T any](path, lockPath string, mutate func(cur T, found bool) (T, error)) error` to `internal/state/state.go`, built on card 1's `readJSONUnlocked`/`writeJSONUnlocked`.
  It must, in this order: `os.MkdirAll(filepath.Dir(path), 0o755)` with the existing `fmt.Errorf("mkdir: %w", err)` wrapping; acquire **one** exclusive lock via `lock.AcquireWriteLock(lockPath)` with `defer l.Release()`; call `readJSONUnlocked[T](path)`; call `mutate(cur, found)`; and write the returned value via `writeJSONUnlocked(path, next)`.
  It must never call `ReadJSON` or `WriteJSON` — both acquire `lockPath` internally and the composition hangs rather than failing, per `## Shared Decisions`.
  Dispositions, all of which abort with no write: a read error from `readJSONUnlocked` (including an existing file that fails to unmarshal) returns that error and `mutate` is never called; a `mutate` error is returned as-is.
  A missing file is not an error — `mutate` receives the zero `T` with `found=false`.
  Give it a doc comment stating that it holds one exclusive lock across read, mutate and write, that a missing file yields the zero value with `found=false`, and that a decode failure aborts without writing rather than handing `mutate` the zero value.
  Write `internal/state/update_test.go` as `package state_test`, reusing the `sample` type already declared in `internal/state/state_test.go` (do not redeclare it).
  Cover, one test function each: missing file — `mutate` sees the zero value and `found=false`, and the returned value is created on disk; existing file — `mutate` sees the decoded value and `found=true`, and the returned value replaces it; a `mutate` returning an error leaves an existing file byte-identical, and leaves a missing file still absent; an existing file holding invalid JSON aborts with a non-nil error, `mutate` never runs (assert via a flag the callback sets), and the file's bytes are unchanged; and a concurrency test where many goroutines each call `UpdateJSON` over a `[]int` (or equivalent slice type) appending one distinct element, after which every element is present exactly once.
  Drive the concurrency test through `UpdateJSON` itself rather than as a read phase followed by an update phase — the latter needs a barrier to fail deterministically pre-fix, which driving through the primitive avoids entirely.
  Use `t.TempDir()` for every path.
- **Commit:** `feat(state): add UpdateJSON read-modify-write primitive under one lock`

### Card 3: state the read-modify-write rule in a real godoc package comment

- **Context:**
  - `internal/state/state.go`
  - `internal/fabricengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/state/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/state/doc.go` holding a godoc package comment — the comment block must sit **immediately** above the `package state` clause with no blank line between them, unlike `state.go`'s existing detached header block, so it surfaces in `go doc`.
  The file contains only that comment and the `package state` clause.
  State the rule: a locked-JSON read-modify-write must hold one lock across the read and the write, and `UpdateJSON` is the primitive for it — reading with `ReadJSON`, mutating, and writing with `WriteJSON` releases the lock between the two and lets a concurrent writer's value be clobbered by a payload composed from a superseded base.
  Record why `UpdateJSON` is not composed from `ReadJSON` + `WriteJSON` (both acquire the lock path internally, so that composition hangs rather than failing), and that this is why the two exported functions sit on lock-free cores.
  State plainly that adoption is deliberately at one consumer today (`internal/fabricengine`'s correspondence index), which is why no `CONSTRAINTS.md` invariant asserts universal use — an invariant would be false on the day it landed.
  Follow `internal/fabricengine/doc.go`'s house voice: rationale the reader cannot recover from the code, not a restatement of the signature.
  Keep it short — this is a small leaf package.
- **Commit:** `docs(state): add package doc carrying the locked read-modify-write rule`

## Batch Tests

`verify: go test ./internal/state/...` runs the whole `internal/state` package, which is exactly this batch's blast radius: `state_test.go` and `strict_test.go` are the unchanged regression cover proving card 1's extraction is behaviour-preserving, and `update_test.go` is card 2's new cover.
The package is three source files and two existing test files, so running all of it is the scoped choice, not an unbounded one.
Card 3 adds no runnable surface of its own; it is covered by the package still compiling under the same command.
The overview's module-wide `verify: go build ./...` catches any cross-package fallout from `state.go`'s changed bodies at this batch boundary, where `internal/state` has consumers in four other engine packages.
