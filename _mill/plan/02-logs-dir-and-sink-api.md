# Batch: logs-dir-and-sink-api

```yaml
task: 'webster standalone mode: run refuses to start Master; logs write untracked into target repo'
batch: 'logs-dir-and-sink-api'
number: 2
cards: 2
verify: go test ./internal/standalonegeom/... ./internal/logger/...
depends-on: []
```

## Batch Scope

This batch delivers the two pieces F22's fix is assembled from, neither of which has a caller yet: the pure path helper that names standalone's trace-log directory, and the production entry point on `internal/logger` that sets a sink directory and the trace header's worktree root in one atomic call.
Both are additive — no existing exported signature changes and no existing behaviour moves — which is what lets this batch run in parallel with batch 1.
Batch 3 is the sole consumer of both.

The external interface batch 3 consumes is two exported functions:

```go
func LogsDir(stateDir string) string
func SetDurableSinkDirWithWorktreeRoot(dir, worktreeRoot string)
```

Batch-local decision: card 4 refactors `SetDurableSinkDir`'s reset body into a shared unexported helper rather than duplicating it, so the two entry points cannot drift apart in what they reset.

## Cards

### Card 3: add standalonegeom.LogsDir as the sole declarer of standalone's trace-log directory

- **Context:**
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/standalonegeom/doc.go`
  - `internal/standalonegeom/standalonegeom_test.go`
- **Creates:**
  - `internal/standalonegeom/logsdir.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/standalonegeom/logsdir.go` with a file header comment in the same shape `internal/standalonegeom/stencilsdir.go` uses, and one exported function `LogsDir(stateDir string) string` returning `filepath.Join(stateDir, lyxdirs.DotLyxDirName, "logs")`.
  Build the path from `lyxdirs.DotLyxDirName` — the Lyxdirs Single-Declarer Invariant forbids naming the `.lyx` literal in path-construction context anywhere outside `internal/lyxdirs`.
  Like every other builder in the package it takes only `stateDir`: it must not call `standalonestate.Derive`, must not read the environment, and must not touch disk, so it stays a pure function and the package stays hermetic.
  Its doc comment must state that it is the sole construction site for the standalone trace-log directory across every standalone-capable CLI, mirroring hub mode's `<anchor>/.lyx/logs` with `stateDir` playing the anchor's role, exactly as `StencilsDir` mirrors the hub stencils directory.
  The doc comment must additionally state, in its own sentence, that reed's own `<stateDir>/logs` — the `LogsDir` field `ReedGeometry` sets on the `reedengine.Geometry` it returns — is a different directory for a different producer and is deliberately not converged with this one.
  That sentence is load-bearing, not decoration: after this card the package carries two same-shaped log-path constructions side by side and the next reader will otherwise try to unify them.
  Do not edit `internal/standalonegeom/reedgeom.go`.
  In `internal/standalonegeom/doc.go`, the package doc's "standalonegeom's contract today is …" sentence enumerates the exported surface;
  add `LogsDir` to that enumeration beside `StencilsDir`, describing it as converting a told `stateDir` alone into the standalone trace-log directory path.
  In `internal/standalonegeom/standalonegeom_test.go`, add a `TestLogsDir` beside the existing `TestStencilsDir`, in the same shape: `t.Parallel()`, a fixed literal `stateDir`, and an assertion against `filepath.Join(stateDir, lyxdirs.DotLyxDirName, "logs")`.
  Add one further assertion in that test that `LogsDir(stateDir)` and `ReedGeometry(target, stateDir, hash8).LogsDir` are different values, pinning the deliberate non-convergence the doc comment records.
- **Commit:** `feat(standalonegeom): add LogsDir, the sole declarer of standalone's trace-log directory`

### Card 4: add an atomic production entry point for the durable sink directory and header worktree root

- **Context:**
  - `internal/logger/logger.go`
  - `internal/logger/trace.go`
