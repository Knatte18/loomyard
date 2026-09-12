# Batch: wiring

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'wiring'
number: 7
cards: 5
verify: go test ./internal/loomcli/... ./internal/webstercli/... ./internal/burlercli/...
depends-on: [2, 3, 4, 5, 6]
```

## Batch Scope

This batch is where Tier 2 actually switches on.
It resolves the friction directory once per CLI wiring, fills it into the two engines that are told it, owns the directory's create and its once-per-task clear, fires the reflection step at its single call site, and surfaces the result on `lyx loom drive`'s envelope.

It depends on every other implementation batch: batch 2 for the accessors and config keys, batch 3 for `frictionengine.Reflect`, batch 4 for `loomengine`'s in-package derivation, batch 5 for `burlerengine.New`'s fifth parameter, and batch 6 for the three webster Deps fields.

Batch-local decision: `internal/webstercli` resolves the friction directory itself in hub mode rather than receiving it from `internal/loomcli`, because `contracts/stencils/webster/webster-template-master.md` has Master drive the batch loop by shelling out to `lyx webster begin-batch` and `lyx webster recover-batch`, so the implementer fork's and recovery strand's prompts are composed in a separate `lyx webster` process whose Deps `internal/webstercli/wiring.go` builds.
The full rationale is in the overview's Shared Decisions.

## Cards

### Card 22: resolve the friction directory in `loomcli`'s wiring and fill the two told engines

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/burlerengine/engine.go`
  - `internal/websterengine/runlevel.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `wire`, immediately after `websterGeom` is built, resolve one value: `frictionDir` is `loomengine.LoomFrictionDir(location)` when `loomCfg.Friction != ""`, and `""` otherwise.
  Everything else in this file reads that one variable, so no two layers can disagree about whether Tier 2 is on.

  Replace the placeholder fifth argument at `internal/loomcli/wiring.go:266` — `burlerengine.New(runner, hubgeom.BurlerGeometry(location), burlerCfg, websterGeom.StencilsDir, "")` — with `frictionDir`, and drop the placeholder comment batch 5 left there.

  Add `FrictionDir: frictionDir` to the `websterengine.RunDeps` literal at `internal/loomcli/wiring.go:268`, with a short comment stating that this covers the Master and integration prompts only, and that the per-batch fork and recovery prompts are composed in a separate `lyx webster` process which resolves the same value itself in `internal/webstercli`.

  Do **not** add a friction field to `shedrecipe.Env`: no registry entry reads these values, and `Env` is documented as carrying only roots and run-wide values its own entries read.
  `loomengine`'s two composers need nothing here — they derive the directory in-package from the `*lyxcwd.Location` and `Config` their Spec factories already take.

  Add a `frictionDir string` field to the `loomCLI` struct in `internal/loomcli/cli.go`, placed beside the existing `registry` and `runner` fields and carrying the same shape of doc comment they do: the absolute friction directory resolved once in `wire`, empty when Tier 2 is off, carried on the struct so `run.go` and `drive.go` read it without re-resolving or re-reading `loom.yaml`.
  The struct is declared in `internal/loomcli/cli.go` and nowhere else, so `internal/loomcli/wiring.go` alone cannot add the field.

  Assign it in `wire` — `c.frictionDir = frictionDir` — beside the existing `c.cfg`/`c.runner` assignments at the end of that function.
  Leave it at its zero value in `wireLightweight`: every verb on that path is read-only, and that function deliberately loads no module config.
- **Commit:** `feat(loomcli): resolve the friction directory once and fill the told engines`

### Card 23: the once-per-task clear and the two ensure sites

- **Context:**
  - `internal/loomshed/seed.go`
  - `internal/friction/friction.go`
  - `internal/loomengine/config.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/drive.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **`internal/loomcli/run.go`, beside the seed call.**
  The shipped statement at `internal/loomcli/run.go:101` is `if err := loomshed.Seed(...); err != nil && !errors.Is(err, loomshed.ErrSeedExists)`, which collapses both outcomes into one branch and never binds the error.
  Restructure it to bind the error to a variable first, so a genuine first seed (nil error) can be told apart from an `ErrSeedExists` re-entry.
  Expect this restructure — it is not a call added beside an unchanged line.

  On the **nil-error branch only**, `os.RemoveAll` the friction directory before anything else runs.
  On **both** branches, call `friction.EnsureDir` on it.
  Clear-on-first-seed-only is what keeps a crash-resumed run's notes: an `ErrSeedExists` re-entry is by definition a resume, and those are exactly the notes most worth reading.
  A failed `os.RemoveAll` logs at `Warn` via `internal/logger` and execution continues;
  it never fails `lyx loom run`.
  Both operations are skipped entirely when the resolved friction directory is empty.

  `loomshed.Seed`'s own signature is **not** touched: its told-parameter list is about status seeding, and widening it for an unrelated directory is a worse seam than one `os.RemoveAll` at the call site that can cheaply tell the two outcomes apart.

  **`internal/loomcli/drive.go`, at startup.**
  Call `friction.EnsureDir` on the resolved friction directory before `loomrecipe.New` and `shed.Run`, and **never** clear it.
  `lyx loom drive` requires an already-seeded task (`internal/loomcli/drive.go:61` calls `loomengine.VerifySeedOwnership`), so a drive-only invocation is by definition a resume, which is the case that must keep its notes.
  The ensure is still needed here because `frictionengine.Reflect` renames the directory away on a clean reflection — including on the `RunBlocked` trigger — so a `blocked -> operator investigates -> lyx loom drive` sequence would otherwise run with no friction directory at all and lose every note silently.

  Both sites read the friction directory from the same resolution card 22 stores on the receiver;
  neither re-derives it, and neither reads `loom.yaml` a second time.
