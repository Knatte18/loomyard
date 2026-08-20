# Batch: wire-two-rows

```yaml
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
batch: 'wire-two-rows'
number: 3
cards: 9
verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
depends-on: [1, 2]
```

## Rename mechanic

_For each `Moves:` pair the implementer MUST:_

1. _Run `git mv <old> <new>` FIRST, before making any other change to the moved file._
2. _Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits)._
3. _Use a full-file `Creates:` entry only for genuinely new files that have no predecessor._
4. _Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs._

## Batch Scope

This batch makes the split real in the running orchestrator: loom's producer list grows from twelve rows to thirteen, `Loom-Preflight` is inserted at index 1 backed by a new producer over `loomengine.CheckSeed`, and `internal/loomcli` constructs row 1 from `preflightshed.NewPreflight` instead of `loomshed.NewPreflightProducer`, which this batch deletes.
It consumes both upstream batches — batch 1's `preflightshed.NewPreflight` and batch 2's `loomengine.CheckSeed` — and it is the last batch in which `loomengine.Preflight` still exists.
After it, `loomengine.Preflight`/`checkResolved`/`runCheck4` have no production caller anywhere and only `internal/loomengine`'s own Tier-2 suite and `internal/loomcli/smoke_test.go` still reference them; batch 4 deletes them and repoints those two.

Batch-local decision: `loomshed.Deps` gains **no** new field.
`New` builds row 2 internally from `deps.StatusPath` and `deps.StatusLockPath`, the way it already builds `Discussion-Validate`, `Plan-Validate` and `Batchifier` from told values. `Deps.Preflight` stays the one injected producer seam, because row 1 is the one row that spawns git and the one row a Tier-1 test needs to substitute a fake for; row 2 spawns nothing and reads one JSON file under a caller-supplied path, so it has neither justification.
This also means `internal/loomshed` does **not** import `internal/preflightshed` — its `seam_enforcement_test.go` allowlist is unchanged by this batch.

## Cards

### Card 12: repurpose the row wrapper as loom's own seed row

- **Context:**
  - `internal/loomengine/seed.go`
  - `internal/loomengine/report.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/webster.go`
  - `internal/preflightshed/preflight.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/loomshed/loomshed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/loomshed/preflight.go` -> `internal/loomshed/loompreflight.go`
- **Requirements:** This card declares the name constant and the row-2 producer together, so card 13's row insert has both in scope and no card carries a forward reference to a symbol a later card declares.
  In `internal/loomshed/loomshed.go`, add `NameLoomPreflight = "Loom-Preflight"` to the name-constant block, immediately after `NamePreflight`, so the block's order matches the producer table's order.
  Change nothing else in that file on this card.
  Then `git mv internal/loomshed/preflight.go internal/loomshed/loompreflight.go` and edit the moved file in place — one producer per file is this package's existing layout (`internal/loomshed/batchifier.go`, `internal/loomshed/planvalidate.go`, `internal/loomshed/discussionvalidate.go`, `internal/loomshed/webster.go`), and the row-1 wrapper's own replacement already landed in `internal/preflightshed` in batch 1.
  Rename the struct `preflightProducer` to `loomPreflightProducer` and replace its `cwd string` field with two fields, `statusPath string` and `statusLockPath string`.
  Replace the exported constructor `NewPreflightProducer(cwd string) shedengine.ShedProducer` with an unexported `newLoomPreflight(name, statusPath, statusLockPath string) *loomPreflightProducer`, matching this package's other internal constructors (`newBatchifier`, `newPlanValidate`) in both visibility and concrete return type — row 2 is built internally by `New`, never injected, so nothing outside the package constructs it.
  Keep the `var _ shedengine.ShedProducer = (*loomPreflightProducer)(nil)` assertion, retyped.
  In `Call`, replace `loomengine.Preflight(p.cwd)` with `loomengine.CheckSeed(p.statusPath, p.statusLockPath, NameLoomPreflight, []string{NamePreflight, NameLoomPreflight})`.
  Pass the constants directly, never `p.name` — see the `told-names-never-come-from-the-producer-name-field` Shared Decision.
  Leave the four exit paths structurally identical: `entryErr` first, a non-nil error consulting `cancelErr` then returning the error, `!report.OK` consulting `cancelErr` then returning `shedengine.Stuck`, and `shedengine.Done` otherwise.
  Rewrite the file header comment and all three doc comments for row 2: this file wires `loomengine.CheckSeed` in behind the internal constructor; the producer validates that loom's own status file is a coherent fresh seed at the told paths; the two told names are the row's own durable on-disk identity and the set of history producers a resumable blocked run may legitimately have left behind; and the outcome mapping is the whole producer, because `CheckSeed` reports a determined verdict rather than erroring on anything short of an infra failure.
  Drop the header's remark about adding the production import of `internal/loomengine` that the Told-Geometry guard allowlists — the import stays, so keep an equivalent sentence, but stop describing it as newly added.
  Removing the exported constructor breaks loomcli's wiring and this package's own resume test until cards 15 and 19 land; that is expected and is why all four cards sit in this one batch.
