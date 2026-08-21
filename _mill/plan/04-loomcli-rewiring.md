# Batch: loomcli-rewiring

```yaml
task: 'loom: convert to a Shed recipe'
batch: 'loomcli-rewiring'
number: 4
cards: 5
verify: go test ./internal/loomcli/...
depends-on: [1]
```

## Batch Scope

This batch swaps loom's CLI off `loomshed.Deps`/`loomshed.New` and onto `shedrecipe.Env`/`loomrecipe.ShedPaths`/`loomrecipe.New`, removing the last production caller of the symbols batch 5 deletes.
It is the widest mechanical edit in the task: four of loom's five verbs read the status-path pair off `c.deps` today, so the replacement carrier has to be reachable from every command, not just `drive`.

It depends only on batch 1 and touches no file batches 2 or 3 touch, so it may run in parallel with them.
`loomshed.Deps` still exists throughout — this batch stops using it, batch 5 deletes it.

Batch-local decisions:

- The two replacement fields on `loomCLI` are named `env` (a `shedrecipe.Env`) and `shedPaths` (a `loomrecipe.ShedPaths`).
  Both are stored on the receiver, not built inside `drive.go`: `status`, `pause`, and `run` all read the told paths off `c.deps` today, exactly as `Deps` is reachable from every verb.
- `c.runDeps` stays exactly as it is.
  It is the assembled `websterengine.RunDeps`, kept beside the wrapper so a test can inspect it without unwrapping, and it is now also `env.WebsterDeps` rather than `deps.WebsterDeps`.
- The `CLI / Cobra Invariant` is untouched: no command's `Use`, `Short`, `Long`, or position in the help tree changes, and no command is added or removed.
  Only the constructor `drive` calls and the field names four verbs read change.
- **Cards 15 through 19 are one compile unit and are green only at the batch boundary.**
  Card 15 assigns `c.env`/`c.shedPaths` before card 16 declares them;
  card 16 removes the `deps` field while cards 17, 18, and 19 still read it.
  Each card still gets its own commit, matching every other batch here, but do not expect a mid-batch commit to build — run `verify:` once the batch is complete, not after each card.
  Batch 2 carries the same coupling for cards 5 and 6.

## Cards

### Card 15: Build `Env` and `ShedPaths` in `wire`

- **Context:**
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/shed.go`
  - `internal/loomengine/config.go`
  - `internal/shedadapters/webster.go`
  - `internal/landingshed/deps.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `c.deps = loomshed.Deps{…}` literal in `wire` with two assignments: `c.env = shedrecipe.Env{…}` and `c.shedPaths = loomrecipe.ShedPaths{…}`.

  `c.env` gets nine of the ten fields loom's thirteen rows read, sourced identically to today;
  `Landing` is the tenth and stays unfilled, per the paragraph below:
  `Cwd` from the `cwd` parameter (the same value `preflightshed.NewPreflight` is handed today);
  `AnchorPath` from `anchorPath`;
  `WorktreeRoot` from `location.WorktreePath()`;
  `StatusPath` from `loomengine.LoomStatusFile(location)`;
  `StatusLockPath` from `loomengine.LoomStatusLock(location)`;
  `DecisionRecordPath` from `loomengine.DiscussionDecisionRecord(location)`;
  `SupportLogPath` from `loomengine.DiscussionSupportLog(location)`;
  `WebsterDeps` from the assembled `runDeps`;
  and `WebsterRun` set explicitly to `websterengine.Run` per the `env-webster-run-is-filled-explicitly` Shared Decision.
  `Landing` is left unfilled, per the `landing-parity` Shared Decision — carry a comment saying so, pointing at `internal/landingshed/deps.go`'s own account of the gap and at the roadmap item card 24 adds for the parent-fabric resolution chain, so the omission reads as preserved parity rather than an oversight.
  `StencilsDir`, `RunRoot`, `Shuttle`, `Burler`, and `Now` are left zero — only `SingleLLM`, `Bouncer`, and `BurlerRound` read them, and no row uses those engines yet.
  Carry a comment saying that too.

  `c.shedPaths` gets `StatusPath` from `loomengine.LoomStatusFile(location)`, `LockPath` from `loomengine.LoomRunLock(location)`, `StatusLockPath` from `loomengine.LoomStatusLock(location)`, and `MaxBounces` left zero.
  Keep the existing `MaxBounces` comment verbatim — the per-producer, episode-scoped default it explains is unchanged by this conversion.
  `StatusPath` and `StatusLockPath` are deliberately told twice, once in each struct;
  carry a comment saying the duplication is inherent to the `Env`-versus-`Shed` split and must not be collapsed, and that `loomrecipe.New` errors if the two copies disagree.
  Fill each pair from a single evaluation of its `loomengine` accessor rather than calling the accessor twice, so the two copies cannot drift here.

  Stop constructing the `Preflight` row: delete the `Preflight: preflightshed.NewPreflight(loomshed.NamePreflight, cwd)` field and the `internal/preflightshed` import.
  `preflightEntry` in `internal/shedrecipe/entries_simple.go` now builds it from `Env.Cwd` with exactly the same call.
  Delete the `internal/loomshed` import if nothing else in this file uses it after the change.
  Add imports for `internal/loomrecipe` and `internal/shedrecipe`.

  Update `wire`'s own doc comment, which says it builds "the assembled `loomshed.Deps` wrapping it".
