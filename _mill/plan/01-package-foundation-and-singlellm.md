# Batch: package-foundation-and-singlellm

```yaml
task: 'Shed engine adapters: SingleLLMProducer, perch, Webster'
batch: package-foundation-and-singlellm
number: 1
cards: 4
verify: go test ./internal/shedadapters/...
depends-on: []
```

## Batch Scope

This batch creates the new package `internal/shedadapters` and delivers its first adapter, `SingleLLMProducer`, together with the two shared helpers batches 2 and 3 consume: the context entry/exit helpers in `ctx.go` and the stale-output archive helper in `archive.go`.
Those two helpers are the external interface the next two batches build on — `entryErr`/`cancelErr` are called by all three adapters, while `archiveStaleOutputs` is used by `SingleLLMProducer` alone.
`SingleLLMProducer` is deliberately first: its behavior is fully determined before a line of it exists (four outcome rows, the archive step, three context checks), so its card ordering writes the table-driven test rows against a mapping that has no discretion left in it.

Batch-local decision, additional to `## Shared Decisions` in the overview: `SingleLLMProducer` takes an injected `now func() time.Time` clock, nil selecting `time.Now`.
This is the one place `manifest/designs/shed.md`'s "no injectable clock" rule does not apply — that rule governs Shed's own `history[].at` field, not an adapter-side archive filename whose same-second collision suffix is the thing under test.
Both cited precedents (`archiveStaleOutcome`, `ArchiveStaleSummary`) take exactly this seam.

## Cards

### Card 1: shared context entry/exit helpers

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/ctx_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create the package `shedadapters` with two unexported helpers, both taking `(ctx context.Context, name, engine string) error`, where `name` is the told producer name and `engine` is a short engine label (`"shuttle"`, `"perch"`, `"webster"`).
  `entryErr` returns nil when `ctx.Err()` is nil, and otherwise a formatted error naming the producer, the engine, and stating the context was cancelled before the run started, wrapping `ctx.Err()` with `%w`.
  `cancelErr` has the identical shape but states the context was cancelled during the run;
  it is the exit-precedence helper every non-success return path consults first.
  Both wrap with `%w` so a caller's `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)` hold — the tests assert exactly that.
  Prefix both messages with `shedadapters: ` to match the per-package error-prefix convention the sibling engines use.
  Add a file-header comment in the repo's style stating what the file holds and that the exit rule's one exception — a genuine success verdict survives cancellation — lives in each adapter's own `Call`, not here.
  Also write the accompanying untagged, in-package test file: a healthy context yields nil from both helpers;
  a cancelled context yields a non-nil error from both, whose text contains the told producer name and the engine label, and which satisfies `errors.Is(err, context.Canceled)`;
  a context past its deadline satisfies `errors.Is(err, context.DeadlineExceeded)`;
  and the two helpers' messages differ, so a reader can tell an entry refusal from an exit override.
- **Commit:** `feat(shedadapters): add shared context entry/exit helpers`

### Card 2: stale-output archive helper