- **Commit:** `refactor(loomshed): repurpose the Preflight wrapper as the Loom-Preflight row`

### Card 13: the thirteenth row

- **Context:**
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/doc.go`
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/shedengine/shed.go`
- **Edits:**
  - `internal/loomshed/loomshed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Insert one row into `New`'s `producers` slice at index 1, immediately after the `NamePreflight` row: `{Name: NameLoomPreflight, Producer: newLoomPreflight(NameLoomPreflight, deps.StatusPath, deps.StatusLockPath), OnStuck: ""}`.
  The constant and the constructor both already exist — card 12 declared them.
  Update the three remaining doc blocks this file carries.
  (1) The file header comment's "12-row" becomes "13-row".
  (2) The name-constant block's opening "The twelve producer names" becomes "The thirteen producer names".
  (3) `New`'s doc comment's "12-row" becomes "13-row", and its enumeration of rows must be renumbered end to end: 1 Preflight (deps.Preflight, ""); 2 Loom-Preflight (real, ""); 3 Discussion-Write (stub, ""); 4 Discussion-Validate (real, bouncing to Discussion-Write); 5 Discussion-Review (stub, bouncing to Discussion-Write); 6 Plan-Write (stub, ""); 7 Plan-Validate (real, bouncing to Plan-Write); 8 Plan-Review (stub, bouncing to Plan-Write); 9 Batchifier (real, ""); 10 Webster (the lazy wrapper, ""); 11 Webster-Review (stub, bouncing to Webster); 12 Publish (real, ""); 13 Finalize (real, "").
  Then extend the escalate-only rationale sentence in that same doc comment, which currently names Preflight, Batchifier, Publish and Finalize: it must add Loom-Preflight, with its own reason — no row in the list produces loom's own status file, so there is nothing to bounce to and a human is the only thing that can fix an incoherent or half-finished seed.
  Also re-read `Deps.Landing`'s field comment, which says the landing passthrough backs "rows 12 (Publish) and 13 (Finalize)": that numbering is wrong today (they are rows 11 and 12) and becomes correct with this insert, so confirm it rather than editing it.
  Leave `Deps` otherwise untouched: no new field, and `Deps.Preflight` keeps its existing doc comment and nil check.
  Do not add any import to this file.
- **Commit:** `feat(loomshed): insert the Loom-Preflight row into loom's producer list`

### Card 14: stub row-count wording

- **Context:**
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomshed/stub.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update both row-count mentions in this file from twelve to thirteen: the file header comment's "12-row producer list" and `stubProducer`'s own doc comment's "backs five rows of loom's 12-row producer list".
  The count of stubbed rows stays **five** — Discussion-Write, Discussion-Review, Plan-Write, Plan-Review and Webster-Review — and the enumeration that follows it is unchanged; only the list's total moves.
