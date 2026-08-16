# Batch: perch-producer

```yaml
task: 'Shed engine adapters: SingleLLMProducer, perch, Webster'
batch: perch-producer
number: 2
cards: 3
verify: go test ./internal/shedadapters/...
depends-on: [1]
```

## Batch Scope

This batch delivers `PerchProducer`, the one adapter reusable by every review-gate producer regardless of which artifact it reviews.
It consumes batch 1's `entryErr`/`cancelErr` helpers and adds nothing other batches consume.
Two things make it the largest of the three adapters: it is the only one whose engine seam is a **factory** rather than a bare runner (perch's `PauseRequested` is fixed at construction, so a seam over a built engine could not install the context bridge at all), and it is the only one that resolves its own per-`Call` run identity from disk.

Batch-local decisions, additional to `## Shared Decisions` in the overview:

- The run-id is `<prefix>-<hash8>-<N>` with **two** varying segments.
  The `hash8` segment exists because `treadleengine.loadOrInitState` has a second refusal branch beyond the terminal one: a non-terminal block whose recorded profile hash differs is refused outright, which would wedge the producer permanently the first time an operator edits `perch.yaml` mid-bounce-loop.
  Namespacing by hash dissolves that branch instead of handling it, and it is the shipped convention — `perchengine.DeriveRunID` is literally `<sanitized-basename>-<hash[:8]>`.
- The adapter advances `N` only past a **terminal** block, so perch's own in-flight crash-resume survives while a completed block never blocks the next attempt.
- `NewPerchProducer` returns `(*PerchProducer, error)` — unlike the other two constructors — because `perchengine.ValidRunID` rejects an invalid prefix before any directory is touched.

## Cards

### Card 5: PerchProducer — seam, factory, constructor, and run-id resolution

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/perchengine/engine.go`
  - `internal/perchengine/result.go`
  - `internal/perchengine/identity.go`
  - `internal/perchengine/profile.go`
  - `internal/perchcli/pause.go`
  - `internal/treadleengine/state.go`
  - `internal/shedadapters/ctx.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/perch.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare the seam `PerchRunner` as `interface { Run(p perchengine.Profile, runDir, scratchDir, stencilsDir string) (perchengine.Result, error) }` with the proof line `var _ PerchRunner = (*perchengine.Engine)(nil)`, and the factory type `type PerchFactory func(pauseRequested func() bool) PerchRunner`.
  The factory, not a built engine, is the seam because `perchengine.Options.PauseRequested` is consumed by `perchengine.New` at construction time, so a seam over an already-constructed engine could never install the bridge.
  Declare `PerchProducer` with unexported fields `name`, `factory`, `profile`, `runDirBase`, `scratchDirBase`, `stencilsDir`, `runIDPrefix`, and the constructor `NewPerchProducer(name string, factory PerchFactory, profile perchengine.Profile, runDirBase, scratchDirBase, stencilsDir, runIDPrefix string) (*PerchProducer, error)`, which returns an error when `perchengine.ValidRunID(runIDPrefix)` is false, when `factory` is nil, or when any of the three directory arguments is empty.
  Add `var _ shedengine.ShedProducer = (*PerchProducer)(nil)`.
  Implement the unexported `resolveRunID() (runID, runDir, scratchDir string, err error)`.
  It computes `perchengine.ProfileHash(p.profile)` and takes its first 8 hex characters as `hash8`, propagating a hashing error unchanged.
  It then reads `p.runDirBase`'s directory entries: an absent base directory starts at `N = 1` without error, any other `os.ReadDir` failure is returned, and each directory entry whose name matches `<prefix>-<hash8>-<N>` — with `N` a positive decimal integer carrying no leading zeros — contributes its `N`, of which the highest is taken.
  Entries carrying a different `hash8` are ignored, never adopted and never deleted.
  With the highest `N` in hand it joins `runDir = filepath.Join(p.runDirBase, runID)` and `scratchDir = filepath.Join(p.scratchDirBase, runID)`, calls `os.MkdirAll` on `scratchDir` (propagating a failure), and probes `perchengine.TerminalOutcome(runDir, scratchDir)`.
  The `os.MkdirAll` is mandatory rather than incidental: `TerminalOutcome` acquires a read lock inside the scratch dir, and the lock helper opens its lock file without creating the parent, so an absent scratch sibling — the ordinary state after a fresh clone, since the run dirs are tracked and the scratch tree never is — would otherwise become a permanent producer failure.
  A probe error is returned as the adapter's own error, failing the `Call`;
  an absent `state.json` is not an error, since the probe reports not-found rather than failing.
  A **non-terminal** block is reused verbatim so perch resumes its own in-flight rounds;
  a **terminal** block advances to `N+1`, recomputing both joined paths for the new run-id, which is a fresh directory that needs no second probe.
  Hold no attempt number in adapter memory — it is rediscovered on every `Call`, so a process restart resolves the same attempt.
  Build no path other than these two joins onto a told base;
  never write the tokens for the tracked and never-tracked lyx directory names.
- **Commit:** `feat(shedadapters): add PerchProducer seam, factory, and run-id resolution`

### Card 6: PerchProducer.Call — bridge installation and outcome mapping

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/perchengine/engine.go`
  - `internal/perchengine/result.go`
  - `internal/logger/logger.go`
  - `internal/shedadapters/ctx.go`
- **Edits:**
  - `internal/shedadapters/perch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** implement `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` in this exact order.
  First `entryErr` with the engine label `"perch"`; a non-nil return exits immediately with the factory never invoked.
  Second, `resolveRunID`; its error is wrapped and returned.
  Third, invoke `p.factory(func() bool { return ctx.Err() != nil })` — once per `Call`, so a second `Call` under a fresh context gets a fresh bridge rather than a closure over the first one — and return an error when the factory yields a nil `PerchRunner`.
  Fourth, call the returned engine's `Run` with `p.profile`, the resolved `runDir` and `scratchDir`, and `p.stencilsDir`.
  Map the result: `perchengine.OutcomeApproved` returns `shedengine.Done` with an **empty** `shedengine.OutputPointer` and a nil error even under a cancelled context — the empty pointer is a decision, not an omission, because a review gate is the canonical gate producer and its verdict must always be re-derived rather than inferred from a file;
  `perchengine.OutcomeStuck` emits `logger.Warn` carrying the producer name, the engine label, `Result.StuckReason`, `Result.RoundsRun`, and the resolved run dir, then returns `shedengine.Stuck` with an empty pointer and a nil error;
  `perchengine.OutcomePaused` returns a non-nil error naming it as an out-of-band pause and identifying the engine and the producer;
  a `default:` branch returns an error quoting the unrecognised value.
  Every non-success return path — the stuck path, the paused path, the seam's own error, the resolve error, and the `default:` branch — consults `cancelErr` first and returns that error instead when the context is cancelled;
  the `OutcomeApproved` path never does.
  Responsiveness is round-granular because perch checks its pause callback between rounds, which is the correct granularity for an orderly drain, and no adapter code may read Shed's status file or accept a caller-supplied pause callback — the context is the sole pause channel.
- **Commit:** `feat(shedadapters): map perch outcomes and install the context bridge`

### Card 7: PerchProducer tests

- **Context:**
  - `internal/shedadapters/perch.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedengine/producer.go`
  - `internal/perchengine/result.go`
  - `internal/perchengine/identity.go`
  - `internal/perchengine/profile.go`
  - `internal/treadleengine/state.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/perch_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** write an untagged, in-package test file driven by a fake `PerchFactory` that records the `pauseRequested` callback it was handed, records the `perchengine.Profile` and the three directory arguments its returned `PerchRunner` was called with, counts its own invocations, and returns a caller-configured `perchengine.Result` and error.
  Cover the mapping table: `OutcomeApproved` yields `shedengine.Done` with an empty `shedengine.OutputPointer` — asserted explicitly, since it is a decision a future reader must see pinned;
  `OutcomeStuck` yields `shedengine.Stuck`, an empty pointer, and a **nil** error for each of the three `perchengine.StuckReason` values, asserting the seam's shape rather than log text;
  `OutcomePaused` under a healthy context yields a non-nil error whose text names the out-of-band pause.
  Cover the context rows: an already-cancelled context returns an error with the factory never invoked;
  the recorded callback reports false before cancellation and true after, which is what proves the bridge is installed rather than merely intended;
  under a cancelled context a returned `OutcomeApproved` still maps to `shedengine.Done`, while `OutcomeStuck` and `OutcomePaused` each become the context error;
  and two `Call`s record two distinct factory invocations.
  Cover run-id advancement against `t.TempDir()`, seeding state by hand-writing `<runDir>/state.json` against the treadle run-state struct's own json tags — `{"outcome": "APPROVED"}` for a terminal block and `{"outcome": ""}` (or the field omitted, since it is `omitempty`) for an in-flight one — because that struct is unexported and cannot be constructed from this package.
  Writing `H` for this profile's `hash8`: a terminal `<prefix>-H-1` makes the next `Call` run against `<prefix>-H-2`;
  a non-terminal `<prefix>-H-1` is reused;
  an empty base starts at `<prefix>-H-1`;
  and with both `<prefix>-H-1` and `<prefix>-H-2` present and `-2` terminal, the next `Call` runs against `<prefix>-H-3`, pinning highest-N rather than first-gap.
  Cover profile-hash namespacing: with a non-terminal `<prefix>-H1-1` on disk, a producer carrying a different `perchengine.Profile` resolves to `<prefix>-H2-1` and leaves `H1`'s directory untouched — no reuse, no deletion, no error — and the resolved `hash8` equals the first 8 characters of `perchengine.ProfileHash` over that profile, so the id is verifiably derived rather than merely different.
  Cover the remaining filesystem rows: a run dir whose scratch sibling does not exist resolves normally, with the adapter creating it and the probe reporting non-terminal;
  a corrupt `state.json` (unparseable bytes, scratch dir present) fails the `Call` with a propagated error;
  the `runDir`/`scratchDir` pair handed to the engine is each base joined with the same run-id, asserted because a mismatched pair would put treadle's state lock in the wrong tree;
  an invalid `runIDPrefix` is rejected by the constructor before any directory is touched;
  and a seam error propagates unchanged.
- **Commit:** `test(shedadapters): cover PerchProducer mapping, bridge, and run-id resolution`

## Batch Tests

`verify: go test ./internal/shedadapters/...` runs the new package's own untagged tests — this batch adds `perch_test.go` and re-runs batch 1's files, which is the correct scope because this batch touches no file outside the package.
Every test stays tier 1: fakes for the factory and the runner, `t.TempDir()` plus hand-written `state.json` bytes for the run-id rows, and no real perch engine, treadle round loop, tmux, or git anywhere.
The hand-written `state.json` seeding is deliberate and named in card 7 — treadle's run-state struct is unexported, so a test in this package cannot construct one, and the json tags are the contract the seeding relies on.
