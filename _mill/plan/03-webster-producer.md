# Batch: webster-producer

```yaml
task: 'Shed engine adapters: SingleLLMProducer, perch, Webster'
batch: webster-producer
number: 3
cards: 2
verify: go test ./internal/shedadapters/...
depends-on: [1]
```

## Batch Scope

This batch delivers `WebsterProducer`, the adapter for Webster's black-box multi-spawn engine.
It consumes batch 1's `entryErr`/`cancelErr` helpers and adds nothing other batches consume, so it is independent of batch 2 and touches no file batch 2 touches.
Webster is the one engine whose entry point is already a free function, so its seam is func-typed and no interface is declared.

Batch-local decisions, additional to `## Shared Decisions` in the overview:

- `RunOptions.Fresh` is fixed `false` and is not configurable on the adapter.
  `Fresh: true` is the destructive fingerprint-mismatch escape — it archives state and reports and clears the rendered prompts — and must stay an explicit human act via the CLI, never something a Shed resume triggers.
- No mid-run bridge is installed.
  Webster's pause is an operator-owned flag file the batch loop polls, and writing it from a context watcher would conflate the two pause channels, race the run's own clear calls, and risk leaving the next invocation permanently paused.
  The accepted consequence is that a cancel is not observed until Master reaches a terminal outcome or its own configured whole-run timeout elapses.
- Webster's three outcome values are unexported, so the adapter compares against its own `"done"`/`"stuck"`/`"paused"` literals.
  The duplication is named rather than hidden: a `default:` branch errors on an unrecognised value and a test row drives it, so a webster-side rename surfaces as a failing test instead of a silently mis-mapped verdict.

## Cards

### Card 8: WebsterProducer — seam, constructor, and mapping

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/summary.go`
  - `internal/websterengine/outcome.go`
  - `internal/logger/logger.go`
  - `internal/shedadapters/ctx.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/webster.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare the func-typed seam `type WebsterRunner func(websterengine.RunDeps, websterengine.RunOptions) (websterengine.RunResult, error)` with the proof line `var _ WebsterRunner = websterengine.Run`.
  Declare `WebsterProducer` with unexported fields `name`, `run`, `deps`, and the constructor `NewWebsterProducer(name string, run WebsterRunner, deps websterengine.RunDeps) *WebsterProducer`, which stores `websterengine.Run` when `run` is nil.
  Add `var _ shedengine.ShedProducer = (*WebsterProducer)(nil)`.
  Declare the three package-local outcome literals as named constants (`websterOutcomeDone`, `websterOutcomeStuck`, `websterOutcomePaused`) with a comment stating that webster's own constants are unexported and that the `default:` branch below is what catches a rename.
  Implement `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` in this exact order.
  First `entryErr` with the engine label `"webster"`; a non-nil return exits immediately with the seam never invoked.
  Second, call `p.run(p.deps, websterengine.RunOptions{Fresh: false})`, writing `Fresh` explicitly rather than relying on the zero value, so the safety property is visible at the call site.
  Third, handle a non-nil error as a default with exactly one exception: an error matching `errors.Is(err, websterengine.ErrMasterAsking)` emits `logger.Warn` carrying the producer name, the engine label, and — when `errors.As` yields the concrete asking-error type — its message, session id, and run dir, then returns `shedengine.Stuck` with an empty `shedengine.OutputPointer` and a nil error;
  every other non-nil error is returned unwrapped, which covers the named sentinels for a died Master, a timed-out Master, a busy run, a fingerprint mismatch, and a nil batcher, and equally the unnamed errors the run returns for plan-validation refusal, zero batches, and directory or run-lock failures.
  Fourth, map `RunResult.Outcome` against the three literals: done returns `shedengine.Done` with `shedengine.OutputPointer{Path: websterengine.SummaryPath(p.deps.WebsterDir)}` and a nil error even under a cancelled context — the summary is Webster's human-readable account of the whole run and is guaranteed present on that outcome, and the webster dir is already told rather than derived;
  stuck emits `logger.Warn` carrying the producer name, the engine label, `RunResult.StuckReason`, and `RunResult.BatchesDone`, then returns `shedengine.Stuck` with an empty pointer and a nil error;
  paused returns a non-nil error naming it as an out-of-band pause and identifying the engine and the producer;
  a `default:` branch returns an error quoting the unrecognised value.
  Every non-success return path — the asking path, the stuck path, the paused path, every other engine error, and the `default:` branch — consults `cancelErr` first and returns that error instead when the context is cancelled;
  the done path never does.
  The pointer stays empty on every non-done path because summary parsing is best-effort there, so a named path could be a dead link in Shed's persisted history.
  Never write Webster's pause flag, and never construct a path under the webster scratch dir.
- **Commit:** `feat(shedadapters): add WebsterProducer over the webster run seam`

### Card 9: WebsterProducer tests

- **Context:**
  - `internal/shedadapters/webster.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedengine/producer.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/summary.go`
  - `internal/websterengine/pause.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/webster_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** write an untagged, in-package test file driven by a fake `WebsterRunner` that records the `websterengine.RunDeps` and `websterengine.RunOptions` it was handed and returns a caller-configured `websterengine.RunResult` and error.
  Cover the outcome table: done yields `shedengine.Done` with `websterengine.SummaryPath` over the told webster dir as the pointer;
  stuck yields `shedengine.Stuck` with an empty `shedengine.OutputPointer` and a nil error;
  paused under a healthy context yields a non-nil error naming the out-of-band pause;
  and an unrecognised outcome value yields an error quoting it, which is the row that turns a webster-side rename into a test failure.
  Cover the error table: the concrete asking error yields `shedengine.Stuck` with an empty pointer and a nil error, matched via `errors.Is` against the asking sentinel rather than by string comparison;
  the died, timed-out, busy-run, fingerprint-mismatch, and nil-batcher errors each yield a non-nil error;
  and a plain `errors.New` matching no sentinel at all yields a non-nil error rather than `shedengine.Stuck`, which is what pins the rule as a default with one exception rather than an enumeration.
  Assert `RunOptions.Fresh` is false on every call the adapter makes, read from the fake's recorded options — a safety property, not a default.
  Cover the context rows: an already-cancelled context returns an error with the seam never invoked;
  under a context cancelled during the call a returned done still maps to `shedengine.Done` with its summary pointer, while stuck, paused, and any error each become the context error.
  Finally assert no bridge is installed: with a `t.TempDir()` as the told scratch dir, `websterengine.PauseRequested` over that dir still reports false after a cancelled call, so the operator's own channel is provably untouched.
- **Commit:** `test(shedadapters): cover WebsterProducer mapping, Fresh, and cancellation`

## Batch Tests

`verify: go test ./internal/shedadapters/...` runs the new package's own untagged tests — this batch adds `webster_test.go` and re-runs the earlier files, which is the correct scope because this batch touches no file outside the package.
Every test stays tier 1: a func-typed fake for the run seam, `t.TempDir()` for the pause-flag absence row, and no real webster run, no batcher, no tmux, and no git.
