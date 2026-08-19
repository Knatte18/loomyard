# Batch: sequence-and-integration

```yaml
task: 'loom: phase-machine scaffolding'
batch: sequence-and-integration
number: 3
cards: 4
verify: go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/... && go test -tags integration ./internal/loomshed/...
depends-on: [2]
```

## Batch Scope

This batch delivers the task's own verify requirement — the full 12-row sequence running to completion against the real list, including resume, crash-recovery and pause — plus the one integration-tagged test that covers the only row needing real git, and the roadmap move that closes the item out.
It is one batch because all four cards sit in `internal/loomshed`'s test surface and share one fixture builder: card 11 builds it, cards 12 and 13 reuse it.

There is no external interface for a later batch to consume; this batch is terminal.

Batch-local decision: rows 3, 7 and 9 are real code reading real on-disk state, and the fixture makes them genuinely pass rather than adding injection points for them. Adding injection points would contradict the two-rows-only rule in `## Shared Decisions`' `explicit-deps-struct` and would stop the sequence test exercising the three producers this task actually builds — which is most of its value.

## Cards

### Card 11: the shared Tier-1 fixture and the full 12-row sequence

- **Context:**
  - `internal/shedengine/shed.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/seed.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate.go`
  - `internal/planparser/testdata/goodplan/00-overview.md`
  - `internal/planparser/testdata/goodplan/01-json-flag.md`
  - `internal/batcher/config.go`
  - `internal/configengine/config.go`
  - `internal/state/state.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/sequence_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/fixture_test.go`, add one shared builder that produces a whole temp anchor a real `New`-built list can run against offline, plus the `Deps` pointing at it, and returns both. Every sequence test in this batch reuses it. It must make rows 3, 7 and 9 genuinely pass, since only rows 1 and 10 are injectable: for `Discussion-Validate` it writes both discussion files with all seven required H2 sections; for `Plan-Validate` it writes a plan directory that satisfies every one of `planparser.Validate`'s checks, including the ones that stat paths against the worktree root, so it materializes those referenced files on disk too — the fixture plan is self-authored and may be as small as one card, so a single representative card file plus the overview is all the format this needs — `internal/planparser/testdata/goodplan/00-overview.md` and `internal/planparser/testdata/goodplan/01-json-flag.md` are that reference pair, already a zero-findings plan, and the directory's three remaining card files are deliberately not read, since they would only re-demonstrate the same card format; for `Batchifier` it writes no batch config at all, since `batcher.Active` resolves the embedded template when the config file or its directory is absent, which is a `Done`. It seeds the status file through the production `Seed`, never by hand-writing JSON — a test-local seed would let a `Seed` regression pass unnoticed. It injects a fake `Preflight` returning `Done` and a fake `WebsterRunner` returning Webster's own done outcome, and gives `LockPath` and `StatusLockPath` two distinct paths, since `shedengine` rejects them naming one file. In `internal/loomshed/sequence_test.go`, add the sequence test the task's verify requirement names: build the fixture, call `Run`, and assert the run terminates with `shedengine.RunDone`, that the persisted history names all twelve producers in table order with every outcome `done`, and that the persisted state is `done` with `current_producer` still naming the final row. Assert against a literal expected name sequence rather than a computed one, so a reordering is a failure rather than a silently-agreeing derivation.
- **Commit:** `test(loomshed): add the Tier-1 fixture and the full 12-row sequence test`

### Card 12: resume, crash-recovery, pause, bounce routing, and cancellation

