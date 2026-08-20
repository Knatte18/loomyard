# Batch: delete-composite

```yaml
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
batch: 'delete-composite'
number: 4
cards: 8
verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
depends-on: [3]
```

## Batch Scope

This batch deletes the composite that the split replaced — `loomengine.Preflight`, `checkResolved`, `runCheck4` and the `CheckResolvedForTest` export shim — and settles everything that named it.
Card order matters here and is not arbitrary: the two coverage-migration cards run **first**, so the Tier-2 cases retiring from `internal/loomengine` are already living in `internal/preflight` before the file that holds them is deleted, and the deletions run before the doc-comment sweep so the sweep is written against the post-deletion tree.

The batch also carries the two doc-defect classes the discussion enumerated as closed greps: `internal/loomengine`'s own in-package prose (the package doc in `status.go`, the file header and four constant docs in `report.go`) and the four `internal/fabricengine` production comments naming the deleted symbol as the caller they exist for.
The five tier-1/tier-2 check-ID const aliases go with it — their stated purpose was letting existing callers of those names keep compiling, and after this batch that population is empty.

Batch-local decision: `internal/loomengine/testmain_test.go` is deleted alongside the integration suite it exists for.
It is untagged and its only reason to exist is `gitkit.HermeticGitEnv()` for that file's `hubforge` fixtures; once they are gone, no test in the package spawns git and the `TestMain` names a thing that no longer exists.

## Cards

### Card 21: migrate the missing tier-1/tier-2 sub-cases

- **Context:**
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/preflight/preflight.go`
  - `internal/preflight/report.go`
  - `internal/fabricengine/doc.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/preflight/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two table tests in this file carry strictly fewer sub-cases than their `internal/loomengine` counterparts, and the counterparts are about to be deleted — see the `tier-2-coverage-is-migrated-not-assumed-equivalent` Shared Decision.
  Bring both up to the union.
  (1) `TestCheckResolved_Dirty` has two sub-cases today, `WarpSide` and `PairedSide`, both untracked-only.
  Add the three shapes it lacks, taken from `TestPreflight_WarpDirty`: a tracked-and-modified file on the resolving side, a staged file on the resolving side, and a both-sides-dirty case.
  Each still asserts exactly `preflight.CheckWorktreeClean` and nothing else.
  Update the test's doc comment so it states it covers all three ways cleanliness can observe a dirty repo across both sides of the pair.
  (2) `TestCheckResolved_BrokenJunction` covers one junction (`_lyx`, via `fabricengine.WarpLyxLinkHere`) in one shape (removed).
  Restructure it into the two-dimensional matrix `TestPreflight_JunctionBroken` carries: three drift shapes — missing, not-a-link, and points-elsewhere — crossed with two junctions, the lyx one and the second, non-lyx one the fixture already wires as `_extra`.
  Every one of the six cases asserts exactly `preflight.CheckJunction`, never `preflight.CheckFabricSync` — the point is that `fabricengine.Healthy`'s typed `Cause` classification holds for the second junction too, not just the one its loop was originally written against.
  Drop every seed-related expectation from the migrated comments and assertions: `internal/preflight` has no notion of loom's status file, so the asymmetry the source test's doc comment describes (a broken lyx junction also failing the seed stat, a broken second junction not) has no counterpart here and must not be carried over.
  Resolve the second junction's path the way the source test does, from `fabricengine.WorktreePath` joined with the location's own anchor-relative path and the junction name; the fixture returns the slug alongside the hub for exactly this.
  Reuse the file's existing `setupFixture` and `assertCheckSet` helpers unchanged — do not add a second fixture builder.
- **Commit:** `test(preflight): migrate the cleanliness and junction sub-cases from loomengine`

### Card 22: migrate the two orphan tier-1/tier-2 cases