- **Context:**
  - `internal/websterengine/summary.go`
  - `internal/websterengine/archive.go`
  - `internal/websterengine/outcome.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/archive_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** add `archiveStaleOutputs(files []string, now func() time.Time) error`, which renames every entry of `files` that exists on disk to a timestamped sibling and leaves absent entries alone without error.
  The target name is the entry's basename with its extension stripped, then `-<stamp>`, then the extension back, joined onto the entry's own directory — so `/abs/dir/review.md` becomes `/abs/dir/review-20260816T151326Z.md`.
  Declare the package-local constant `archiveTimestampFormat = "20060102T150405Z"` and format `now().UTC()` with it, mirroring `ArchiveStaleSummary`'s own stamp.
  Add the package-local `firstFreeArchivePath(candidate func(suffix string) string) (string, error)` collision helper: it returns the first path in the sequence `candidate("")`, `candidate("-1")`, `candidate("-2")`, … that does not exist, and returns the error from `os.Stat` when the failure is anything other than not-exist.
  Re-implementing it locally is deliberate — `websterengine`'s own copy is unexported, and the no-new-engine-surface rule forbids exporting it.
  A `now` of nil is not this helper's concern: the caller resolves nil to `time.Now` at construction.
  Also write the accompanying test file with `t.TempDir()`-backed rows: an existing file is renamed to the expected timestamped sibling under a fixed-instant fake clock and the original path is free afterwards;
  a second archive of a freshly re-created file under the same fixed instant lands on the `-1` suffix;
  an absent entry is a no-op returning nil;
  and a mixed list archives only the entries that exist.
- **Commit:** `feat(shedadapters): add stale-output archive helper`

### Card 3: SingleLLMProducer — seam, constructor, and outcome mapping

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/run.go`
  - `internal/burlerengine/engine.go`
  - `internal/loomengine/discussion.go`
  - `internal/logger/logger.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/singlellm.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare the seam `Shuttle` as `interface { Run(shuttleengine.Spec) (shuttleengine.Result, error) }` with the proof line `var _ Shuttle = (*shuttleengine.Runner)(nil)`, matching `burlerengine`'s own identically-shaped seam.
  Declare `type SpecSource func() (shuttleengine.Spec, error)` — the caller-supplied Spec source evaluated once per `Call`, the shape `loomengine.DiscussionSpec` already returns once its own arguments are bound.
  Declare `SingleLLMProducer` with unexported fields `name`, `specs`, `shuttle`, `now`, and the constructor `NewSingleLLMProducer(name string, specs SpecSource, shuttle Shuttle, now func() time.Time) *SingleLLMProducer`, which stores `time.Now` when `now` is nil.
  Add `var _ shedengine.ShedProducer = (*SingleLLMProducer)(nil)`.
  Implement `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` in this exact order.
  First `entryErr`; a non-nil return exits immediately with the seam never invoked.
  Second, evaluate `p.specs()`; an error is wrapped and returned with the seam never invoked.
  Third, reject a `Spec.OutputFiles` entry that is not `filepath.IsAbs`, with an error naming the offending entry and the producer, again without invoking the seam — absolute entries are a precondition because `Spec.validate` runs inside `Runner.Start`, after this archive step, and resolves relative entries against a worktree root this adapter must not read.
  Fourth, call `archiveStaleOutputs(spec.OutputFiles, p.now)` and wrap any error.
  Fifth, call `p.shuttle.Run(spec)`.
  Map the result: `shuttleengine.OutcomeDone` returns `shedengine.Done` with `shedengine.OutputPointer{Path: spec.OutputFiles[0]}` and a nil error even under a cancelled context, but guards the empty-list case first and returns an error rather than indexing, since a panic inside a long unattended Shed run is what the guard exists to avoid;
  `shuttleengine.OutcomeAsking` emits `logger.Warn` carrying the producer name, the engine label, `Result.LastAssistantMessage`, `Result.SessionID`, `Result.StrandGUID`, and `Result.RunDir`, then returns `shedengine.Stuck` with an empty pointer and a nil error;
  `shuttleengine.OutcomeDied` and `shuttleengine.OutcomeTimeout` each return a non-nil error whose text names both the outcome and the producer, since that text is what lands in Shed's persisted `error` field;
  a `default:` branch returns an error quoting the unrecognised value.
  Every non-success return path — the asking path, the died/timeout paths, the seam's own error, and the `default:` branch — consults `cancelErr` first and returns that error instead when the context is cancelled;
  the `OutcomeDone` path never does.
  Do not install a mid-run bridge of any kind: `shuttleengine` has no pause seam, and the accepted consequence is a wait bounded by `Spec.Timeout`.
- **Commit:** `feat(shedadapters): add SingleLLMProducer over the shuttle seam`

### Card 4: SingleLLMProducer tests

- **Context:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedengine/producer.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/run.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/singlellm_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** write an untagged, in-package test file driven by a fake `Shuttle` that records the `shuttleengine.Spec` it was handed and returns a caller-configured `shuttleengine.Result` and error, plus a fixed-instant clock function.
  Cover the outcome-mapping table: `OutcomeDone` yields `shedengine.Done` with `Spec.OutputFiles[0]` as the pointer and a nil error, asserted against a multi-entry `OutputFiles` so the first-entry convention is pinned rather than incidental;
  `OutcomeAsking` yields `shedengine.Stuck` with an empty `shedengine.OutputPointer` and a nil error;
  `OutcomeDied` and `OutcomeTimeout` each yield a non-nil error whose text contains both the outcome and the told producer name.
  Cover the remaining rows: a seam error propagates as a non-nil error distinguishable from the died/timeout mapping;
  an `OutcomeDone` with an empty `OutputFiles` returns an error rather than panicking;
  a `SpecSource` error surfaces without the fake seam ever being invoked;
  a relative `OutputFiles` entry is rejected with an error naming that entry, again with the seam never invoked.
  Cover the archive rows against `t.TempDir()`: a pre-existing output file is renamed to the expected timestamped sibling under the fixed clock and the original path is free by the time the fake seam runs (assert from inside the fake);
  a second archive under the same fixed instant takes the `-1` collision suffix;
  a missing output file is a no-op rather than an error;
  a nil `now` passed to the constructor still archives, asserted by shape rather than by a literal filename.
  Cover the context rows: an already-cancelled context returns an error satisfying `errors.Is(err, context.Canceled)` with the seam never invoked;
  a fake seam that cancels the context and then returns `OutcomeDone` still yields `shedengine.Done` with its pointer, with the output files left un-archived after the run;
  the same fake returning `OutcomeAsking` yields the context error instead of `shedengine.Stuck`.
  Finally assert no bridge is installed: the fake seam receives nothing but the `Spec` — no callback field, no cancellation channel — which is pinned by the seam's single-argument shape plus a row asserting the recorded spec is the one the source produced.
- **Commit:** `test(shedadapters): cover SingleLLMProducer mapping, archive, and cancellation`

## Batch Tests

`verify: go test ./internal/shedadapters/...` runs the new package's own untagged tests only — `archive_test.go` and `singlellm_test.go` in this batch.
That scope is exactly what the batch touches: no other package is edited, and the package is new, so no existing test file anywhere else can be affected.
Every test is tier 1 by construction — fakes for the seam, `t.TempDir()` for the filesystem rows, a fixed-instant clock for the archive rows, and no `exec.Command`, `gitexec.Run`, or fixture-tree copy anywhere.