- **Commit:** `refactor(loomcli): wire loom's Shed through the recipe Env`

### Card 16: Repoint the `loomCLI` receiver fields

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/env.go`
- **Edits:**
  - `internal/loomcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `deps loomshed.Deps` field on `loomCLI` with two fields: `env shedrecipe.Env` and `shedPaths loomrecipe.ShedPaths`.
  Rewrite the field doc comment, which names `loomshed.Deps` and `loomshed.New` explicitly and says `statusCmd`/`pauseCmd` read its `StatusPath`/`StatusLockPath` pair: `env` is the assembled `shedrecipe.Env` that `driveCmd` passes to `loomrecipe.New`, and `shedPaths` carries the four told values `shedengine.Shed` itself reads, which `driveCmd` passes alongside and which `statusCmd`, `pauseCmd`, and `runCmd` read directly.
  Update the `runDeps` field's doc comment where it says "embedded verbatim as deps.WebsterDeps" — it is now `env.WebsterDeps`.
  Update the `cwd` field's doc comment where it says `preflightshed.NewPreflight` reads it — `cwd` now travels to `preflightEntry` as `Env.Cwd`, and that entry makes the same constructor call.
  Replace the `internal/loomshed` import with `internal/loomrecipe` and `internal/shedrecipe` if nothing else in the file uses `loomshed`.
- **Commit:** `refactor(loomcli): carry Env and ShedPaths on the CLI receiver`

### Card 17: Swap the constructor and repoint every told-path read

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomshed/seed.go`
  - `internal/shedengine/shed.go`
- **Edits:**
  - `internal/loomcli/drive.go`
  - `internal/loomcli/pause.go`
  - `internal/loomcli/status.go`
  - `internal/loomcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/drive.go`, replace `loomshed.New(c.deps)` with `loomrecipe.New(c.env, c.shedPaths)` and repoint the two `c.deps.StatusPath` reads in the no-status-file refusal at `c.shedPaths.StatusPath`.
  Swap the `internal/loomshed` import for `internal/loomrecipe`.
  Everything else in the file — the `ShouldAbort` guard, the refusal's message text and remedy, the `ErrShedBusy` posture, and the output envelope's four keys — is unchanged.

  In `internal/loomcli/pause.go`, repoint the `c.deps.StatusPath`/`c.deps.StatusLockPath` pair passed to `state.UpdateJSON`, the `c.deps.StatusPath` in the not-found error text, and the `c.deps.StatusPath` in the output envelope's `status_file` key, all at `c.shedPaths`.

  In `internal/loomcli/status.go`, repoint all five `c.deps.` reads at `c.shedPaths`: the `state.ReadJSONStrict` pair, the decode-error message, the no-status-file refusal, the `--watch` poll's `ReadJSONStrict` pair, and the product-payload decode-error message.

  In `internal/loomcli/run.go`, repoint the `loomshed.Seed(c.deps.StatusPath, c.deps.StatusLockPath, …)` call's two arguments and the `runLockPath := c.deps.LockPath` assignment at `c.shedPaths`.
  Keep the `internal/loomshed` import here — `Seed` and `ErrSeedExists` both stay in that package and are unchanged by this task.

  Also repair `RunAliasCommand`'s doc comment in the same file, which says the alias "would run with location, cwd, and deps all left unresolved" — the `deps` field disappears in card 16, so name the two replacement carriers instead.

  Before committing, run a `c\.deps\.` grep across `internal/loomcli` and confirm the only remaining hits are in `cli_test.go` and `wiring_test.go`, which cards 18 and 19 handle, and a bare `deps` grep across the four files this card edits to confirm no doc comment still names the removed field.
- **Commit:** `refactor(loomcli): build loom's Shed through loomrecipe.New`

