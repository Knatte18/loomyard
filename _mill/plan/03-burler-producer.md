# Batch: BurlerProducer

```yaml
task: 'shedadapters: Burler-round producer'
batch: 'BurlerProducer'
number: 3
cards: 5
verify: go test ./internal/shedadapters/
depends-on: [1, 2]
```

## Batch Scope

This batch delivers the task's centrepiece: `BurlerProducer`, a reusable `shedengine.ShedProducer` in `internal/shedadapters` that wraps `internal/burlerengine`'s one-round A-review/B-fix API as a single `Shed` row.
It ships the narrow `BurlerRunner` seam with its compile-time assertion, the constructor and its told-path validation, from-disk round resolution and prior-round hydration over the pair predicate, the per-round artifact path convention, the one-retry-on-`died`/`timeout` slice, the archive-on-every-non-usable-review exit rule, the cancellation contract, and the fake-runner unit suite that drives all of it deterministically.
It is one batch because every one of those pieces is a rule about the same `Call` method and the same two artifact paths — splitting them would leave a batch that cannot be verified on its own.
It depends on batch 1 for `burlerengine.Profile.ClusterExclude` and on batch 2 for `RoundFocus`, `readRoundFocus`, and `burlerEngineLabel`.
The external interface batch 4 documents is `NewBurlerProducer`, its `Stuck`-only outcome mapping, and the two-sided pair predicate.
Batch-local decision differing from `## Shared Decisions`: nothing.

## Cards

### Card 6: `BurlerRunner` seam, `BurlerProducer`, and its constructor

- **Context:**
  - `internal/shedadapters/perch.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/focus.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/run.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/burler.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedadapters/burler.go` with a file-header comment naming what the file implements, then the following declarations.
  An exported interface `BurlerRunner` with the single method `Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error)`, followed by `var _ BurlerRunner = (*burlerengine.Engine)(nil)` as a compile-time proof, mirroring `PerchRunner` and its assertion in `internal/shedadapters/perch.go`.
  An exported struct `BurlerProducer` with unexported fields `name string`, `runner BurlerRunner`, `profile burlerengine.Profile`, `opts burlerengine.RunOpts`, `runDir string`, and `now func() time.Time`, followed by `var _ shedengine.ShedProducer = (*BurlerProducer)(nil)`.
  An exported constructor `NewBurlerProducer(name string, runner BurlerRunner, profile burlerengine.Profile, opts burlerengine.RunOpts, runDir string, now func() time.Time) (*BurlerProducer, error)` returning a distinct error for each of: a nil `runner`, an empty `name`, an empty `runDir`, and a `runDir` that is not absolute per `filepath.IsAbs`.
  A nil `now` defaults to `time.Now`, exactly as `NewSingleLLMProducer` does, and the injected clock resolves only the archive filename's same-second collision suffix.
  The constructor must not stat, create, or otherwise touch `runDir` — creating it is `Call`'s job.
  Godoc on `BurlerProducer` must state, in this order: that `Call` returns `shedengine.Stuck` on every successful round and never `shedengine.Done`, and that this `Stuck` is a routine hand-off signal to the segment's `Bouncer` via `OnStuck`, never a real stuck condition, so an operator reading a status file is never misled; that a round which did not reach `shuttleengine.OutcomeDone` after the bounded retry is a hard error rather than `Stuck`, because the `Bouncer` tells its seed call from its judge call by the round artifacts on disk and a failed round returning `Stuck` with no review written would be misread as a seed call; and that, because the producer never returns `Done`, its `Shed` bounce episode never resets, so its `effectiveMaxBounces` stops being a bounce-loop guard and becomes a cap on review rounds.
  That last paragraph must state the coupling as a two-row relationship and must never advise raising this row's own `MaxBounces`: the segment's `Bouncer` row has the same unresetting property and is the segment's entry point, so its `Stuck` sequence runs one ahead of this producer's round count and with equal budgets it exhausts first — the segment's round cap is therefore the smaller of the two rows' budgets, the `Bouncer`'s normally binds, and raising the cap means raising both rows together.
  Godoc on the constructor must state that `profile` is a template whose `ReviewPath`, `FixerReportPath`, `PriorReviews`, `PriorFixerReports`, and `ClusterExclude` fields are overwritten per round, and that `opts` is a template whose `Round` field is overwritten per attempt.
  Declare no second copy of `burlerEngineLabel` — batch 2 declared it in `internal/shedadapters/focus.go`.
- **Commit:** `feat(shedadapters): add the BurlerRunner seam and BurlerProducer constructor`