- **Context:**
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/shed.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/webster.go`
  - `internal/loomshed/preflight.go`
  - `internal/loomshed/stub.go`
  - `internal/state/state.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/resume_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/resume_test.go`, cover the four run-level behaviours the task exists to make real from the start, plus the one obligation `Shed` cannot enforce, all over card 11's shared fixture. Resume: run the list to a mid-list producer, construct a fresh `Shed` over the same status file, and assert it re-calls whatever `current_producer` names and then completes — the second run must not restart at row 1. Crash-recovery: assert the re-call is unconditional, so a producer whose output already exists is still called again rather than skipped; count calls on an injected fake to prove it. Pause: set the pause flag on the status file, assert the run stops at the next producer boundary with `shedengine.RunPaused`, that the flag is written false in the same persist that records the paused state, and that a subsequent run resumes rather than re-pausing on the flag it is resuming from. Bounce routing: drive a gate to `Stuck` and assert the run continues at that row's declared bounce target; drive a row whose target is empty to `Stuck` and assert the run ends `shedengine.RunBlocked` instead; and assert the bounce budget is consumed and that exhausting it blocks, using a small `Deps.MaxBounces` so the test does not need ten round trips. Cancellation: assert each real producer this task builds — the discussion validator, the plan validator, the batch gate, the Webster wrapper, and the Preflight wrapper — returns a non-nil error rather than `Stuck` when called under an already-cancelled context. That last one is the obligation `Shed` cannot check for itself: a `Stuck` under a cancelled context is indistinguishable to `Shed` from a genuine verdict and would silently consume bounce budget for what was an operator stop. Drive the stuck cases by swapping the fixture's on-disk state so a real producer genuinely fails — a missing discussion file, an invalid plan — rather than by substituting a fake row, wherever the row under test is one this task builds.
- **Commit:** `test(loomshed): cover resume, crash-recovery, pause, bounce routing, and cancellation`

### Card 13: integration coverage for the real `Preflight` wrapper

- **Context:**
  - `internal/loomshed/preflight.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/loomengine/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/loomshed/seed.go`
  - `internal/loomengine/config.go`
  - `internal/gitkit/hermetic.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/preflight_integration_test.go`
  - `internal/loomshed/testmain_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one integration-tagged test covering `loomshed.NewPreflightProducer`'s real wrapper against a `hubforge` fixture hub — the only row that needs real git. Both new files carry `//go:build integration` as their first line and live in package `loomshed_test`, an external test package: `internal/loomshed` imports `internal/loomengine`, which sits inside the dependency set `internal/hubforge` reaches, so an in-package test importing `hubforge` would close a compile cycle — the same reason `internal/loomengine`'s own integration test is an external-package file. `internal/loomshed/testmain_integration_test.go` holds a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, required because the package now spawns git via a fixture helper. Keep `internal/loomshed/preflight_integration_test.go` to the wrapper's outcome mapping and nothing more: a fixture whose preconditions all pass yields `shedengine.Done`, and a fixture with a deliberately broken precondition yields `shedengine.Stuck`. Seed the fixture's status file through `loomshed.Seed`, which writes exactly the fresh, coherent shape check 4 expects. Do not re-test `Preflight`'s own four checks here — they are already covered exhaustively by `internal/loomengine`'s existing integration suite, and duplicating them here would couple this package's tests to another package's check set.
- **Commit:** `test(loomshed): add integration coverage for the real Preflight wrapper`

### Card 14: move the roadmap item to Done

- **Context:**
  - `manifest/designs/loom.md`
  - `docs/overview.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, move the Planned `**loom: phase-machine scaffolding**` item into the Done section, rewritten to the short name-plus-a-sentence-or-two shape the file's own Maintenance section requires of a Done entry — the Planned item's seven-bullet detail does not survive the move, since a Done entry points at the module's own package documentation rather than restating its build list. Say what shipped: `internal/loomshed` carrying the full 12-row producer list with `Discussion-Validate`, `Plan-Validate` and `Batchifier` built for real, `Preflight` and `Webster` wired in as-is, the remaining seven rows stubbed, and loom's status file migrated onto `shedengine.Status` with a production seeder. Point at the `internal/loomshed` package documentation and at the Told-Geometry Invariant, following the link style the neighbouring Done entries already use. Leave the numbering alone — every item is written literally as `1.` and renders sequentially, so no renumbering is needed anywhere. Leave the Planned `loom: session bootstrap` and `loom: write and wire in the real LLM producers` items untouched: both are still Planned, and this task deliberately builds neither.
- **Commit:** `docs(roadmap): move loom phase-machine scaffolding to Done`

## Batch Tests

`verify:` runs `go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/...` plus a second, integration-tagged pass over `./internal/loomshed/...`.

- The untagged `./internal/loomshed/...` pass covers this batch's own `fixture_test.go`, `sequence_test.go` and `resume_test.go`, and re-runs batch 2's whole suite against them.
- The `-tags integration` pass is required, not optional: card 13's two files carry `//go:build integration` and are invisible to the untagged run, so without it the only coverage of the real `Preflight` wrapper would never execute.
- `./cmd/lyx/...` covers `tierpurity_test.go`, which is what proves cards 11 and 12 stayed offline while card 13 correctly tagged the file that does not, and `hermeticenv_test.go`, which is what proves card 13's `TestMain` is present.
- `./internal/lyxcwd/...` covers `docslink_test.go` over card 14's `manifest/roadmap.md` edit.