- **Commit:** `docs(loomshed): move stub.go's row count to thirteen`

### Card 15: wire row 1 from the new package

- **Context:**
  - `internal/preflightshed/preflight.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomengine/config.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `wiring.go`, replace `Preflight: loomshed.NewPreflightProducer(cwd)` with `Preflight: preflightshed.NewPreflight(loomshed.NamePreflight, cwd)`, adding the `github.com/Knatte18/loomyard/internal/preflightshed` import in its sorted position.
  Telling `loomshed.NamePreflight` rather than a bare `"Preflight"` literal is the point of the told-name constructor: the durable row identity stays declared once, in the package that owns loom's producer table.
  Rewrite the three-line comment above that field, which currently explains why the adapter is used "never the bare `loomengine.Preflight` function" — after this task there is no bare function, so the comment must instead say that `loomshed.New` requires a `shedengine.ShedProducer` and `internal/preflightshed`'s general producer is what maps `preflight.Check`'s `Report` onto that contract, with the row name told from `loomshed`'s own constant.
  Leave `StatusPath`/`LockPath`/`StatusLockPath` and every other field in the assembled `loomshed.Deps` untouched — row 2 is built inside `loomshed.New` from the `StatusPath`/`StatusLockPath` already present here.
  In `cli.go`, update the `cwd` field's comment, which names `loomshed.NewPreflightProducer` as its reader: it becomes `preflightshed.NewPreflight`, keeping the rest of the sentence ("Preflight is the one row that spawns git, over this exact cwd") intact.
  Do not remove the `internal/loomengine` import from `wiring.go` — the three status-path accessors still come from there.
- **Commit:** `refactor(loomcli): build the Preflight row from internal/preflightshed`

### Card 16: pin the row-1 producer's new package in the wiring test

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/preflightshed/preflight.go`
- **Edits:**
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestWire_PreflightIsTheAdapter`, change the expected `%T` literal from `"*loomshed.preflightProducer"` to `"*preflightshed.preflightProducer"` in both the comparison and the failure message.
  Update the test's own doc comment and the explanatory comment above the assertion, both of which name `loomshed.NewPreflightProducer` and "package loomshed"; they now name `preflightshed.NewPreflight` and that package.
  Keep the assertion's stated point verbatim — that the field holds a struct value from the producer's own package rather than a bare func value, which is why `%T` is compared rather than a type assertion made — and keep the "loomshed adapter" phrasing in the nil check retargeted at the new package.
  If the concrete type ends up named something other than `preflightProducer` in `internal/preflightshed`, use whatever card 3 actually declared; the package qualifier is what this assertion exists to pin.
- **Commit:** `test(loomcli): pin the Preflight row's concrete type to internal/preflightshed`

### Card 17: producer table and shed-validation fixture

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/seed.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/loomshed/loomshed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Insert `{NameLoomPreflight, ""}` into `wantProducerTable` at index 1, immediately after the `NamePreflight` row, so the literal table stays a mirror of `New`'s own order.
  Fix `TestNew_PassesShedValidation`, which is the one `testDeps`-based test that actually calls `Run`: replace its hand-written `state.WriteJSON` seed (a bare `shedengine.Status` with `CurrentProducer: NamePreflight`, running, empty history and **no product**) with a `Seed(deps.StatusPath, deps.StatusLockPath, "validation-slug", "validation-parent")` call.
  Without this, the run still reaches `RunBlocked` and the test still passes, but for the wrong reason: the productless seed makes row 2 report `CheckSeedIncoherent` on the two mandatory-field rules and block at `Loom-Preflight`, which is not the bounce-budget exhaustion between Discussion-Write and Discussion-Validate the test's own comment says it is asserting.
  Seeding through the production `Seed` keeps that comment true and matches the seeding discipline `buildSequenceFixture` already documents.
  Remove the now-unused `internal/state` import; `internal/shedengine` is still used and must stay.
  Leave `testDeps` itself writing no status file — every other test built on it stops at `New`, and that is what keeps them free of `shedengine`'s step-1 read gate.
  Confirm rather than edit `TestNew_ProducerTableOrderUnchangedByWiring`'s doc comment: its "the thirteen rows" and "rows 12 and 13" are both wrong today and become correct with this insert.