### Card 7: Round resolution and prior-round hydration

- **Context:**
  - `internal/shedadapters/perch.go`
  - `internal/shedadapters/archive.go`
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `internal/shedadapters/burler.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the round-artifact path convention and the from-disk scan to `internal/shedadapters/burler.go`.
  Two unexported path helpers, `roundReviewPath(runDir string, n int) string` and `roundFixerReportPath(runDir string, n int) string`, joining `runDir` with `round-<n>-review.md` and `round-<n>-fixer-report.md` respectively, where `n` is rendered as a plain positive decimal integer with no leading zeros and no attempt suffix.
  An unexported `roundComplete(runDir string, n int) bool` reporting whether both of those paths exist.
  An unexported `highestCompleteRound(runDir string) (int, error)` that reads `runDir`'s directory entries and returns the highest `n` for which `roundComplete` holds, returning `0` when the directory is absent — which is not an error — and returning any other `os.ReadDir` failure wrapped.
  Its name-matching discipline mirrors `highestRunAttempt` in `internal/shedadapters/perch.go` and must be exact-shape: skip directory entries, require the literal prefix `round-` and the literal suffix `-review.md`, require the middle to be a non-empty run of decimal digits with no leading zero, and require the parsed integer to be at least 1.
  A name that does not match exactly — a stamped archive sibling such as `round-2-review-20260820T101500Z.md`, an attempt-suffixed token such as `round-3b-review.md`, a zero-padded token, or any unrelated file — is ignored: never adopted, never deleted.
  An unexported `hydrationPaths(runDir string, current int) (reviews []string, fixerReports []string, err error)` returning, for every `n` from 1 up to but excluding `current` for which `roundComplete` holds, the review path and the fixer-report path in ascending round order, using the same predicate so hydration can never name a fixer report that does not exist.
  Godoc on `highestCompleteRound` must state that the completion predicate is the pair, never the review alone, and why: a producer process killed in the phase-A-written/phase-B-pending window leaves a review with no fixer report beside it and no exit path ever ran to clean it up, so under a review-only predicate the next call would advance and hydrate a fixer report that `burlerengine`'s `requireExistingPaths` rejects fail-loud, wedging the segment permanently, whereas under the pair predicate the orphan simply means the round is incomplete and is re-run.
  It must also state that the same pair predicate is what the segment's `Bouncer` uses to tell its seed call from its judge call, so the two sides run the same test.
- **Commit:** `feat(shedadapters): resolve burler rounds and hydration from disk`

### Card 8: `BurlerProducer.Call`

- **Context:**
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/focus.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/perch.go`
  - `internal/shedengine/producer.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/verdict.go`
  - `internal/shuttleengine/engine.go`