- **Context:**
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/preflight/preflight.go`
  - `internal/preflight/report.go`
  - `internal/loomengine/report.go`
  - `internal/configengine/config.go`
  - `internal/fabricengine/doc.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/preflight/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two tests in the retiring `internal/loomengine` suite have no counterpart at all in this file, and both assert tier-1/tier-2 behaviour that belongs here.
  Migrate them rather than deleting them.
  (1) `TestPreflight_ConfigLoadFailed` becomes `TestCheckResolved_ConfigLoadFailed`: corrupt the repo-wide fabric config resolved via `configengine.ConfigFile(fabricengine.BoardDir(h.Location.HubPath), "fabric")` with unparseable YAML and assert `preflight.CheckResolved` reports exactly `preflight.CheckJunction`.
  Its doc comment keeps the point that a config-load failure classifies as junction rather than a distinct check ID of its own, and drops the sentence about the seed check being unaffected.
  (2) `TestPreflight_MissingOptionalJunctionIsAJunctionFault` becomes `TestCheckResolved_MissingOptionalJunctionIsAJunctionFault`: remove the second, non-lyx junction from an otherwise-healthy fixture, assert exactly `preflight.CheckJunction`, then drive one `fabricengine.NewTopology(fabricengine.Config{}).Reconcile(h.Location)` and assert the pair for this worktree reports `fabricengine.ReconcileActionJunctionRepointed` with an empty error, that the junction resolves again via `fslink.IsLink`, and that a fresh `preflight.CheckResolved` then reports OK.
  Keep every assertion of the source test except the seed expectations.
  Add the `internal/configengine` import in its sorted position; `internal/fslink` and `internal/fabricengine` are already imported.
  Retarget both tests from `loomengine.CheckResolvedForTest(h.Location)` to `preflight.CheckResolved(h.Location)` and from `loomengine.CheckID` constants to their `preflight` originals — the two are the identical type today only because `internal/loomengine/report.go` aliases them, and that alias set is deleted in card 25.
- **Commit:** `test(preflight): migrate the config-load and missing-junction cases from loomengine`

### Card 23: retire loomengine's Tier-2 suite

- **Context:**
  - `internal/preflight/preflight_integration_test.go`
  - `internal/loomengine/seed_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/loomengine/testmain_test.go`
- **Moves:** none
- **Requirements:** Delete `internal/loomengine/preflight_integration_test.go` outright.
  Its disposition is settled and complete: eight of its tests already had direct counterparts in `internal/preflight/preflight_integration_test.go`; the sub-cases those counterparts lacked were migrated by card 21; its two orphans were migrated by card 22; and its three seed tests (`TestPreflight_SeedMissing`, `TestPreflight_SeedUnknownField`, `TestPreflight_SeedHalfFinished`) were replaced at Tier 1 by `internal/loomengine/seed_test.go` in batch 2.
  Nothing in it is dropped without a home.
  Before deleting, re-read the file once and confirm that claim against the current contents rather than against this card's list — if a test is found with neither a counterpart nor a migration, stop and report it rather than deleting it.
  Then delete `internal/loomengine/testmain_test.go`.
  It is untagged and exists solely to run `gitkit.HermeticGitEnv()` before that suite's `hubforge` fixtures spawn git; with the suite gone, no test file in `internal/loomengine` spawns git at all, and its own doc comment would name integration tests the package no longer has.
  Confirm that by grepping the package's remaining `_test.go` files for the git-spawning tokens the Hermetic Git Test Environment Invariant names before deleting.
  Do not delete any other test file in `internal/loomengine` — `coherence_test.go`, `seed_test.go`, `config_test.go`, `discussion_test.go`, `discussionpath_test.go`, `loomstatus_test.go`, `plan_test.go` and `prompt_test.go` all stay.
- **Commit:** `test(loomengine): retire the Tier-2 preflight suite in favour of Tier-1 seed tests`

### Card 24: delete the composite

- **Context:**
  - `internal/loomengine/seed.go`
  - `internal/loomengine/coherence.go`
  - `internal/loomshed/loompreflight.go`
  - `internal/loomcli/wiring.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/export_test.go`
- **Moves:** none
- **Requirements:** Delete `internal/loomengine/preflight.go` in full — `Preflight`, `checkResolved` and `runCheck4` all go, along with the file's header comment describing the composite.
  Delete `internal/loomengine/export_test.go`, whose only content is the `CheckResolvedForTest = checkResolved` shim for a suite card 23 already retired.
  No production caller survives: `internal/loomcli` reaches preflight only through the producer constructor, which batch 3 repointed at `internal/preflightshed`, and loom's own seed row calls `CheckSeed` directly.
  Do not keep a thin deprecated composite forwarding to `preflight.Check` plus `CheckSeed`: a composite carrying the old bundled semantics with no consumer is a second, divergent path.
  After deleting, confirm `internal/loomengine` no longer imports `internal/preflight` anywhere except `report.go`'s three type aliases, and that `internal/lyxcwd` is still imported only by `config.go`'s path accessors.
