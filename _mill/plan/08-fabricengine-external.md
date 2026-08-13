# Batch: fabricengine external

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'fabricengine external'
number: 8
cards: 11
verify: go vet -tags integration ./... && go test -tags integration ./internal/fabricengine/...
depends-on: [7]
```

## Batch Scope

This batch migrates the thirty-seven `Copy*` sites and the eleven `NewPairedForTest` sites that already live in `package fabricengine_test` files inside `internal/fabricengine/`.
No file moves and no export-shim growth are needed here — every file is already external — which is what makes it the right half of `fabricengine`'s eighty-two sites to do first.
Batches 9 and 10 handle the forty-five in-package sites, which do need both.

`NewPairedForTest` is the second axis in this batch.
Its counts are call expressions, counted with `grep -o 'fabricengine\.NewPairedForTest('` — the same call-expression method the `Copy*` table uses, and for the same reason: a line-based count over-reports badly here, since `internal/fabricengine/export_test.go` declares the identifier and several `t.Fatalf` messages name it in string literals.
Eleven real call expressions exist: five in `warpforward_integration_test.go`, four in `weftgit_exclude_test.go`, one in `checkout_index_refresh_test.go`, and one in `fabric_test.go`.
Ten of them exist only because the old fixtures could not produce a genuine warp/weft pair — they bolt an unrelated `CopyWeft` weft onto a `CopyWarpHub` warp — and a real hub's `PrimeWorktree()`/`PrimeWeft()` **is** a genuine pair, so those ten go away.
The eleventh, in `fabric_test.go`, is not a fixture at all and stays;
the next paragraph is why.

Batch-local decision, and a deliberate deviation from the discussion's scope line: `NewPairedForTest` is **not** deleted outright.
`internal/fabricengine/fabric_test.go` is an untagged unit test of the `newPaired` constructor itself — it hands the shim two empty `os.Mkdir` directories and asserts the warp and weft fields come back non-nil.
That has nothing to do with hub fixtures, spawns no git, and would be lost coverage if deleted, and it cannot move onto `hubforge.NewHub` because the Test Tier Purity Invariant bans an untagged test from calling it.
The shim therefore survives, renamed to `NewPairedFromPathsForTest` and documented as a constructor seam rather than a fixture — which is what card 51 does.
The discussion's "delete `NewPairedForTest`" line was derived from the ten fixture-pairing sites and did not account for this one;
batch 11's grep gate is written against the fixture usage accordingly.

## Cards

### Card 42: Migrate dotlyxjunction_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the five `gitkit.CopyPaired(t)` calls with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the five `gitkit.SeedConfig(` calls.
  This file's subject is the hub-level `.lyx` directory and the `_lyx` junction, so it is the file most likely to have been asserting against the invented shape: the old fixture had no hub-level `.lyx` at all, and a real hub has one as a real directory that is never a link.
  Every assertion that constructs a junction path by hand must be re-expressed through `fabricengine`'s own name accessors.
- **Commit:** `test(fabricengine): build the .lyx junction fixtures with hubforge.NewHub`

### Card 43: Migrate junction_pattern_integration_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the seven `gitkit.CopyPairedLocal(t)` calls with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the five `gitkit.SeedConfig(` calls.
  These five seeds configure junction patterns, so they are override seeds rather than template seeds and almost certainly become `hubforge.SeedConfig(t, h, …)` rather than deletions — but apply the triage per site rather than assuming.
  A real hub arrives with the repo-wide `fabric.yaml` already committed at `BoardDir`, so any site here seeding **repo-wide** fabric config rather than a module config uses `hubforge.SeedFabricConfig` instead.
  The five `gitkit.MustRun(` calls stay on `gitkit` unchanged.
- **Commit:** `test(fabricengine): build the junction-pattern fixtures with hubforge.NewHub`

### Card 44: Migrate junction_repoint_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/fabricengine/junction_repoint_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the four `gitkit.CopyPairedLocal(t)` calls with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the four `gitkit.SeedConfig(` calls.
  This file repoints junctions deliberately, which is one of the two hostile shapes `hubforge`'s teardown is tested against in card 18 — so a repointed junction left behind at test end is removed by teardown without failing the test, and nothing here needs to restore state before returning.
- **Commit:** `test(fabricengine): build the junction-repoint fixtures with hubforge.NewHub`

### Card 45: Migrate add_rollback_adopt_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/add_rollback_adopt_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the three `gitkit.CopyPairedLocal(t)` calls with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the three `gitkit.SeedConfig(` calls.
  The fifteen `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  This file drives `Add` rollback and adoption, so it plants pre-existing branches and worktrees;
  where it did so relative to the old fixture's flat `Hub` directory, re-express those paths through `h.PairWarpWorktree(slug)` and `h.PairWeftSibling(slug)` rather than by string concatenation.
- **Commit:** `test(fabricengine): build the add-rollback fixtures with hubforge.NewHub`

### Card 46: Migrate cleanreason and config-driven-junctions

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/cleanreason_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the three `gitkit.CopyPairedLocal(t)` calls in each file with `hubforge.NewHub(t, ".")` and retarget the fixture fields per the overview's mapping table.
  `internal/fabricengine/config_driven_junctions_integration_test.go` drives junction wiring off configuration;
  if it seeds that configuration through a mechanism other than `gitkit.SeedConfig` — a hand-written file under the old fixture's `_lyx` — replace that scaffolding with `hubforge.SeedConfig` or `hubforge.SeedFabricConfig` as the base dictates, rather than porting the hand-written path onto the real hub.
  The one `gitkit.MustRun(` call in that file stays on `gitkit` unchanged.
- **Commit:** `test(fabricengine): build the cleanreason and config-junction fixtures with hubforge.NewHub`

### Card 47: Migrate destructivegaps and the two reconcile-stale suites

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/destructivegaps_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the one `gitkit.CopyPairedLocal(t)` call in each of the three files with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the one `gitkit.SeedConfig(` call in `internal/fabricengine/destructivegaps_integration_test.go` and the one in `internal/fabricengine/reconcile_stale_registration_test.go`.
  The `gitkit.MustRun(` calls in all three files stay on `gitkit` unchanged.
  `internal/fabricengine/destructivegaps_integration_test.go` exercises the destruction gate's path-ownership kinds;
  three of those kinds (`ownedWiredJunction`, `ownedDriftedWiredJunction`, `ownedUnderGeometryRoot`) were structurally unreachable on the old fixture because it had no junctions, so this file may now be able to assert things it previously could not.
  Do not add that coverage here — note it in the commit message as available follow-up.
- **Commit:** `test(fabricengine): build the destructive-gap and reconcile-stale fixtures with hubforge.NewHub`

### Card 48: Migrate open and ready suites

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabricengine/open_integration_test.go`
  - `internal/fabricengine/ready_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the three `gitkit.CopyPaired(t)` calls in `internal/fabricengine/open_integration_test.go` and the two in `internal/fabricengine/ready_integration_test.go` with `hubforge.NewHub(t, ".")`, retargeting the fixture fields per the overview's mapping table.
  Neither file seeds config.
- **Commit:** `test(fabricengine): build the open and ready fixtures with hubforge.NewHub`

### Card 49: Migrate unwire and worktreelist suites

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/worktreelist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the one `gitkit.CopyWarpHub(t)` call in each file with `hubforge.NewHub(t, ".")` and retarget the `.Hub` field read per the overview's mapping table.
  These two are the external half of the twelve hub-shaped `CopyWarpHub` sites: the old fixture's field named `Hub` held a directory that was not a hub, which is exactly the invented shape this task removes, so read what each test does with the path before choosing between `h.PrimeWorktree()` and `h.Path`.
  `internal/fabricengine/unwire_test.go` drives `Unwire`, which is per-warp-worktree and deliberately partial;
  on a real hub it now has genuine junctions to unwire, so assertions that previously observed a no-op will change.
  The two `gitkit.MustRun(` calls in `internal/fabricengine/worktreelist_test.go` stay on `gitkit` unchanged.
- **Commit:** `test(fabricengine): build the unwire and worktreelist fixtures with hubforge.NewHub`

### Card 50: Migrate weftgit_exclude_test.go off CopyWeft and the pairing shim

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/weftgit_exclude_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the two `gitkit.CopyWeft(t)` calls with `hubforge.NewHub(t, ".")` and the four `fabricengine.NewPairedForTest(` calls with the hub's own genuine pair — `h.PrimeWorktree()` and `h.PrimeWeft()` are a real warp/weft pair, so the `Fabric` under test is obtained through `fabricengine`'s ordinary exported constructor against `h.Location` rather than through the shim.
  Delete this file's stand-in-hub scaffolding: its header comment records that it seeds config "at warpPath's parent directory (this fixture's stand-in Hub)", and that seeding goes away entirely, because a real hub materializes config at `BoardDir` and `WeftBase` for real.
  Update the header comment to say what the file now does instead of describing the removed hack.
  The five `gitkit.MustRun(` calls stay on `gitkit` unchanged, and the `gitkit.GitStatusPorcelain` call batch 2 introduced stays as it is.
- **Commit:** `test(fabricengine): drop weftgit_exclude's stand-in hub for a real one`

### Card 51: Retire the pairing shim in warpforward and checkout-index-refresh

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/warpforward_integration_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/fabric_test.go`
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the five `fabricengine.NewPairedForTest(` call expressions in `internal/fabricengine/warpforward_integration_test.go` and the one in `internal/fabricengine/checkout_index_refresh_test.go` with a `Fabric` built against `hubforge.NewHub(t, ".")`'s genuine pair, adding the `internal/hubforge` import to each and dropping any fixture-bolting helper that existed only to pair an unrelated weft with a warp.
  The `gitkit.MustRun(` calls in both files stay on `gitkit` unchanged.
  In `internal/fabricengine/export_test.go`, rename `NewPairedForTest` to `NewPairedFromPathsForTest` and rewrite its doc comment: it is no longer a fixture-pairing shim — its one remaining consumer is `internal/fabricengine/fabric_test.go`'s untagged unit test of the `newPaired` constructor, which hands it two empty directories and asserts the warp and weft fields come back non-nil.
  Say in the comment that it must not be used to assemble a fixture pair, and that a test needing a real pair takes one from `internal/hubforge`.
  In `internal/fabricengine/fabric_test.go`, retarget all four of its textual references onto the new name — one call expression plus three `t.Fatalf`/`t.Error` message strings naming the old identifier;
  the file stays untagged and must not gain a `hubforge` import, because the Test Tier Purity Invariant bans an untagged test from calling `hubforge.NewHub`.
- **Commit:** `test(fabricengine): retire the fixture-pairing shim for real hub pairs`

### Card 52: Confirm fabricengine's external files hold no stand-in hub

- **Context:**
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/add_rollback_adopt_test.go`
  - `internal/fabricengine/cleanreason_integration_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/destructivegaps_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/open_integration_test.go`
  - `internal/fabricengine/ready_integration_test.go`
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/worktreelist_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/warpforward_integration_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/fabric_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification-only gate, no diff.
  Confirm that every `package fabricengine_test` file in `internal/fabricengine/` contains zero occurrences of `gitkit.CopyPaired`, `gitkit.CopyPairedLocal`, `gitkit.CopyWeft` and `gitkit.CopyWarpHub`, and that the only surviving reference to the renamed pairing shim is `internal/fabricengine/fabric_test.go`'s untagged constructor unit test.
  Confirm no assertion was deleted rather than re-expressed without a stated reason: cross-read this batch's twelve commit messages against the diff and make sure each removal is accounted for.
  If any check fails, fix it under the card that owns the file rather than here.
- **Commit:** none

## Batch Tests

`verify:` compile-checks the repo under `-tags integration` and runs `internal/fabricengine`'s integration suite in full.
Every file this batch touches is integration-tagged except `internal/fabricengine/fabric_test.go`, which the same command covers as part of the package's untagged tests, so the whole batch gets runtime proof.

The full-package scope is justified rather than lazy: the thirty-seven migrated sites are spread across thirteen files in one package, the package's own live-state matrix (relocated in batch 2) shares the same test binary, and a `-run` filter narrow enough to matter would have to name most of the suite anyway.