- **Edits:**
  - `internal/shedadapters/burler.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement `func (p *BurlerProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` in `internal/shedadapters/burler.go`.
  Sequence: consult `entryErr(ctx, p.name, burlerEngineLabel)` and return its error without touching anything; create the run directory with `os.MkdirAll(p.runDir, 0o755)` so a fresh clone's first call works; resolve `round` as `highestCompleteRound(p.runDir) + 1`; derive this round's `reviewPath` and `fixerReportPath` from the card-7 helpers; read the round's directive with `readRoundFocus(p.name, p.runDir, round)`; and build a fresh copy of the template `profile` for this round rather than reusing a resolved one, because `burlerengine.Profile.validate` mutates the profile in place and `Engine.Run` takes it by value.
  On the fresh copy set `ReviewPath` and `FixerReportPath` to the two derived paths; set `PriorReviews` to the template's own told entries as a fixed prefix, followed by the derived review paths from `hydrationPaths`, followed by the focus file's surviving `Hydrate` entries; set `PriorFixerReports` to the template's own told entries followed by the derived fixer-report paths.
  Set `ClusterExclude` to the focus file's `ExcludeLenses` only when the template profile's `ClusterFan` is non-empty; when `ClusterFan` is empty and `ExcludeLenses` is non-empty, drop the directive with a `logger.Warn` and leave `ClusterExclude` nil, so a well-formed but unusable directive never becomes a `validate` hard error downstream.
  Run at most two attempts.
  Before **every** attempt, including attempt 1, call `archiveStaleOutputs` over `reviewPath` and `fixerReportPath` with `p.now`, returning a wrapped error if it fails, so a leftover file at the round's own paths is renamed to a stamped sibling rather than being passed through to a run whose spec validation rejects a pre-existing output file.
  Each attempt copies `p.opts` and sets `Round` to the round rendered as a decimal integer for attempt 1 and that same integer with the literal suffix `b` for attempt 2 — the attempt-distinguishing token belongs on `RunOpts.Round`, which names the shuttle run, and never on the artifact paths, which stay canonical per round.
  Outcome handling, with `cancelErr(ctx, p.name, burlerEngineLabel)` consulted first on every non-success exit exactly as the other adapters do: a runner error is wrapped and returned, naming the round and the attempt; `shuttleengine.OutcomeAsking` is a hard error on its first occurrence, never retried, with `Result.LastAssistantMessage` in the error text; `shuttleengine.OutcomeDied` and `shuttleengine.OutcomeTimeout` on attempt 1 emit a `logger.Warn` naming the outcome and `Result.SessionID` and then retry, and on attempt 2 are a hard error naming both attempts' session ids and kept run dirs; any other outcome value is a hard error naming it.
  Before starting attempt 2, re-check `ctx.Err()` and return the `cancelErr` result instead of spawning a fresh, expensive round.
  On `shuttleengine.OutcomeDone` with a nil runner error, return `shedengine.Stuck` with `shedengine.OutputPointer{Path: reviewPath}` and a nil error for both `burlerengine.VerdictApproved` and `burlerengine.VerdictBlocking` — never `shedengine.Done` — unless the context is cancelled, in which case return the `cancelErr` error, because `internal/shedengine/producer.go` binds every implementation to surface cancellation as a non-nil error and never as `Stuck`.
  Archive rule on exits, keyed on whether the round produced a usable review rather than on whether the return is an error: every return in which the round did not produce a usable review must archive both round paths first — that covers a runner error whose `Result.Outcome` is `shuttleengine.OutcomeDone` (the cluster-audit and verdict-parse cases, reached only after every output file already exists on disk), a runner error with any other outcome, an `asking` hard error, a second consecutive `died`/`timeout`, an unrecognized outcome, and a cancellation detected between attempts.
  Exactly two returns leave the round's files in place: the success return, and a cancellation detected after the round already completed and parsed — that return is an error, but its artifacts are intact and must survive, so the next call advances to the following round instead of re-running this one.
  An archive failure on an error exit must be logged with `logger.Warn` and must not replace or mask the error being returned.
  Add a doc comment on `Call` stating the archive rule's keying, the two carve-outs, and that no mid-run cancellation bridge is installed because `internal/burlerengine` exposes no pause seam, so a cancel is observed only once the round reaches a terminal outcome or its own `RunOpts.Timeout` elapses.
  Construct no `_lyx` or `.lyx` literal anywhere in this file, call no `lyxcwd`, `os.Getwd`, or git, and add no import of `internal/treadleengine`.
- **Commit:** `feat(shedadapters): implement BurlerProducer.Call`

### Card 9: Fake runner, constructor, round-scan, and hydration tests

- **Context:**
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/perch_test.go`
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedadapters/archive_test.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/burler_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedadapters/burler_test.go` with a `fakeBurlerRunner` in the same style as `fakePerchRunner` in `internal/shedadapters/perch_test.go`: it records every `burlerengine.Profile` and `burlerengine.RunOpts` it is handed, counts its invocations, returns caller-scripted `burlerengine.Result`/`error` values per invocation so a test can script attempt 1 and attempt 2 differently, exposes a `duringRun` hook a test uses to cancel the context or write files as if mid-round, and satisfies `BurlerRunner`.
  Then cover the constructor and the from-disk resolution, using `t.TempDir()` for every run directory.
  Constructor cases: a nil runner, an empty name, an empty run directory, and a relative run directory each return an error, and a valid call returns a usable producer with no error.
  Round-scan cases, asserted through the round token the fake runner is handed and through the review path on the profile it is handed: an absent run directory is created and starts at round 1; an empty run directory starts at round 1; a complete round `N` with both files present advances to `N+1`; a round whose review file is absent but whose fixer report is present is re-run at the same `N`; a review-only orphan at `N` counts as incomplete, so `N` is re-run and the orphan is archived aside rather than deleted or passed through; a run directory holding unrelated files is unaffected; a stamped archive sibling such as `round-2-review-20260820T101500Z.md` does not satisfy the scan and never shifts the resolved round; and non-numeric, zero-padded, and attempt-suffixed tokens such as `round-3b-review.md` are ignored rather than adopted.
  Hydration cases: round 1 hydrates nothing beyond whatever the template profile already told; with rounds 1 and 2 complete, round 3's profile carries both prior reviews and both prior fixer reports in ascending round order; a stamped archive sibling is never hydrated; a round with only one of its two files is skipped by hydration entirely, so the missing fixer report is never named; told `PriorReviews`/`PriorFixerReports` entries on the template profile are preserved as a prefix ahead of the derived entries; and a focus file's surviving `hydrate` entries are appended to `PriorReviews`.
  Assert a pre-existing file at the round's own paths is renamed to a stamped sibling before the runner is invoked, by checking from inside the fake runner that neither round path exists at invocation time while the stamped sibling does.
  The test file stays untagged and spawns nothing.
- **Commit:** `test(shedadapters): cover BurlerProducer construction and round resolution`

### Card 10: `Call` outcome, retry, cancellation, and archive tests

- **Context:**
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/perch_test.go`
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedadapters/archive_test.go`
  - `internal/shedengine/producer.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/verdict.go`
  - `internal/shuttleengine/engine.go`
- **Edits:**
  - `internal/shedadapters/burler_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/shedadapters/burler_test.go` with the `Call` behaviour suite, driven entirely by the card-9 `fakeBurlerRunner`.
  Outcome cases: a done round returns `shedengine.Stuck`, never `shedengine.Done`, with the round's review path as the output pointer, for both `burlerengine.VerdictApproved` and `burlerengine.VerdictBlocking`; the profile handed to the runner carries the derived per-round review and fixer-report paths, the derived hydration lists, and the focus file's `exclude_lenses` in `ClusterExclude`; the `burlerengine.RunOpts` handed to the runner carries the round token; `shuttleengine.OutcomeDied` on attempt 1 followed by `shuttleengine.OutcomeDone` on attempt 2 succeeds, the runner is invoked exactly twice, the second attempt's round token carries the `b` suffix while the review path is unchanged from attempt 1, and the review written by the successful attempt survives; `shuttleengine.OutcomeTimeout` twice is a hard error whose text names both attempts' session ids; `shuttleengine.OutcomeAsking` is a hard error on the first occurrence whose text carries `Result.LastAssistantMessage`, with the runner invoked exactly once; a runner error is wrapped rather than swallowed, with the original error still matched by `errors.Is`; and a focus file naming every lens in the fan, or naming a lens the fan lacks, still reaches the runner as a profile that runs, rather than as an error.
  Cancellation cases: a context already cancelled at entry returns an error without invoking the runner; a context cancelled between attempt 1 and attempt 2 returns the cancellation error with the runner invoked exactly once; a context cancelled during a failed round yields the cancellation error and archives the round's paths; and a context cancelled during a *completed* round yields a non-nil error and never `shedengine.Stuck`, but leaves both artifacts in place, proven by a second `Call` on a fresh context resolving the following round rather than re-running the same one.
  Archive-on-exit cases, each asserted by driving two `Call`s against the fake runner and checking the second call's round token is unchanged from the first: two consecutive `shuttleengine.OutcomeDied` attempts where attempt 1 wrote only the review file (the phase-A-only partial); an `asking` hard error; a cancellation between attempts; and a runner error returned alongside a `burlerengine.Result` whose `Outcome` is `shuttleengine.OutcomeDone` with both files already written, covering the cluster-audit and verdict-parse cases.
  Use the fake runner's `duringRun` hook to write the scripted artifacts and to cancel the context at the exact point each case needs.
  Inject a fixed clock into the producer so the stamped archive filenames are deterministic, the same way `fixedClock` is used in `internal/shedadapters/archive_test.go`.
  The test file stays untagged and spawns nothing.
- **Commit:** `test(shedadapters): cover BurlerProducer.Call outcomes, retry, and archiving`

## Batch Tests

`verify: go test ./internal/shedadapters/` runs the whole `internal/shedadapters` package suite: the new `burler_test.go` and `focus_test.go` plus the existing `archive_test.go`, `ctx_test.go`, `perch_test.go`, `singlellm_test.go`, and `webster_test.go`.
Whole-package scope is correct here because Go's unit of test invocation is the package, and this batch's new code shares `ctx.go` and `archive.go` with the three existing adapters, so a regression in either helper must surface.
The suite is pure Tier 1: a fake `BurlerRunner`, `t.TempDir()` fixtures, and an injected clock, with no LLM, no git, and no process spawn.
No new opt-in real-engine smoke test is added — `internal/burlerengine`'s existing `smoke_round_test.go` and `smoke_cluster_test.go` already cover the real round and the real cluster fan, and this producer adds only path assembly, retry, and outcome mapping over that engine.