- **Commit:** `refactor(loomengine): delete the Preflight composite and its export shim`

### Card 25: report.go — drop the aliases, narrow the docs

- **Context:**
  - `internal/loomengine/seed.go`
  - `internal/loomengine/coherence.go`
  - `internal/preflight/report.go`
  - `internal/shedengine/run.go`
  - `contracts/specs/loom-status-spec.md`
- **Edits:**
  - `internal/loomengine/report.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the five tier-1/tier-2 check-ID **constant** aliases — `CheckGeometry`, `CheckWorktreeClean`, `CheckFabricReady`, `CheckFabricSync`, `CheckJunction` — together with the block comment above them, which states their entire purpose as letting "existing callers of these names keep compiling unchanged".
  After this task that population is empty: card 22 repointed the two migrated tests at the `preflight` originals, card 23 deleted the only other test naming them, and card 27 repoints the smoke suite.
  Keep the three **type** aliases (`CheckID`, `Failure`, `Report`) exactly as they are — `CheckSeed` returns `Report`, and the alias is what keeps `loomengine.Report` and `preflight.Report` the identical type across the package boundary.
  Keep all four loom-specific constants; they are what `CheckSeed` reports.
  Rewrite the file header comment, which describes the deleted `Preflight` and "the four preconditions": it now says the file re-exposes the three result types as aliases and declares the four loom-specific check-ID constants `CheckSeed` reports against.
  Edit three of the four constant doc comments.
  `CheckSeedMissing` **loses** its now-stale "and fabric is otherwise ready and healthy" clause — dropping the short-circuit made the not-exist verdict unconditional, so that qualifier describes a branch the code no longer has — and **gains** the unreachable-through-Shed note.
  `CheckSeedUnreadable` **loses** its second clause, "or when a stat failure (including not-exist) is attributable to fabric not being ready or healthy rather than a genuinely missing seed", which described the removed branch exactly, narrowing it to genuine stat/read failures other than not-existing; it gains **no** unreachability note, because its remaining branch turns on a filesystem state change no gate can pre-empt.
  `CheckSeedIncoherent` gains the same unreachable-through-Shed note, since two of its three producing branches are pre-empted.
  Each note is one clause referring the reader to the step-1 pre-emption rule stated in full on `CheckSeed`, never a restatement of it — the rule is stated once, generally, and per-branch restatement is what left neighbouring branches undisposed before.
  `CheckHalfFinished`'s doc changes only where it names `Preflight` as the gate; it now names loom's own seed row.
- **Commit:** `docs(loomengine): drop the tier-1/2 check-ID aliases and narrow the seed docs`

### Card 26: rewrite the package doc

- **Context:**
  - `internal/loomengine/seed.go`
  - `internal/loomengine/report.go`
  - `internal/preflight/doc.go`
  - `contracts/specs/loom-status-spec.md`
- **Edits:**
  - `internal/loomengine/status.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The `package loomengine` doc comment lives in this file, above the package clause, and every sentence of it is now false.
  It claims the package "implements loom's `Preflight` precondition validator: the four checks (worktree geometry, worktree cleanliness, fabric readiness/sync, and status.json coherence) that must all pass before a task is fit to run", then that "Callers MUST NOT invoke `Preflight` except when the task is at the fresh/preflight stage", then that "`Preflight` is a stateless validator".
  Rewrite it around `CheckSeed`: the package implements loom's own seed-coherence check — one of the four preconditions a task must meet, with the other three now `internal/preflight`'s orchestrator-agnostic tier-1/tier-2 checks — over told absolute paths, reporting a determined verdict rather than erroring on anything short of an infra failure.
  Carry forward the surviving half of the caller-obligation paragraph, retargeted: invoking the seed check on an already-advanced task is a caller error reported as a half-finished precondition failure rather than diagnosed as misuse, because the check is a stateless validator.
  Drop every reference to the deleted symbol name.
  This file was missed by the discussion's first sweep because that sweep grepped the qualified spelling `loomengine.Preflight`, which is structurally blind to in-package references.
  Close that gap now: grep `internal/loomengine`'s own remaining production files for the unqualified word `Preflight` and dispose of every hit, in this card, rather than only the ones listed here.
  Leave the `Status` type and its own doc comment below untouched — they describe loom's product payload and are unaffected.