- **Commit:** `feat(loomcli): own the friction directory's once-per-task clear and its creation`

### Card 24: fire the reflection step and surface it on drive's envelope

- **Context:**
  - `internal/frictionengine/deps.go`
  - `internal/frictionengine/reflect.go`
  - `internal/loomengine/config.go`
  - `internal/shedengine/run.go`
  - `internal/modelspec/modelspec.go`
- **Edits:**
  - `internal/loomcli/drive.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `driveCmd`'s `RunE`, immediately after `shed.Run(cmd.Context())` returns and before the `output.Ok` envelope is written, call `frictionengine.Reflect` — but only when `result.Outcome` is `shedengine.RunDone` or `shedengine.RunBlocked`, and only when `err` is nil, and only when the resolved friction directory is non-empty.

  Never on `shedengine.RunPaused`: that is an operator stopping the run mid-flight, the run is not over, and re-filing on every pause would be noise.
  Never on a non-nil `err`: that is an engine-level fault where the run's own bookkeeping is untrustworthy.
  Never from a recipe row: `shedengine.Run` returns immediately on `RunBlocked` without calling any further producer (both the `def.OnStuck == ""` and the bounce-budget-exhausted branches in `internal/shedengine/run.go` return rather than continue), so a row can structurally never cover the `stuck` half of the two trigger points, and a row placed after Finalize would cover only the `RunDone` half while still needing a second implementation for `stuck`.

  Build `frictionengine.Deps` from values already in scope: `Shuttle: c.runner`;
  `FrictionDir` from the receiver;
  `ArchivePrefix: loomengine.LoomFrictionArchivePrefix(c.location)`;
  `StencilsDir: c.runDeps.Geom.StencilsDir` — the same value `wire` already gave every other consumer, read from one place rather than resolved a second time;
  `FrictionSpec: c.cfg.Friction`;
  `Registry: c.registry`;
  `Timeout: time.Duration(c.cfg.FrictionTimeoutMin) * time.Minute`;
  `Clock` left nil so the production clock applies.

  Add a `"friction"` key to the existing `output.Ok` map at `internal/loomcli/drive.go:138-143`, carrying `frictionengine.Report.Status` — one of `"skipped"`, `"reflected"`, or `"failed"`.
  Write `"skipped"` for every case where `Reflect` is not called at all, including Tier 2 being off.
  It never replaces or reorders the existing `outcome`, `halted_producer`, `reason`, or `history_length` keys.

  A non-nil error from `Reflect` is a `Deps`-validation failure — a wiring bug — and is logged at `Warn` with the `friction` key set to `"failed"`;
  it never sets a non-zero exit, never converts `RunDone` into an error, and never reaches `output.Err`.
  Failing a successful, already-merged run because an optional bookkeeping agent could not run is strictly worse than filing nothing, and the `RunBlocked` case is worse still: an operator staring at a blocked run does not need a second, unrelated failure layered on top.

  On `RunDone` this ordering means the reflection agent runs after Finalize has already merged and published.
  That is correct: self-report filing is out-of-band bookkeeping, never a gate on landing the work.
  Say so in a comment at the call site.
- **Commit:** `feat(loomcli): fire the friction reflection step after shed.Run and report it on drive's envelope`

### Card 25: `webstercli` resolves the friction directory in hub mode

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/logger/logger.go`
  - `internal/loomcli/wiring.go`
  - `contracts/stencils/webster/webster-template-master.md`
- **Edits:**
  - `internal/webstercli/wiring.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `frictionDir string` field to the `websterCLI` receiver in `internal/webstercli/cli.go`, documented as the absolute friction directory in hub mode and the empty string in standalone, and as the reason this CLI imports `internal/loomengine` at all.

  In `wireHub` (`internal/webstercli/wiring.go:105`), after `anchorPath` is bound, resolve it **tolerantly**: load loom's config with `loomengine.LoadConfig(anchorPath, "loom")` and, on success with a non-empty `Friction`, set `c.frictionDir = loomengine.LoomFrictionDir(loc)`.
  On a load error, or on an empty `Friction`, leave it `""`.
  A load error additionally logs at `Warn` naming the error and stating that Tier 2 friction notes are disabled for this invocation.
  It must never return the error: a `lyx webster` verb is not allowed to fail because an unrelated module's config could not be read, and this whole value is optional bookkeeping.

  In `wireStandalone` (`internal/webstercli/wiring.go:200`), leave `c.frictionDir` at its zero value with a one-line comment: a standalone webster run is not a loom run and has no friction directory.

  Fill `FrictionDir: c.frictionDir` into all three Deps literals: `websterengine.BeginDeps` at `internal/webstercli/beginbatch.go:93`, `websterengine.RecoverDeps` at `internal/webstercli/recoverbatch.go:136`, and `websterengine.RunDeps` in `runDeps()` at `internal/webstercli/run.go:34`.

  This is what makes the per-batch implementer fork and the cold recovery strand actually receive the directive under `lyx loom run`: Master drives the batch loop by running `lyx webster begin-batch <NN>` and `lyx webster recover-batch <NN>` as separate processes, so their prompts are composed here and not in `internal/loomcli`.
  Record that in `wireHub`'s own comment, so a future reader does not "simplify" the resolution away as a duplicate of `internal/loomcli/wiring.go`'s.