- **Commit:** `test(loomshed): add the Loom-Preflight row to the producer table fixture`

### Card 18: sequence order and row numbering

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/seed.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/loomshed/sequence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Insert `NameLoomPreflight` into `wantSequenceOrder` at index 1, giving twelve expected entries — rows 1 through 12, ending at `NamePublish`.
  Shift every row number in this file's two doc comments by one: `wantSequenceOrder`'s comment describes the row 1–12 sequence a clean `Run` produces and stops at Publish (row 12) because Publish's `OnStuck` is `""`, so row 13 (Finalize) is never invoked; `TestSequence_FullRunBlocksAtPublish`'s comment describes the 13-row list running rows 1 through 12.
  Add one sentence to `wantSequenceOrder`'s doc comment recording why the real row 2 passes against this fixture rather than needing a substituted fake: `buildSequenceFixture` seeds through the production `Seed`, which writes a coherent fresh seed, and by the instant row 2 runs `shedengine.Run` has already persisted `current_producer: "Loom-Preflight"` alongside a single `Preflight` `Done` history entry — exactly the shape row 2's told expected name and tolerated set accept.
  Change no assertion logic: the `len(result.History) != len(wantSequenceOrder)` check, the per-entry name/outcome loop and the persisted-state assertions all still hold as written.
- **Commit:** `test(loomshed): extend the sequence fixture to the thirteen-row list`