### Card 18: Update the `wire` tests

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/cli.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/loomrecipe/loomrecipe.go`
- **Edits:**
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Repoint every `c.deps.` read at the new carriers.

  `TestWire_PathFieldsMatchLoomengineAccessors` asserts seven fields today.
  Split them across the two carriers: `StatusPath`, `LockPath`, and `StatusLockPath` come off `c.shedPaths`;
  `AnchorPath`, `WorktreeRoot`, `DecisionRecordPath`, and `SupportLogPath` come off `c.env`.
  Add an eighth assertion for `c.env.StatusPath` and a ninth for `c.env.StatusLockPath` — both are told twice and both halves must match their `loomengine` accessor, since `loomPreflightEntry` reads the `Env` copy while `shedengine.Shed` reads the `ShedPaths` copy.

  `TestWire_RunLockDiffersFromStatusLock` repoints both sides at `c.shedPaths`.

  `TestWire_PreflightIsTheAdapter` is **restated, not repointed**.
  The `Preflight` field it asserts on disappears entirely under this batch's change, so the test becomes `TestWire_CwdIsToldToTheEnv`: assert `c.env.Cwd` equals the `cwd` argument `wire` was called with, which is the only preflight-related property `wire` still owns.
  Rewrite the doc comment accordingly, stating where the old property went: "row 1 is the `preflightshed` adapter, not a bare func" is now `preflightEntry`'s own property, covered by `internal/shedrecipe`'s own entry test, and the row's engine name is pinned by the recipe-side coverage guard in `internal/loomrecipe`.
  Drop the `fmt` import if the `%T` rendering it existed for is now gone.

  Add a new `TestWire_WebsterRunIsFilled` asserting `c.env.WebsterRun` is non-nil.
  This is the single most likely regression the conversion reintroduces: `websterEntry` calls `requireSeam("Webster", "WebsterRun", env.WebsterRun)` and errors on nil, while the pre-conversion `wire` deliberately left the corresponding field nil.
  Say that in the test's doc comment.

  `TestWire_WebsterDepsFullyPopulated`'s final assertion reads `c.deps.WebsterDeps.Geom` — repoint it at `c.env.WebsterDeps.Geom`.
  Its other eight assertions read `c.runDeps` and are unchanged.
  `TestWire_RefMatcherIsRealScanner` and `TestWire_BisectorOpenerNonNilInHubOnlyMode` read `c.runDeps` only and need no change.
- **Commit:** `test(loomcli): assert the Env and ShedPaths wire builds`

### Card 19: Repoint the verb-refusal fixture

- **Context:**
  - `internal/loomcli/drive.go`
  - `internal/loomcli/pause.go`
  - `internal/loomcli/cli.go`
  - `internal/loomrecipe/loomrecipe.go`
- **Edits:**
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestVerbRefusals` hand-builds a `loomCLI` whose `deps` is a `loomshed.Deps{StatusPath, StatusLockPath}` literal, so it cannot compile once that field is gone.
  Repoint the fixture at `shedPaths: loomrecipe.ShedPaths{StatusPath: …, StatusLockPath: …}` over the same `t.TempDir()`-derived paths.
  Every assertion stays unchanged: exit code 1, an `"ok":false` envelope, and the `lyx loom run` remedy string, for both the `drive` and `pause` sub-cases.
  Update the test's doc comment where it names `c.deps.StatusPath/StatusLockPath`.
  Swap the `internal/loomshed` import for `internal/loomrecipe` if nothing else in the file uses `loomshed`.
  Do not route this fixture through `wire` — the comment's reason still holds: `wire` needs a real git repository, which this untagged suite must not spawn.
- **Commit:** `test(loomcli): repoint the verb-refusal fixture at ShedPaths`

## Batch Tests

`verify: go test ./internal/loomcli/...` runs the package's untagged suite: `wiring_test.go` (now nine path assertions split across the two carriers, the restated cwd test, and the new `WebsterRun` non-nil assertion), `cli_test.go` (the repointed verb-refusal fixture plus the help-tree and envelope tests, which this batch does not touch), `status_test.go`, `seedinput_test.go`, and `bootstrap_test.go`.
`smoke_test.go` carries a `//go:build smoke` tag and does not run here;
it uses only `loomshed.Seed`, which this task leaves untouched, so it is unaffected by this batch either way.
Note that `pipeline.done_gate`'s `go test -tags integration ./...` does **not** reach it — `-tags integration` does not enable `smoke` — so the only thing in this plan that compiles that file is batch 6's `&& go vet -tags smoke ./internal/loomcli/` tail, added there because batch 6 edits a comment in it.

`wire` itself is still driven directly against a hand-built `*lyxcwd.Location` over a temp directory seeded with `loom.yaml` alone, so the package stays tier 1 and spawns no process.
Note that `wire` never builds a `Shed` — `loomrecipe.New` is called only from `driveCmd` — so no `wire` test triggers `landingshed.NewPublish`'s nil-closure rejection despite `Env.Landing` being left unfilled.
That existing production failure is preserved unchanged, per the `landing-parity` Shared Decision, and this batch adds no test asserting it either way.

The module-wide `go vet ./...` at the batch boundary is what proves the four verb files and both test files typecheck against the new receiver shape together — `go build` would not, since it never compiles `_test.go` files.