- **Commit:** `feat(webstercli): resolve the friction directory in hub mode for the fork and recovery prompts`

### Card 26: wiring tests

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/run.go`
  - `internal/loomcli/drive.go`
  - `internal/webstercli/wiring.go`
  - `internal/frictionengine/deps.go`
  - `internal/loomengine/config.go`
  - `internal/loomshed/seed.go`
- **Edits:**
  - `internal/loomcli/wiring_test.go`
  - `internal/webstercli/wiring_test.go`
- **Creates:**
  - `internal/loomcli/friction_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Every test added here is untagged Tier 1: no `exec.Command`, no `gitexec`, no `hubforge.NewHub`, no real spawn, no `time.Sleep` at or above one second.
  Do not add a `smoke`-tagged test for any of this.

  **`internal/loomcli/wiring_test.go`.**
  Assert that `wire` fills the friction directory into both told engines when `loom.yaml`'s `friction` key is non-empty, and fills the empty string into both when it is present-but-empty — asserted against `loomengine.LoomFrictionDir(loc)`'s own return value rather than a hand-built path literal, the way that file's existing `RunRoot` assertion at `internal/loomcli/wiring_test.go:483` already compares against `loomengine.LoomReviewsDir(loc)`.

  **`internal/loomcli/friction_test.go`.**
  Cover the clear-and-create split, which is the pair of behaviours with genuinely different conditions:

  - A nil-error `Seed` clears a pre-populated friction directory, then creates it.
  - A `loomshed.ErrSeedExists` re-entry leaves existing notes **untouched** but still ensures the directory exists.
    This is the crash-resume guarantee and is the one that must not regress.
  - An `os.MkdirAll` failure leaves the run's own outcome unchanged: assert no error is returned and that the seed path still completes.
  - A `drive` invocation against a location whose friction directory is absent creates it before the run starts.

  Cover the reflection call site at the seam rather than through a real Shed: assert that the `RunPaused` outcome and the non-nil-`err` path never call the reflection seam at all, and that `RunDone` and `RunBlocked` both do.
  Assert the envelope's `friction` key is present for `RunDone` and `RunBlocked`, that its value is one of `"skipped"` / `"reflected"` / `"failed"` and **never** `"filed"`, and that a reflection failure leaves the envelope's `outcome` key untouched.
  If `drive.go`'s existing test surface cannot reach that path without a real Shed, assert the seam instead — never widen the test tier to reach it.

  **`internal/webstercli/wiring_test.go`.**
  Assert that `wireHub` resolves a non-empty friction directory when loom's config enables Tier 2, resolves `""` when it is present-but-empty, and resolves `""` **without returning an error** when `loom.yaml` is absent or unparseable — that last case is the tolerance this CLI's whole resolution depends on.
  Assert `wireStandalone` always resolves `""`.
- **Commit:** `test(loomcli,webstercli): cover friction wiring, the clear/create split, and the drive envelope`

## Batch Tests

`verify: go test ./internal/loomcli/... ./internal/webstercli/... ./internal/burlercli/...` covers the three CLI packages this batch writes to.

`./internal/loomcli/...` runs the extended `wiring_test.go`, the new `friction_test.go` (the clear-on-first-seed-only split, the `ErrSeedExists` crash-resume guarantee, the `MkdirAll`-failure tolerance, drive's own ensure, the four reflection-trigger branches, and the envelope key), and that package's existing untagged tests — which are what catch the `run.go:101` restructure breaking the seed path, the single riskiest edit in this batch.

`./internal/webstercli/...` runs the extended `wiring_test.go` covering the hub-mode resolution, its tolerance of an absent or unparseable `loom.yaml`, and standalone's unconditional empty value.

`./internal/burlercli/...` is included as a cheap regression guard: batch 5 changed `burlerengine.New`'s signature and this batch changes `internal/loomcli/wiring.go`'s argument to it, so running the third caller's package confirms the three call sites stayed consistent.

The `smoke`-tagged files in `internal/loomcli` are not run by this command and are not extended by this batch;
the repo-wide and tagged-suite confirmation is `pipeline.done_gate`'s job, which this hub already sets to `go test ./... && go test -tags integration ./...`.