- **Commit:** `docs(loomengine): rewrite the package doc around CheckSeed`

### Card 27: repoint the smoke suite

- **Context:**
  - `internal/preflight/preflight.go`
  - `internal/preflight/report.go`
  - `internal/loomshed/stub.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two changes, both in a `//go:build smoke` file that neither `go test` invocation compiles — this card is why the batch's third verify command exists.
  (1) In `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit`, replace the `loomengine.Preflight(worktree)` call with `preflight.Check(worktree)`, which returns three values; discard the `*lyxcwd.Location` with `_`.
  Change the assertion's check ID from `loomengine.CheckWorktreeClean` to `preflight.CheckWorktreeClean` and the failure-message text accordingly.
  Repointing at the tier-2 function rather than at the row-1 producer is deliberate and must stay that way: the assertion is about a specific check ID, and the producer collapses the whole report to `Done`/`Stuck`, which would lose exactly the discrimination this test exists for.
  Add the `internal/preflight` import in its sorted position; keep the `internal/loomengine` import, which the file still needs for `loomengine.LoomStatusRel()` and the status-file reads elsewhere.
  (2) In the file-level doc comment, fix the driver-liveness note's row counts: it says loom's producer table "backs eight of its thirteen rows with stub producers", which is doubly wrong — `internal/loomshed/stub.go` records five stubbed rows, and the list is thirteen only after this task.
  It becomes five of thirteen.
  Do not change any other assertion or helper in this file.
- **Commit:** `test(loomcli): repoint the smoke cleanliness assertion at preflight.Check`

### Card 28: sweep the fabricengine comments

- **Context:**
  - `internal/preflight/preflight.go`
  - `internal/preflight/doc.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/warpclean.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Four production comments name the deleted symbol as the caller they exist for.
  Repoint each at `preflight.CheckResolved`, which is the half that actually does that work now — all four describe tier-2 classification, not seed coherence.
  The four sites are: `internal/fabricengine/doc.go`'s "a caller like `loomengine.Preflight` switches on `HealthReason.Cause`"; `internal/fabricengine/drift.go`'s "letting a caller like `loomengine.Preflight` classify the failure without parsing a display string"; and two in `internal/fabricengine/warpclean.go`, its file header's "by `loomengine.Preflight` to determine whether both sides ... have any dirty" and its later "It is package-level for use by `loomengine.Preflight`".
  This enumeration is closed, not sampled — it is the full result of a repo-wide grep for the symbol across non-test Go files, re-run and confirmed while planning.
  Re-run that grep after editing and confirm zero production hits remain anywhere outside `manifest/` and `contracts/`, which batch 5 handles.
  Change only the symbol name and whatever minimal wording the substitution requires; do not restructure these comments or touch any code in these three files.
- **Commit:** `docs(fabricengine): repoint the four preflight-caller comments at preflight.CheckResolved`

## Batch Tests

Verified by the batch's three-command `verify:` chain, and this is the batch where all three commands are individually load-bearing.

Tier 1 proves the deletions did not take anything live with them: `internal/loomengine`'s remaining untagged suite (`coherence_test.go`, `seed_test.go`, and the six unrelated files) must still compile and pass with `preflight.go`, `export_test.go`, `testmain_test.go` and the five const aliases gone, and `internal/loomshed`'s and `internal/loomcli`'s untagged suites must be untouched by them.
`cmd/lyx/hermeticenv_test.go` is the specific guard on card 23's second deletion — if any remaining `internal/loomengine` test still carried a git-spawning token, removing the `TestMain` would fail it there rather than silently.

Tier 2 is where cards 21 and 22 actually run, against real `hubforge` hubs: the three added cleanliness sub-cases, the six-case junction matrix, and the two migrated orphan tests — including the `Reconcile`-repairs-and-then-passes sequence, which is the only test in the repo asserting that remedy end to end.
It is also what proves card 23's deletion lost no coverage: if a migrated case had been transcribed wrong, it fails here rather than in the deleted file.

`go vet -tags smoke ./internal/loomcli` is the gate for card 27 and for card 24 jointly.
Neither `go test` invocation compiles `//go:build smoke` files, so without this command the batch would report green over a smoke suite whose line 641 calls a symbol card 24 just deleted.
