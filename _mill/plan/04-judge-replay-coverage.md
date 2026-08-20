# Batch: judge-replay-coverage

```yaml
task: 'Bouncer: the generic review-gate producer'
batch: 'judge-replay-coverage'
number: 4
cards: 3
verify: go test ./internal/shedadapters/...
depends-on: [3]
```

## Batch Scope

This batch adds only test files: the judge-call, harvest, replay, degradation, pointer-discipline, stale-output, and cancellation coverage batch 3 deliberately left out so its own cards stayed a reviewable size.
It edits no production file.
If a case here fails, the fix belongs in `internal/shedadapters/bouncer.go`, and any such fix is part of this batch's work — a red test is a defect in card 6's implementation, not a reason to weaken the assertion.
No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 9: judge call, happy paths and prompt inputs

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/bouncer_config_test.go`
  - `internal/shedadapters/bouncer_seed_test.go`
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedadapters/archive_test.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/bouncer_judge_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedadapters/bouncer_judge_test.go`, reusing `fakeShuttle`, `fixedClock`, and the stencils-fixture helper the earlier test files already establish.
  Add a local helper that lays out a run dir at a given round: it writes the round producer's report for each round, and optionally the verdict, ledger, and focus files, so each case below states its disk state in one call.

  Judge call, `APPROVED`: with a report for round 1 on disk and no verdict, the fake writes all three declared output files and reports completion; `Call` returns `shedengine.Done`, the pointer is `round-1-bouncer-ledger.md`, and the error is nil.
  Assert `round-2-focus.md` exists afterwards and parses with an empty `exclude_lenses` and an empty `focus`.
  Judge call, `BLOCKING`: same setup with a `BLOCKING` verdict; `Call` returns `shedengine.Stuck`, the pointer is the round-1 ledger, the error is nil, and all three files exist afterwards.
  Unconditional `OutputFiles`: assert the recorded judge `Spec.OutputFiles` has exactly three entries on both the `APPROVED` and the `BLOCKING` round, and that the next round's focus file exists after the `APPROVED` round too.
  This is the guard against the conditional-output-file regression that would make `shedengine.Done` unreachable.
  Judge call at round 3: with reports for rounds 1 through 3 and ledgers for rounds 1 and 2 on disk, assert the round judged is 3, that the prompt's `report_path` names round 3's report, and that `previous_ledger` names `round-2-bouncer-ledger.md` rather than round 1's.
  Previous-ledger handling: a valid prior ledger's absolute path reaches the prompt; a *malformed* prior ledger degrades to the `(none)` literal with a warning logged and the judge still runs; the no-prior-ledger case fills `(none)` rather than an empty string.
  Assert on the recorded `Spec.Prompt` for each.
  For each assertion on a returned pointer, `os.Stat` the reported path rather than comparing strings — the rule is about the file existing, and `Shed` will never check it.
- **Commit:** `test(shedadapters): cover the Bouncer's judge call and prompt inputs`

### Card 10: degradations, debris, harvest, and stale outputs

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedadapters/archive_test.go`
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedadapters/bouncer_judge_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend `internal/shedadapters/bouncer_judge_test.go`.

  Judge-call degradations, enumerated individually rather than as one case: an unreadable report file; an empty report file; a whitespace-only report file; the judge template unreadable; the rubric stencil unreadable; a fill failure; a `Run` error; each non-completion shuttle outcome that harvest cannot rescue; an unreadable verdict file; an unparseable verdict file; an unparseable ledger file.
  Each asserts `shedengine.Stuck`, a nil error, an empty pointer, and a warning logged.
  Assert in every one of these cases that the outcome is not `shedengine.Done` — that is the single property that must never regress.

  Harvest: the fake writes the verdict and the ledger but not the focus file, and reports a non-completion outcome.
  `Call` still acts on the recorded verdict — `shedengine.Done` on `APPROVED`, `shedengine.Stuck` on `BLOCKING` — rather than degrading, and it synthesizes `round-<N+1>-focus.md` on the `BLOCKING` branch.
  Contrast case: the fake writes nothing and reports a non-completion outcome, which degrades to `shedengine.Stuck` with an empty pointer.

  `judged(N)` is not satisfied by debris — four cases, each of which must produce a judge call with the fake invoked rather than a replay: a verdict present with the ledger absent; a ledger present with the verdict absent; both present with the verdict unparseable; both present with the *ledger* unparseable.
  Assert in each that the debris was archived on the way in, by checking that a timestamped sibling of the debris path exists and that the debris path itself was recreated by the spawn.

  Stale outputs: a pre-existing verdict, ledger, and focus file at the three target paths are all archived before the spawn, and the spawn then succeeds rather than tripping the shuttle's own pre-existing-output rejection.
  Assert each archived sibling exists and that its content matches what stood there before.
- **Commit:** `test(shedadapters): cover the Bouncer's degradations, debris handling, and harvest`