- **Edits:**
  - `internal/logger/sink.go`
  - `internal/logger/sink_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/logger/sink.go`, extract the body of `SetDurableSinkDir` — everything it does after taking `sinkMu` — into an unexported `resetDurableSinkLocked(dir string)`, and have `SetDurableSinkDir` become a lock-plus-delegate wrapper around it.
  `SetDurableSinkDir` keeps its exact name and its exact `func SetDurableSinkDir(dir string)` signature, so all twenty-one existing call sites are untouched;
  do not widen it.
  Add `SetDurableSinkDirWithWorktreeRoot(dir, worktreeRoot string)`, which takes `sinkMu`, calls `resetDurableSinkLocked(dir)`, and then sets `header.WorktreeRoot = worktreeRoot`.
  The order matters and is the whole reason this is one call rather than two: `resetDurableSinkLocked` zeroes `header` and `headerOnce` as part of its reset, so a worktree root supplied by any separate call made beforehand would be silently wiped.
  Setting it after the reset is safe because `armHeader` populates `Command`, `Argv`, `TraceID` and `PID` only and never writes `WorktreeRoot`.
  Rewrite `SetDurableSinkDir`'s doc comment: it currently reads "sets the durable sink directory for testing", which is no longer what it is.
  It is now the documented shorthand for "this directory, no worktree root supplied", used by tests and by production alike, and its comment must say so and must point at `SetDurableSinkDirWithWorktreeRoot` as the entry point that supplies one.
  Both entry points' doc comments must state the ordering obligation explicitly: the directory only takes effect if it is set before the first record that arms the sink, because `ensureDurableSink` is `sync.Once`-guarded and reads `sinkDirOverride` once.
  `SetDurableSinkDirWithWorktreeRoot`'s comment must additionally say what the worktree root is for — it is trace metadata answering "which repository was this process working on", a separate concern from where the trace file lands, which is the same distinction the existing comment beside `header.WorktreeRoot = layout.WorktreePath()` in `ensureDurableSink` already draws.
  Make no change to `ensureDurableSink`, to `headerLine`, to `NotifyExit`, or to the cwd-derived fallback path's own `header.WorktreeRoot` assignment.
  In `internal/logger/sink_test.go`, rename `TestEnsureDurableSink_SeamPathLeavesWorktreeRootEmpty` to name the reason rather than the seam — an empty header is now a choice the caller made by picking the no-worktree-root-supplied shorthand, not a property of the seam — and update its failure message and doc comment to match.
  Its body, its `SetDurableSinkDir(dir)` call and its empty-header expectation stay as they are.
  Add three tests beside it.
  First, the populated counterpart: `SetDurableSinkDirWithWorktreeRoot(dir, worktreeRoot)` followed by `ensureDurableSink()` leaves `header.WorktreeRoot` equal to the supplied value, and the trace file's first line carries it.
  Second, the redirect actually redirects: with the override set before the first record, the trace file lands in the given directory and no file appears in the cwd-derived location.
  Third, the ordering obligation as an executable assertion rather than only a doc sentence: setting the directory after the sink is already armed does not move the file that was already opened.
  Every one of these tests registers `t.Cleanup(func() { SetDurableSinkDir("") })`, per the overview's `sink-override-is-process-global` decision.
- **Commit:** `feat(logger): add SetDurableSinkDirWithWorktreeRoot as the atomic production sink entry point`

## Batch Tests

`verify: go test ./internal/standalonegeom/... ./internal/logger/...` runs the untagged suites of exactly the two packages this batch edits.
`internal/standalonegeom` covers card 3's new `TestLogsDir` and the untouched `TestStencilsDir`/`TestReedGeometry` beside it;
`internal/logger` covers card 4's three new sink tests, the renamed seam test, and — the reason the whole package is run rather than one file — the nineteen pre-existing tests that depend on `SetDurableSinkDir` keeping its current name, signature, and reset semantics.
Those nineteen are the load-bearing proof that card 4's extraction of `resetDurableSinkLocked` changed nothing observable.
