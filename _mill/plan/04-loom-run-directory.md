# Batch: loom-run-directory

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: loom-run-directory
number: 4
cards: 4
verify: go build ./... && go test ./internal/loomengine/... ./internal/loomcli/...
depends-on: [1]
```

## Batch Scope

This batch moves loom's own run state onto the unified run directory: `_lyx/loom/status.json` becomes `_lyx/shed/self/status.json`, its two locks move to the mirrored `.lyx/shed/self/`, `lyx loom start` gains a seed write beside the status seed it already performs, and all four generic loom verbs gain the seed-presence refusal and the `MaximumNArgs(1)` run-id positional.
It is one batch because the four changes share one file set — `internal/loomengine/config.go`, `internal/loomcli/wiring.go`, `arm.go`, `start.go`, `sharedbootstrap.go`, `landingdeps.go` — and because splitting the path deletion from its call-site retargeting would leave an intermediate commit that does not compile.

The external interface batch 7 consumes: `loomcli.Arm` still exported with today's signature (batch 7 changes it), now refusing when `_lyx/shed/<run-id>/seed.json` is absent, and loom's verbs accepting an optional run-id positional.

Batch-local decision beyond the overview's: **loom's `run` and `step` do not auto-seed.**
All four generic verbs require a seed and refuse with the run-id listing when absent; only `lyx loom start` writes one.
This is loom's existing only-`start`-may-seed discipline — stated in today's `battenPreRun` doc comment about loom refusing where batten seeds — extended unchanged from the status file to the seed, and it is the asymmetry against batten most likely to be "fixed" into symmetry by mistake.
A status file present with no seed beside it takes the same refusal, whose remedy is re-running `lyx loom start`.

## Cards

### Card 12: relocate loom's status and run-lock paths onto shedrun

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/shedrun/runid.go`
  - `internal/fabricengine/commitweftpaths.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/config_test.go`
  - `internal/loomengine/loomstatus_test.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/landingdeps.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `LoomStatusRel`, `LoomStatusFile`, `LoomStatusLock` and `LoomRunLock` from `internal/loomengine/config.go`, together with the `loomStatusFileName` constant they share.
  Keep the `loomDirName` constant and the seven accessors that still need it — `LoomDriverLog`, `LoomBootstrapLock`, `LoomSelfreportFiled`, `LoomSelfreportFiledLock`, `LoomStepHandoff`, `LoomStepHandoffLock`, `LoomScratchDir` — per the overview's `loomDirName-survives-the-status-relocation` Shared Decision, and amend `loomDirName`'s own doc comment so it no longer claims to scope loom's status file.
  Amend `LoomScratchDir`'s doc comment where it names `LoomRunLock` as one of the paths it sits beside.
  Retarget every call site: `internal/loomcli/wiring.go`'s two `ShedPaths` literals (lines filling `StatusPath`, `LockPath`, `StatusLockPath` in both `wireLightweight` and `wire`) and its `statusPath`/`statusLockPath` locals become `shedrun.StatusFile(location, shedrun.SelfRunID)`, `shedrun.RunLock(location, shedrun.SelfRunID)` and `shedrun.StatusLock(location, shedrun.SelfRunID)`; `loomCommitStatusDeps`'s `fabricengine.CommitAnchoredPaths` pathspec becomes `[]string{shedrun.StatusRel(shedrun.SelfRunID)}`; `internal/loomcli/sharedbootstrap.go`'s `commitPaths` first element and `internal/loomcli/landingdeps.go`'s single-element pathspec take the same replacement, and `landingdeps.go`'s comment naming `loomengine.LoomStatusRel()` is updated with it.
  Do not change `landingdeps.go`'s `ScratchDir: loomengine.LoomScratchDir(l)` — that is loom's own scratch tree under `.lyx/loom/`, a different directory from `shedrun.ScratchDir`'s run directory, and it stays.
  Update the three test files listed so their pinned path expectations name the new locations.
- **Commit:** `refactor(loomcli,loomengine): move loom's status and run lock onto the shed run directory`

### Card 13: lyx loom start writes and commits the seed

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/paths.go`
  - `internal/loomcli/start.go`
  - `internal/fabricengine/commitweftpaths.go`
- **Edits:**
  - `internal/loomcli/sharedbootstrap.go`
  - `internal/loomcli/sharedbootstrap_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `seedAndCommitBootstrap`, after step 1 has resolved `parent` and before step 2 seeds the status file, write loom's own seed: call `shedrun.WriteSeed(c.location, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo, Params: map[string]string{"parent": parent}})`.
  Placing it before the status seed matters: the seed is the run's identity and the status file must never exist without one beside it, which is the inconsistency card 14's refusal exists to catch.
  `WriteSeed` is already idempotent against a byte-identical seed, so a re-run needs no sentinel handling of its own; a disagreeing seed's refusal propagates as a returned error at `bootstrapStageSeed`.
  Add `shedrun.SeedRel(shedrun.SelfRunID)` to `commitPaths` in step 3, alongside the status-file and origin-record entries, for the same self-healing reason the comment there already gives for including the origin record unconditionally.
  The `params.parent` value is the run's *recorded startup choice*, not the durable truth: `fabricengine.Origin.ParentBranch` stays the durable record and `resolveParentBranch`'s existing disagreement refusal is reused unchanged.
  Do not touch `resolveParentBranch` or its tests — if they need changing, the parent decision has drifted.
  In `sharedbootstrap_test.go`, cover the seed being written with `recipe: "loom"` and `params.parent` matching the resolved parent, the re-run being idempotent, and the seed's relative path appearing in the committed pathspec.
