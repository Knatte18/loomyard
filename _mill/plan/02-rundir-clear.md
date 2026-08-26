# Batch: rundir-clear

```yaml
task: "Fix Bouncer anchor-path and run-dir clearing"
batch: "rundir-clear"
number: 2
cards: 5
verify: go test ./internal/shedadapters/... ./internal/loomrecipe/...
depends-on: [1]
```

## Batch Scope

This batch closes defect 2: a `Bouncer` re-entered after it has already settled a segment re-resolves the highest round, finds `judged(n)` still true, and replays the already-settled verdict — so an APPROVED replay returns `Done` immediately and a rewritten artifact passes the gate unjudged.
The fix is a clear-and-re-seed step at `Call` entry, triggered by the already-APPROVED verdict the Bouncer itself wrote for the resolved round: archive the run directory aside, recreate it empty, and fall through with the round re-resolved to 0, which makes the same call a seed call.
Card 8 adds the archive helper, card 9 makes the parsed verdict available at `Call` entry without a second read, card 10 wires the step in and repairs every claim and test the change falsifies, card 11 rewrites the two package-level docs that restate the falsified claims from other angles, and card 12 rewrites the recipe comment that justified `Plan-Revalidate`'s routing with the now-fixed live-lock.

The whole batch lives in `internal/shedadapters` except card 12's comment-only edit to `contracts/recipes/loom-recipe.yaml`, which is the file this batch shares with batch 1 and the reason for the `depends-on: [1]` edge.

Batch-local decisions, on top of `## Shared Decisions`:

- The archive helper goes in `internal/shedadapters/archive.go`, beside the `archiveTimestampFormat` constant and the `firstFreeArchivePath` collision helper it composes with, rather than in `round.go` — `round.go` owns turning a *round number* into a path, and the run directory is not round-numbered.
- `shedengine`'s episode and budget semantics are not touched, so a re-entered segment runs generation 2 on the Burler row's leftover budget.
  That asymmetry is documented in cards 10 and 11 rather than compensated for in code, per the `second-generation-runs-on-the-burlers-leftover-budget` Decision in `_mill/discussion.md`.

## Cards