### Card 19: resume and cancellation coverage for two rows

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/sequence_test.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/loomshed/resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestCancellation_RealProducersReturnErrorNotStuck`, replace the `{NamePreflight, NewPreflightProducer(deps.AnchorPath)}` table entry with `{NameLoomPreflight, newLoomPreflight(NameLoomPreflight, deps.StatusPath, deps.StatusLockPath)}`, so the table still means "every real producer this package builds".
  Row 1's own cancellation coverage left with the constructor and now lives in `internal/preflightshed`'s own tests.
  Update the test's doc comment, which enumerates the producers it covers ("the discussion validator, the plan validator, the batch gate, the Webster wrapper, and the Preflight wrapper") — the last of those becomes loom's own seed row.
  Update `TestResume_DoesNotRestartAtRowOne`'s doc comment where it names Publish's row number, and confirm its `preflightCount != 1` assertion still holds unchanged: row 2 appends `Loom-Preflight` history entries, which never compare equal to `NamePreflight`, so the count of `NamePreflight` entries across both runs stays exactly one.
  Rewrite `TestResume_CrashRecoveryRecallsUnconditionally`'s doc comment, which currently claims "Both runs end shedengine.RunBlocked at Publish".
  That is no longer true and the test is still correct: after `resetCurrentProducer(..., NamePreflight, false)`, the second run re-calls row 1, `Shed` advances to row 2, and row 2 finds a history carrying the first run's later producers, which is exactly the half-finished shape the fresh-start rule rejects — so the second run blocks at `Loom-Preflight` instead.
  Say so explicitly, and restate that this test's own point is the re-call count, not the terminal state.
  Do not change any assertion in that test: both `RunBlocked` checks and the `counting.calls` checks hold as written.
  In `TestResume_PauseStopsAtBoundaryAndClearsFlag`, confirm rather than change: it resets `current_producer` straight to `NameBatchifier`, so row 2 is never called on either run.
  Leave `resetCurrentProducer`, `countingProducer` and all four `TestBounceRouting_*` tests untouched.
- **Commit:** `test(loomshed): retarget resume and cancellation coverage at the two rows`

### Card 20: Tier-1 row-2 producer test

- **Context:**
  - `internal/loomshed/loompreflight.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/seed.go`
  - `internal/loomengine/status.go`
  - `internal/state/state.go`
  - `internal/shedengine/status.go`
  - `docs/benchmarks/running-tests.md`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/loompreflight_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an **untagged** (Tier-1) `package loomshed` test file covering `newLoomPreflight`'s outcome mapping only, over status files under `t.TempDir`.
  Three cases: a coherent post-row-1 seed → `shedengine.Done` with a nil error; an incoherent seed → `shedengine.Stuck` with a nil error; a lock path whose parent cannot be created → a non-nil error with an outcome that is neither `Done` nor `Stuck`.
  **The passing fixture must be hand-written, not produced by `Seed`.** `Seed` writes `current_producer: "Preflight"`, which row 2 rejects because it tells `NameLoomPreflight` as the expected value, so a `Seed`-built fixture would yield `Stuck` and the "coherent seed → Done" case would silently assert the opposite of what it names.
  Write the shape `Shed` itself persists after row 1: `CurrentProducer: NameLoomPreflight`, `State: shedengine.StateRunning`, `History` carrying one entry `{Producer: NamePreflight, Outcome: shedengine.Done, At: <an RFC3339 UTC timestamp literal>}`, `PauseRequested: false`, and a `Product` holding a marshalled `loomengine.Status` with non-empty `Slug`/`Parent` and a nil `StartSha`; write it with `state.WriteJSON`.
  Record that deliberate divergence in the test's doc comment — this is the one place in the task where the seed contract and the row-2 contract differ, and a fixture built from the wrong one fails in the direction that looks like a passing test.
  For the incoherent case, reuse the same helper with `CurrentProducer` left at `NamePreflight`.
  For the error case, create a **regular file** at some path under the temp dir and point `statusLockPath` at a child path beneath that file, so `CheckSeed`'s `os.MkdirAll(filepath.Dir(statusLockPath), …)` guard fails; the status file itself must exist and be coherent so the failure is provably the lock-parent creation and not the stat.
  Do **not** re-test `CheckSeed`'s check set here — that is `internal/loomengine`'s job, and duplicating it would couple this package's tests to another package's checks, the same reasoning the moved Tier-2 wrapper test already records.
  This file must contain none of the substrings named in the `untagged-tests-carry-no-spawn-token` Shared Decision.
- **Commit:** `test(loomshed): add the Tier-1 Loom-Preflight outcome-mapping test`

## Batch Tests

Verified by the batch's three-command `verify:` chain.

Tier 1 is where nearly all of this batch is proven.
`internal/loomshed`'s untagged suite is the densest gate in the task: `TestNew_ProducerTable` and `TestNew_ProducerTableOrderUnchangedByWiring` pin the thirteen-row order and each row's `OnStuck`; `TestSequence_FullRunBlocksAtPublish` drives the whole list against a real, `Seed`-built fixture and is what actually proves a real row 2 reports `Done` against the post-row-1 shape `Shed` persists; the four `TestResume_*` tests prove resume, crash recovery and pause still behave with an inserted row; `TestCancellation_RealProducersReturnErrorNotStuck` proves row 2 honours the cancellation obligation; and the new `loompreflight_test.go` pins the three-way outcome mapping in isolation.
`internal/loomcli`'s untagged `wiring_test.go` is what catches the moved concrete type, which would otherwise fail only at runtime.
`cmd/lyx/tierpurity_test.go` guards the one new untagged file.

Tier 2 covers two things this batch could break invisibly: `internal/preflightshed`'s moved wrapper test still passes against the producer `internal/loomcli` now constructs, and `internal/loomengine`'s still-present integration suite proves batch 2's transitional `runCheck4` is untouched by this batch's edits.

`go vet -tags smoke ./internal/loomcli` compiles `smoke_test.go`, which this batch does not edit but which references `loomshed` symbols — it is the cheapest proof that the row insert did not break the smoke suite's compilation ahead of batch 4's own edits to that file.