- **Commit:** `feat(loomcli): write and commit loom's own seed during start bootstrap`

### Card 14: loom's seed-presence refusal, with no auto-seed

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/runid.go`
  - `internal/loomcli/wiring.go`
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/step_test.go`
- **Edits:**
  - `internal/loomcli/arm.go`
- **Creates:**
  - `internal/loomcli/arm_seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `loomcli`'s `arm`, after `lyxcwd.Resolve` succeeds and before either wiring call, resolve the run-id from `args` — `args[0]` when present, `shedrun.SelfRunID` otherwise — and apply the seed-presence check to all four generic verbs, `run` and `step` included.
  When `shedrun.ReadSeed` reports `found == false`, return an error listing every run-id `shedrun.List` finds, sorted, and naming `lyx loom start` as the remedy; an empty listing is the ordinary case for a worktree from before this task and must read as such rather than as a fault.
  Render that text through `shedrun.MissingSeedMessage`, the shared renderer batch 1 card 3 declares, so this path and batten's word the refusal identically.
  `lyx loom start` is not a generic verb and never reaches this check — it is the one site that writes a seed.
  Record the run-id on the receiver so card 15 and batch 7 can read it back, and carry it into the `shedrun.*` path calls card 12 introduced, replacing the hardcoded `shedrun.SelfRunID` there.
  The refusal must carry **no** `kind` field on its envelope: the `step` refusal-kind vocabulary is pinned closed at five values by `internal/shedverbs/step_test.go` and by `ly-drive`'s contract, and a missing run is not a sixth kind.
  In `arm_seed_test.go` cover each of the four verbs refusing when no seed is present, the refusal naming every existing run-id, a status file present with no seed taking the same refusal, and — the asymmetry that matters — `run` and `step` writing nothing to disk when they refuse.
- **Commit:** `feat(loomcli): refuse every generic verb with a run-id listing when no seed exists`

### Card 15: loom's verbs accept an optional run-id positional

- **Context:**
  - `internal/loomcli/arm.go`
  - `internal/shedcli/table.go`
  - `internal/shedrun/runid.go`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/parity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Set `cobra.MaximumNArgs(1)` on each of loom's four generic verbs — `run`, `step`, `status`, `pause` — in `internal/loomcli/cli.go`, where none declares an `Args` validator at all today.
  Without it a run-id typed at `lyx loom status <run-id>` is silently swallowed and addresses `self`, which is the worst of the available behaviours: `MaximumNArgs(1)` is the same arity every one of the three surfaces lands on, so there is no rule to remember about which subtree accepts a run-id.
  Update each verb's `Use` string to show the optional positional and its `Long` text's examples to mention addressing a named run, keeping every `Short` non-empty.
  Leave `lyx loom start`'s own arity alone — it takes no run-id.
  In `parity_test.go` assert each of the four verbs independently carries `MaximumNArgs(1)`, refusing two positionals and accepting both zero and one.
- **Commit:** `feat(loomcli): accept an optional run-id positional on loom's generic verbs`

## Batch Tests

`verify: go build ./... && go test ./internal/loomengine/... ./internal/loomcli/...` pairs a whole-module build with the two packages this batch changes.
The build half is required rather than decorative: card 12 deletes four exported `loomengine` functions, and any consumer outside these two trees that still calls one breaks compilation without failing either package's own tests.
`internal/loomcli`'s untagged tests are Tier 1 — `specFor` is called directly against a hand-populated receiver so the leaf-command tests never spawn git, and that shape is preserved by every card here.

Coverage lands across four files.
`internal/loomengine/config_test.go` and `loomstatus_test.go` pin the relocated status, status-lock and run-lock paths, and must also prove the seven surviving `loomDirName`-backed accessors are unmoved — that is what makes the overview's `loomDirName-survives-the-status-relocation` decision checkable rather than asserted.
`internal/loomcli/wiring_test.go` pins both `ShedPaths` literals and the commit pathspec.
`sharedbootstrap_test.go` covers the seed write, its `params.parent` value, its idempotency and its presence in the committed pathspec.
The new `arm_seed_test.go` carries the batch's highest-consequence assertions: all four verbs refusing without a seed, `run` and `step` **not** auto-seeding, and the refusal carrying no `kind` field.

`internal/loomcli`'s `smoke`-tagged files are deliberately out of this verify's scope — they spawn real processes, and the batch changes no spawn path.