### Card 8: `archiveRunDir`, the rename-aside-and-recreate helper

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/round.go`
- **Edits:**
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/archive_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported `archiveRunDir(runDir string, now func() time.Time) error` to `internal/shedadapters/archive.go`.
  It renames `runDir` to a timestamped sibling beside it and then recreates `runDir` as an empty directory with `os.MkdirAll` and mode `0o755`.
  The archive target is chosen through the existing `firstFreeArchivePath` helper with a candidate closure of the form `filepath.Join(filepath.Dir(runDir), base + "-" + stamp + suffix)`, where `base` is `filepath.Base(runDir)` and `stamp` is `now().UTC().Format(archiveTimestampFormat)` — the same constant `archiveStaleOutputs` uses.
  A directory has no extension to preserve, so unlike `archiveStaleOutputs` the candidate closure appends nothing after the suffix.
  Every failure — the `firstFreeArchivePath` probe, the `os.Rename`, and the `os.MkdirAll` — is returned as a wrapped error naming `runDir`, in the `shedadapters: <what failed> <path>: %w` shape the file's existing errors already use.
  The recreate is not optional and must not be skipped on any path: `ResolveRound` hard-errors when the run directory is absent, so leaving it unrecreated would turn the next `Call` into a hard error rather than a seed.
  Do not change `archiveStaleOutputs`, `firstFreeArchivePath`, or `archiveTimestampFormat`.
  In `internal/shedadapters/archive_test.go`, add tests covering: the rename moves every entry of a populated directory to the sibling, the original path exists again and is empty afterwards, the sibling's name is `<base>-<stamp>` under an injected fixed clock, a second call in the same second lands on the `-1` suffix via `firstFreeArchivePath`, and a rename failure is returned as a non-nil error.
  Build each test's `runDir` as a subdirectory the test creates inside `t.TempDir()` rather than as `t.TempDir()` itself, so the archived sibling lands inside the temp tree and is cleaned up with it.
  Write the tests first, watch them fail against the absent helper, then implement.
- **Commit:** `feat(shedadapters): add archiveRunDir, the run-directory archive helper`

### Card 9: expose the parsed verdict at `Call` entry without a second read

- **Context:**
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/bouncer_replay_test.go`
- **Edits:**
  - `internal/shedadapters/bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Split `(*Bouncer).judged` in `internal/shedadapters/bouncer.go` into a value-returning sibling plus a thin boolean wrapper, so `Call` can branch on the parsed verdict without re-reading the file.
  Add `judgedVerdict(round int) (bouncerVerdict, bool)`, carrying `judged`'s current body unchanged in every respect — it reads and parses round's verdict file, then reads and parses round's ledger file, returns false on any read or parse failure, and still deliberately excludes the focus file — but returning the verdict `parseVerdict` produced alongside the boolean.
  Reduce `judged(round int) bool` to `_, ok := b.judgedVerdict(round); return ok`, keeping its existing doc comment's substance (both files must exist and parse; the focus file is deliberately excluded because it is an input to the next round rather than evidence about this one, and including it would let a missing focus file invalidate a judgment that provably happened).
  Give `judgedVerdict` a doc comment stating what the extra return value is for: `Call`'s clear trigger needs the parsed verdict at entry, and `judged`'s discarding of it is what forced `settle` to re-read the file.
  This card changes no behavior: `Call`'s `if b.judged(n)` and `judgeCall`'s harvest `if b.judged(n)` both keep calling the wrapper, and `settle` keeps its own defensive re-read, which preserves its documented "vanished between judged and settle" degrade path and is the smaller change.
  `TestBouncer_Judged_IgnoresFocusFile` in `internal/shedadapters/bouncer_replay_test.go` must keep passing unchanged, which is the check that this split is behavior-preserving.
- **Commit:** `refactor(shedadapters): judgedVerdict returns the parsed verdict beside its bool`

### Card 10: clear an already-approved run directory at `Call` entry

- **Context:**
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/round.go`
  - `internal/shedadapters/bouncerfiles.go`
  - `internal/shedadapters/focus.go`
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/bouncer_seed_test.go`
  - `internal/shedadapters/bouncer_judge_test.go`
  - `internal/shedengine/run.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncer_replay_test.go`
  - `internal/shedadapters/bouncer_commit_test.go`
- **Creates:**
  - `internal/shedadapters/bouncer_clear_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Insert a clear-and-re-seed step into `(*Bouncer).Call` between the `ResolveRound` call and the existing four-mode branch, so the branch sees a fresh directory and re-resolves to the seed path.
  When `n > 0` and `b.judgedVerdict(n)` reports true with a verdict equal to `verdictApproved`, call `archiveRunDir(b.cfg.RunDir, b.cfg.Now)` and then set the local round to 0, falling through into the existing branch.
  A non-nil `archiveRunDir` error returns `b.degrade(ctx, ...)` — `Stuck` with an empty pointer and a nil error — with a warning message naming the producer, the engine label, the round, and the cause, matching `seedCall`'s stale-focus-archive degrade beside it.
  Do not swallow the failure and continue: continuing would replay the stale verdict, which is the defect.
  After the clear, `round1FocusSeeded()` is false over the recreated empty directory, so the same call takes `seedCall` — this is the intended path and no extra branch is needed to force it.
  The harvest path stays untouched: `judgeCall` calls `settle(n, spawned: true)` within the same `Call` that produced the verdict, so an approval still returns `Done` normally on the call that earns it, and the trigger cannot fire on it because the clear runs before the spawn, not after.

  Rewrite the four doc/comment sites in this file the change falsifies, per `_mill/discussion.md`'s defect-2 inventory.
  `NewBouncer`'s budget-rule paragraph claims the Bouncer's episode "never resets"; that is now false, because `shedengine.episodeStuckCount` returns at the first history entry whose producer is this row with a `Done` outcome, so a re-entered segment's Bouncer episode does reset — say "within one generation" instead, and point at the two-row consequence (the `BurlerProducer` row's episode does not reset, so a second generation runs on the Burler's leftover budget) rather than claiming a fresh segment budget.
  `Call`'s own doc says the call branches "into one of four modes -- seed, re-bounce, judge, or replay"; there is now a fifth entry action ahead of the branch, and `Done` is reachable through harvest only, since the replay path yields `Stuck` or the clear and never `Done`.
  Keep `Call`'s pointer rule, which is unchanged, and restate it accurately against the narrowed set of `Done`-reaching paths.
  `settle`'s doc justifies routing a `Commit` error as `settle`'s own error rather than through `degrade` by saying `judged(n)` stays true on re-entry; that trailing clause is the defect being removed.
  The decision it justifies stays correct and must keep a stated reason — the reason becomes that `degrade` only ever returns `Stuck`, which would silently convert an approval into a rejection.
  The inline comment on `settle`'s BLOCKING arm reading "an APPROVED replay is not a warning condition" is rewritten or deleted, because an APPROVED replay no longer reaches `settle` at all; the surviving half, why a BLOCKING replay *is* warned about, stays.

  Create `internal/shedadapters/bouncer_clear_test.go` covering, using the existing `newTestBouncer`, `testBouncerConfig`, `layoutBouncerRun`, `bouncerReport`, `bouncerVerdictContent`, `bouncerLedgerContent`, `judgeFakeShuttle`, `fixedClock`, and `captureBouncerWarnings` helpers:
  a `Call` over a run directory whose highest round is judged APPROVED renames the directory aside, recreates it empty, and takes the seed path in the same call — asserting the archived sibling exists and carries the old artifacts, that the fresh directory contains only what `seedCall` writes, and that the outcome is the seed `Stuck` with an empty pointer;
  the full generation is moved, not just the Bouncer's files, so the archived sibling carries `round-<N>-review.md` and `round-<N>-fixer-report.md` as well as the verdict, ledger, and focus files;
  archive naming under an injected fixed clock produces `<runDir>-<stamp>`, and a second clear in the same second lands on the `-1` suffix;
  each non-triggering case leaves the run directory untouched and its existing outcome unchanged — an in-segment BLOCKING replay, a mid-segment resume with an unjudged round N, a re-bounce with round-1 focus seeded and no report, and a round N with a verdict but no parsable ledger;
  the harvest path is unaffected, so a `judgeCall` whose spawn produces an APPROVED verdict and ledger returns `Done` with that round's ledger pointer in the same call and does not clear;
  a rename failure and a recreate failure each degrade to `Stuck` with an empty pointer and a nil error;
  a `Bouncer` constructed fresh over a run directory a previous process left APPROVED clears and re-seeds on its first `Call`, which is the cross-invocation case and is asserted as intended rather than left implicit;
  a `Commit` that returns an error surfaces that error from `settle` unchanged, and a subsequent `Call` over the same still-APPROVED directory then clears and re-seeds — assert both halves in one test so the sequence is the subject;
  and an end-to-end sequence within the package of seed, judge BLOCKING, judge APPROVED with `Done`, re-enter, whose next `Call` is a seed call writing `round-1-focus.md` into a fresh directory with the prior generation preserved beside it.
  Because `testBouncerConfig` sets `RunDir` to `t.TempDir()` itself, give this file's fixtures a run directory nested one level inside `t.TempDir()` so each archived sibling is cleaned up with the temp tree.

  Repair the two existing test files the change falsifies.
  In `internal/shedadapters/bouncer_replay_test.go`, `TestBouncer_Replay_Approved` asserts the removed behaviour outright and must be rewritten to assert the new one (the same disk state now clears and seeds), and `TestBouncer_PointerDiscipline`'s `Replay_Approved` subtest must be re-pointed the same way or replaced by a `Harvest_Approved` subtest that still proves a `Done` pointer names a file that exists.
  `TestBouncer_Replay_Blocking`, `TestBouncer_Cancellation_DuringRun_ParsedVerdictSurvives`, and the remaining pointer-discipline subtests reach `settle` through the judge path or a BLOCKING verdict and are unaffected.
  In `internal/shedadapters/bouncer_commit_test.go`, `TestBouncer_Commit_ApprovedCallsExactlyOnce`, `TestBouncer_Commit_NilIsNotAnError`, and `TestBouncer_Commit_FailingCommitIsAnError` each reach `settle` via the APPROVED-replay vehicle, which no longer exists; convert each to the harvest vehicle by laying out the round's report only and driving the shuttle with `judgeFakeShuttle`, which writes the verdict and ledger during the run so `judgeCall` harvests and settles in the same call.
  Update that file's own header comment, which names the replay path as every case's vehicle.
  `TestBouncer_Commit_BlockingNeverCalls` uses a BLOCKING verdict and is unaffected;
  `TestBouncer_Commit_CancelledContextStillCommits` calls `b.settle` directly and is likewise unaffected.
  Write the new tests and the test repairs first, watch them fail against the unchanged producer, then implement.