### Card 11: replay, focus synthesis, pointer discipline, and cancellation

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedadapters/archive_test.go`
  - `internal/shedadapters/bouncer_judge_test.go`
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/bouncer_replay_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/shedadapters/bouncer_replay_test.go`, reusing the same fakes and helpers.

  Replay, `APPROVED`: a round whose verdict and ledger both exist and parse, with a recorded `APPROVED` verdict, returns `shedengine.Done` with the ledger as the pointer, the fake was never called, and no warning was logged.
  Name this test for what it guards: without replay this disk state bounces forever and `shedengine.Done` is unreachable.
  Replay, `BLOCKING`: the same shape with a recorded `BLOCKING` verdict returns `shedengine.Stuck` with the ledger as the pointer, a warning logged, and the fake never called; `round-<N+1>-focus.md` is present afterwards, synthesized when it was absent.
  Assert the existing verdict and ledger files are left untouched — neither archived nor rewritten — by comparing their bytes before and after.
  `judged(N)` ignores the focus file: verdict and ledger both present and parsing with `round-<N+1>-focus.md` absent is a replay, not a re-judge; assert the fake was never called.

  Focus synthesis over an unparseable file: a malformed `round-<N+1>-focus.md` standing where synthesis must write.
  Assert the malformed content survives under an archive name and that the synthetic file at the original path parses with `round: N+1` and both lists empty.
  Add the same assertion for the seed path over a malformed `round-1-focus.md`.
  This is the guard against the writer quietly overwriting the evidence of whatever malformed the judge's output.

  Pointer discipline: one case per row of the discussion's pointer table.
  `shedengine.Done` and a `BLOCKING` `shedengine.Stuck`, reached from both a judge call and a replay, name a ledger that `os.Stat` finds on disk.
  The seed call, the re-bounce, every degraded judge path, and every error return report an empty pointer.
  Use `os.Stat` on the reported path for each ledger-naming case rather than comparing strings.

  Cancellation: an already-cancelled context at entry returns a non-nil error with nothing started, asserted by the fake never being called.
  A context cancelled during the run — arranged through `fakeShuttle`'s `duringRun` hook — returns a non-nil error on every path *except* a genuinely parsed verdict, which returns its mapped outcome and its pointer regardless.
  Cover both the `APPROVED` and the `BLOCKING` verdict for that exception, and cover a seed call and a degraded judge path for the ordinary rule.
- **Commit:** `test(shedadapters): cover the Bouncer's replay, focus synthesis, pointer rule, and cancellation`

## Batch Tests

`verify: go test ./internal/shedadapters/...` runs the whole package, now including all four Bouncer `Call` test files — `bouncer_config_test.go` and `bouncer_seed_test.go` from batch 3, and `bouncer_judge_test.go` and `bouncer_replay_test.go` from this one.
Package-wide scope is correct rather than over-broad for the same reason batches 1 and 3 give: Go's test unit is the package, and this package's suite is fast, fake-driven, and filesystem-only.
The cases here are the ones that pin the design's two non-obvious mechanisms — harvest and replay — plus the property that must never regress: no degraded path ever returns `shedengine.Done`.
Every `Call` case asserts on the recorded `shuttleengine.Spec` where a spawn happened and on whether the fake was called at all where one must not have, so a mode misclassification fails loudly rather than passing on a coincidentally-correct outcome.