- **Commit:** `fix(shedadapters): Bouncer clears an already-approved run directory on re-entry`

### Card 11: rewrite the two package docs that restate the falsified claims

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/shedadapters/doc.go`
  - `internal/shedadapters/burler.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two doc-only rewrites, both from `_mill/discussion.md`'s defect-2 inventory, restating card 10's claims from the angles those two files own.
  In `internal/shedadapters/doc.go`'s "Outcome mapping" section, the `Bouncer` bullet claims `Call` "resolves into one of four modes -- seed, re-bounce, judge, or replay" and that "A parsed APPROVED verdict maps to `Done`".
  Rewrite both: there is a fifth entry action ahead of the branch, and an APPROVED verdict maps to `Done` only on the harvest that earns it — at `Call` entry it now maps to the clear.
  The exists-or-empty pointer rule and the ledger-is-reported rationale in the same paragraph are unaffected and must survive the rewrite intact.
  Also state the segment re-entry rule in the "The round-artifact convention" section, which is that contract's durable home and the reason `_mill/discussion.md`'s `no-new-constraints-invariant` Decision adds nothing to `CONSTRAINTS.md`: a gate that has already approved and is entered again archives its generation and re-judges rather than replaying, and both rows' artifacts move together because `BurlerProducer` would otherwise resume at round N+1 with hydration from a dead generation.
  Do not touch the stale `round-<N>-focus.json` spelling in the same section — the code writes `round-<N>-focus.md`, but that drift is pre-existing, unrelated, and explicitly Out of scope.
  In `internal/shedadapters/burler.go`, the `BurlerProducer` doc's two-row cap paragraph rests on a symmetry that card 10 breaks.
  This row's own "never resets" stays true, because `BurlerProducer.Call` returns `Stuck` on every successful round and `Done` never;
  what breaks is the claim that the Bouncer row has the same unresetting property, and therefore the conclusion that "the Bouncer's normally binds".
  Rewrite it to say the Bouncer's budget binds in a segment's first generation, and that in any later generation the Burler's leftover budget binds, because the Bouncer's episode resets at its own `Done` and this row's never does.
  Record the residual limitation there rather than leaving it to be rediscovered: with equal budgets, a first generation that approved on round *k* leaves this row `max_bounces - k` units, and one that approved on the last round leaves none, so the segment's first Burler hand-off in generation 2 halts the run with a bounce-budget-exhausted escalation to a human — accepted because it fails safe, and because compensating for it means changing `shedengine`'s shared budget model.
  Change no code in either file: `BurlerProducer`, its constructor, and its `Call` are untouched, and `doc.go` has no code at all.
- **Commit:** `docs(shedadapters): record segment re-entry and the second-generation budget`

### Card 12: restate `Plan-Revalidate`'s routing rationale without the fixed live-lock

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomrecipe/revalidate_test.go`
  - `_mill/discussion.md`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** One comment-only edit in `contracts/recipes/loom-recipe.yaml`, on the `Plan-Revalidate` row.
  Its `on_stuck` comment currently justifies pointing at `Plan-Write` rather than `Plan-Bouncer` on the grounds that bouncing back into the segment live-locks, because `judged(n)` is still true for the already-APPROVED round and `settle` returns `Done` immediately.
  That live-lock is what this task removes, so the stated reason rots even though the routing is unchanged.
  `on_stuck` stays `Plan-Write`;
  only the comment changes.
  The replacement reason is the one the `plan-revalidate-on-stuck-stays-plan-write` Decision in `_mill/discussion.md` states: `Plan-Revalidate` reports mechanical format findings, and the fixer round is rubric-forbidden from re-deriving those, so the row that can actually repair them is the writer.
  State the cost the change now carries, because removing the live-lock is not free: each `Plan-Revalidate` `Stuck` to `Plan-Write` to `Plan-Validate` to `Plan-Bouncer` pass now runs a complete review generation where it previously replayed a settled verdict instantly, bounded by `Plan-Revalidate`'s own bounce budget and, from the second generation on, by the Burler row's leftover budget.
  Keep the surviving sentences that `Plan-Write` is the same target `Plan-Validate` already bounces to, that it terminates, and that the bounce budget bounds it.
  Change no row name, no `engine`, no `on_stuck`, no `on_done`, and no config value.
  Run `internal/loomrecipe`'s coverage guard, shape, and revalidate tests afterwards to confirm the comment edit disturbed neither the row names nor the parsed shape.
- **Commit:** `docs(loom-recipe): Plan-Revalidate on_stuck reason no longer cites the live-lock`

## Batch Tests

`verify:` runs `go test` over `internal/shedadapters` — where every card but card 12 lands, and which carries the new `bouncer_clear_test.go` plus the repaired `bouncer_replay_test.go` and `bouncer_commit_test.go` — and `internal/loomrecipe`, whose coverage-guard, shape, and revalidate tests are what prove card 12's comment edit disturbed neither the recipe's row names nor its parsed shape.

The scope is per-batch rather than repo-wide: no package outside these two is edited by any card here, and `internal/shedadapters` has no production importers whose behaviour changes beyond what `internal/loomrecipe` already exercises.
Cross-package regressions are caught by `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), which mill-go runs from the git root before marking the task done.

Card 10's test set is the substance of the batch, and its non-triggering cases carry as much weight as its triggering ones: the trigger's whole justification is that its fire set is exactly a marker's would be, so each of the four non-fire cases — in-segment BLOCKING bouncing, a mid-segment resume with an unjudged round, a re-bounce with focus seeded and no report, and a verdict with no parsable ledger — is asserted to leave the run directory untouched.
Three fire cases beyond the in-run re-entry are asserted rather than left implicit, because two are intended and one is an accepted regression: a fresh `Bouncer` over a directory a previous process left APPROVED, a later run reaching the segment from the top, and a resume after a `Commit` failure.

`_mill/discussion.md` explicitly declines a recipe-level LLM-driven or fake-shuttle end-to-end run: every branch is reachable at package level, and a recipe-level run would duplicate the coverage at far higher cost.
