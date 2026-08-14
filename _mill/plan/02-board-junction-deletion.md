# Batch: board-junction-deletion

```yaml
task: "Move <hub>/.lyx into <hub>/_board"
batch: "board-junction-deletion"
number: 2
cards: 13
verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [1]
```

## Rename mechanic

For the one `Moves:` pair in this batch the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (the file header, the surviving test's name, and the deletion of the cases that go).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch deletes the `_board` convenience junction in full: the wiring function and its three call sites, the unwiring function and its two call sites, the `UnwireVerbResult` field, the CLI envelope key, and the board root in `linkIsFabricOwned`'s ownership predicate.
It preserves the two contracts that would otherwise be lost as collateral — the `filterHubReserved` wiring guard (re-homed from the deleted integration file) and the CLI `"refusal"` envelope object (re-homed from the deleted unwire test onto the portal junction, driven through `fabric remove`) — and adds the regression coverage that gives the new Hub Containment Invariant teeth across clone, add and reconcile.
Every prose and scenario surface naming the junction is updated in the same batch, per the Documentation Lifecycle.

Nothing in this batch migrates existing state: no sweeper removes a `_board` junction an older binary wired, and nothing unseeds a leftover `_board` line from a warp repo's `.git/info/exclude`.
Both were verified absent on every hub on disk, and cleanup code for a state that exists nowhere is dead code from birth.

## Cards

### Card 13: Delete `wireBoardLink`

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/gitexclude.go`
  - `internal/fslink/fslink.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `wireBoardLink` from `internal/fabricengine/junction.go` in full, including its doc comment beginning "wireBoardLink creates or repairs the operator-convenience `_board` junction".
  Its standalone `seedGitExclude(rec, l, slug, []string{BoardDirName})` tail goes with it — `seedGitExclude` itself stays, since `WireJunctions` still calls it.
  Do not change `seedLyxJunction`, `repointLink`, `ownedDriftedWiredJunction`, `WireJunctions`, `WireJunctionsWith`, or `UnwireResult` in this card.
  The Fabric Destruction Chokepoint Invariant is unaffected here: `wireBoardLink` is a create-side function and its only destructive call was the `repointLink` re-point branch, which remains reachable from `seedLyxJunction`.
- **Commit:** `refactor(fabricengine): delete wireBoardLink`

### Card 14: Delete `wireBoardLink`'s three call sites

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/clone.go`, delete the `wireBoardLink(rec, l, filepath.Base(warpWorktreePath))` call, its `teardownHub` error branch, and the four-line comment above it beginning "Wire the operator-convenience `_board` junction as a named special case".
  The `l, err := lyxcwd.Resolve(primeCwd)` block above it stays — `weftBase` still needs it.
  In `internal/fabricengine/add.go`, delete the `(10c)` comment block and the `wireBoardLink(rec, l, slug)` call together with its `rollbackAdd` error branch, leaving `(10)`'s `WireJunctionsWith` immediately followed by `(11)`'s branch push.
  In `internal/fabricengine/reconcile.go`, delete the `wireBoardLink(rec, warpLayout, slug)` call and its `appendPrDetail(pr, "board junction wiring failed: …")` branch.
  Also correct the two comments in that file that name the deleted function: `reconcileWarpBinding`'s doc comment simile "like `wireBoardLink`'s board-junction repair, a binding backfill is a convenience that may never fail or downgrade a reconcile verdict" must state that property without the simile, and `appendPrDetail`'s own doc comment must drop "and `wireBoardLink`'s failure note in Reconcile above" from its list of callers.
  Do not touch `restorePortalAndLaunchers`, `applyStaleRemoval`, or the `seedWeftArtifactExcludes(boardDir)` best-effort call in `reconcileWarpBinding`.
- **Commit:** `refactor(fabricengine): stop wiring the _board junction at clone, add and reconcile`

### Card 15: Delete `unwireBoardLink` and the `BoardJunctionRemoved` result field

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/gitexclude.go`
  - `internal/fabricengine/destroy.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/unwire.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `unwireBoardLink` from `internal/fabricengine/unwire.go` in full, including its doc comment and its trailing `unseedGitExclude(rec, l, slug, []string{BoardDirName})` call.
  `unseedGitExclude` itself stays — `UnwireJunctions` still calls it.
  Delete the `BoardJunctionRemoved bool` field from `UnwireVerbResult` together with its explanatory comment block beginning "BoardJunctionRemoved reports whether the operator-convenience _board link was present and removed".
  In `Unwire`, delete the `boardRemoved, err := unwireBoardLink(rec, l, slug)` call, its error branch, and the `result.BoardJunctionRemoved = boardRemoved` assignment, plus the comment above the call beginning "Remove the operator-convenience `_board` junction as an explicitly named case".
  Rewrite `Unwire`'s own doc comment only where it is now false;
  its statement that the junction name-set is enumerated from a full on-disk scan becomes unconditionally true rather than true-except-for-`_board`.
  Removing this `removeLink` call site must not weaken the Fabric Destruction Chokepoint Invariant for the remaining callers: `removeLink`, `pathRequest`, `ownedWiredJunction` and `dirtinessNA` all stay, still reached from `unseedJunctionRecords` and `removePortal`.
- **Commit:** `refactor(fabricengine): delete unwireBoardLink and UnwireVerbResult.BoardJunctionRemoved`

### Card 16: Restore `Remove`'s natural link-sweep shape

- **Context:**
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/portals.go`
- **Edits:**
  - `internal/fabricengine/remove.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/remove.go`, delete the `boardRemoved, boardErr := unwireBoardLink(rec, l, slug)` call, its `surfaceRefusal` branch, and the `if boardErr == nil && boardRemoved { linksRemoved++ }` block, leaving `linksRemoved` set solely by `len(ownedNames)` from the anchored sweep.
  `RemoveResult.LinksRemoved` must stay correct after the deletion: with no separately-swept link left, the anchored sweep's count is the whole count.
  Update `Remove`'s doc comment where it enumerates what is torn down, and the file header if it names the board link.
  Do not change the sweep's own comment explaining why it reads the anchored directory and filters by ownership;
  that reasoning is unaffected.
- **Commit:** `refactor(fabricengine): drop the _board special case from Remove's link sweep`

### Card 17: Drop the `board_junction_removed` envelope key

- **Context:**
  - `internal/fabricengine/unwire.go`
  - `internal/fabriccli/envelope.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabriccli/unwire.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the `"board_junction_removed": res.BoardJunctionRemoved` entry from the unwire envelope map in `internal/fabriccli/unwire.go`.
  This is a CLI-observable contract change under the CLI / Cobra Invariant, deliberate and covered by the deletion of its only asserting test in card 21.
  Leave every other key in that envelope unchanged.
- **Commit:** `feat(fabriccli)!: remove board_junction_removed from the unwire envelope`

### Card 18: Narrow `linkIsFabricOwned` to the weft worktree alone

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/unwire.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `linkIsFabricOwned`, remove `BoardDir(l.HubPath)` from the root slice, leaving `WeftWorktreePath(l, slug)` as the only root a link may resolve inside to be claimed as fabric's.
  Rewrite the function's doc comment accordingly, and the matching sentence in `scanOnDiskJunctionNames`'s own doc comment — "A link is fabric-owned only when it resolves inside the paired weft worktree or onto the hub's board directory — the only two targets any fabric junction is ever created with."
  The justification is the Hub Containment Invariant: with no fabric junction pointing at the board any more, keeping that root would let the sweep claim and remove a link an operator hand-made pointing at `<hub>/_board`, which is exactly what the invariant says is never fabric's to claim.
  Do not change the `HubReservedNames()` skip set in `scanOnDiskJunctionNames` — `_board` stays a member, carrying the slug reservation and the `filterHubReserved` wiring guard, and the skip keeps the sweep off an operator's own checked-in `_board` entry.
  Do not change the unresolvable-link branch, which still returns `(false, nil)`.
- **Commit:** `refactor(fabricengine): drop the board root from linkIsFabricOwned`

### Card 19: Re-home the surviving pathspec-exclusion guard

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junctionnames_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/hubreservedroutes_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/fabricengine/boardjunction_integration_test.go` -> `internal/fabricengine/hubreservedroutes_integration_test.go`
- **Requirements:** Run `git mv internal/fabricengine/boardjunction_integration_test.go internal/fabricengine/hubreservedroutes_integration_test.go` first, then edit in place.
  Delete every test case in the moved file except `TestBoardJunction_ExcludedFromPathspecRoutes`, which is the `filterHubReserved` wiring guard that survives the junction's deletion.
  The five cases that go are `TestBoardJunction_WiredAtClone`, `TestBoardJunction_WiredAtAddAndSurvivesReconcileThenUnwireRemoves`, `TestBoardJunction_ReconcileRepairsOutsideHealthCheck`, `TestBoardJunction_ReconcileRepointsWrongTarget`, and `TestBoardJunction_AbsentUntilPairFullyWired`.
  Rename the survivor to `TestHubReserved_BoardExcludedFromPathspecRoutes` and rewrite its doc comment: it guards that `_board` appears in neither `WiredNames`' output nor `ScopedPathspec`'s output over the real loaded config, which is the half `junctionnames_test.go`'s `TestFilterHubReserved` covers only at unit level.
  Rewrite the file header entirely — it currently describes the junction's creation, repair and removal, all of which are gone.
  Keep the `//go:build integration` constraint and `package fabricengine_test`;
  the surviving case needs `newFabricFixture`, which lives in an integration-tagged file.
  Prune the import block to what the single surviving case uses, dropping `fslink`, `gitexec` and `lyxcwd` if nothing else references them.
  It adds no `TestMain` — the package shares the single one in `testmain_test.go`.
- **Commit:** `test(fabricengine): re-home the _board pathspec-exclusion guard off the junction file`

### Card 20: Correct the exact junction-count and walk-rationale assertions

- **Context:**
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/unwire.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/livestate_manifest_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/remove_junctions_integration_test.go`, change the expected `result.LinksRemoved` from 3 to 2 and update the failure message to name only `_lyx` and `.lyx`.
  Rewrite the comment above it — "Exactly the two wired junctions (`_lyx`, `.lyx`) plus the `_board` convenience link — an exact count, not merely non-zero: the `_board` link is removed on a separate path and contributes 1 on its own, so a non-zero assertion stayed green with the anchored sweep entirely disabled."
  The exactness still matters and the reason must be restated without the board link: with the special case gone, the anchored sweep is the sole contributor, so an exact count is what proves the sweep ran at all.
  This is a behaviour assertion, not prose — it fails without card 16.
  In `internal/fabricengine/livestate_manifest_test.go`, remove "warp/`_board` to the hub's `_board`" from `CaptureManifest`'s no-descend walk rationale, which enumerates fabric's wired junctions.
  The rule and the two surviving examples are unaffected.
- **Commit:** `test(fabricengine): drop the _board link from the exact junction count`

### Card 21: Preserve the CLI refusal-envelope contract on the portal junction

- **Context:**
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabriccli/envelope.go`
  - `internal/fabriccli/fabric.go`
  - `internal/hubforge/hub.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete `TestRunCLI_Unwire_ReportsBoardJunctionRemoval` and its doc comment from `internal/fabriccli/cli_test.go` outright — its entire subject is the envelope key card 17 removes.
  Re-home `TestRunCLI_Unwire_RefusesDriftedBoardJunctionWithRefusalObject` onto the portal junction rather than deleting it: rename it to `TestRunCLI_Remove_RefusesDriftedPortalJunctionWithRefusalObject`, add a pair via `fabriccli.RunCLIIn(h.PrimeWorktree(), …, []string{"add", slug})`, re-point `h.PairPortalLink(slug)` at a `t.TempDir()` using `fslink.Remove` then `fslink.CreateDirLink`, and drive `[]string{"remove", "--force", slug}` instead of `unwire`.
  Keep every assertion unchanged: exit code 1, `ok` false, a non-empty flattened `"error"` string, a `"refusal"` object carrying non-empty `check`, `what`, `target` and `reason`, `refusal["check"]` equal to `fabricengine.CheckOwnership`, and a `"mutations"` key present on the failure path.
  This test is the repo's only positive assertion that the `"refusal"` object reaches an envelope — `envelope_test.go` asserts only its absence — so losing it as collateral would silently weaken a CLI contract this task has no business touching.
  The re-homing target must reach `removeLink` with an independent expected target: `removePortal` builds `ownedWiredJunction([]string{PortalLink(l, slug)}, portalTarget(l, slug))`, structurally identical to the deleted `unwireBoardLink`'s own gate, and `Remove` propagates the resulting `*destructiveRefusal` through `surfaceRefusal`.
  Do not re-home this test onto `_lyx` or `.lyx`.
  A drifted `.lyx` link never reaches the gate — `scanOnDiskJunctionNames` drops it from `names` because `linkIsFabricOwned` cannot resolve it inside the weft worktree — and an in-weft drift is rejected by a plain `fmt.Errorf` in `junction.go` before `removeLink` is ever called.
  Rewrite the test's doc comment to describe the portal path;
  the verb changes from `unwire` to `remove`, the envelope contract under test does not.
- **Commit:** `test(fabriccli): re-home the refusal-envelope contract onto the portal junction`

### Card 22: Regression coverage for the Hub Containment Invariant

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/clone_adopt_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/hubcontainment_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/hubcontainment_integration_test.go` with a `//go:build integration` constraint on its first line and `package fabricengine_test`, reusing `newFabricFixture`, `makeBareRemote` and `readExcludeLines` rather than re-declaring any of them, and adding no `TestMain`.
  Its header states that these cases give the Hub Containment Invariant teeth: no hub-level container is ever junctioned into a worktree, so `<worktree>/<anchorRel>/_board` must not exist after any verb.
  Add one case per verb — clone, add, and reconcile — each asserting two things after the verb runs: `os.Lstat` on `filepath.Join(<warp worktree>, <anchorRel>, fabricengine.BoardDirName)` returns an `os.IsNotExist` error, and the warp repo's `.git/info/exclude` carries no line equal to the `_board` exclude pattern.
  The clone case drives `fabricengine.CloneHub` and checks `res.PrimeCwd`;
  the add case drives `Topology.Add` on a fixture and checks the new pair's anchored directory;
  the reconcile case drives `Topology.Reconcile` and checks every pair it converged, since reconcile is the verb that used to re-wire the link unconditionally on every pass.
  Assert absence, never removal: nothing in this task sweeps a pre-existing junction, so a case that wires one by hand and expects it gone would be testing behaviour the plan deliberately does not build.
- **Commit:** `test(fabricengine): assert no _board junction is wired at clone, add or reconcile`

### Card 23: Update the module doc and the fabric CLI's user-visible text

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/junctionnames.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`, delete the whole `# The _board convenience junction` section describing the link, its wire-only-and-unmonitored property, its unconditional re-wiring, and its read-by-no-code-path property.
  Replace it with nothing rather than a stub — the Hub Containment Invariant in `CONSTRAINTS.md` is where the rule now lives.
  In `internal/fabriccli/fabric.go`, correct both cobra `Long` help strings that enumerate the warp junctions as "(`_lyx`, `.lyx`, and the `_board` convenience link)" — one on `removeCmd`, one on the `unwire` command — to name `_lyx` and `.lyx` only.
  Both are user-visible, so the CLI / Cobra Invariant's help-accuracy obligation applies.
  Also correct `resolveWarpLocation`'s doc comment, which lists "or inside the `_board` link fabric wires at every anchor" among the cwds `lyxcwd` resolves cleanly but that are not warp worktrees.
  Verify whether the surrounding logic depends on that link existing or merely describes it, and state the reason accordingly — the weft-sibling case on its own still justifies the check.
  Do not change either command's `Short`, `Use`, or `Args`.
- **Commit:** `docs(fabric): drop the _board convenience junction from module doc and CLI help`

### Card 24: Record the reversal in the repo docs and design docs

- **Context:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/junctionnames.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
  - `manifest/designs/fabric-unified-view.md`
  - `manifest/designs/fabric-windows-verification.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/overview.md`, delete the `<anchor>/_board` → `<hub>/_board` entry from the three-junction list, correct the surrounding sentence that says "three concrete junctions this repo ships with today" to two, and delete the two paragraphs describing the link as wire-only and unmonitored and as deliberately not a `pathspec` entry.
  Leave the `filterHubReserved` sentence intact — `_board`, `_portals` and `_launchers` remain the hub-structural tokens that can never be a per-worktree junction, and that is now the whole story rather than an exception plus a special case.
  Add a sentence pointing at the Hub Containment Invariant as the rule that forbids re-adding it.
  In `manifest/designs/fabric-unified-view.md`, keep the "Shipped (batch 7)" paragraphs describing the junction and append a reversal note stating that the decision was reversed, when, and why: the link was pure redundancy — the board is already reachable at `<hub>/_board` and no lyx code path read through it — and it was the one thing that broke the fabric illusion from the inside, being neither warp nor weft, shared across every worktree, and physically writable while bypassing `BoardWriteLockPath`.
  Name millhouse's `.wiki` junction as the empirical case: same shape, distinctly named, guarded by an explicit prohibition, and still edited by mistake.
  Do not delete those paragraphs;
  a design doc that silently drops a reversed decision loses the reasoning and invites a future reader to re-propose it.
  Also note in that file that the unbuilt plan to junction `_portals` and `_launchers` into every worktree is cancelled by the same invariant.
  In `manifest/designs/fabric-windows-verification.md`, drop `_board` from the "`_lyx`/`.lyx`/`_board` placement" item in the subpath-anchored verification bullet.
- **Commit:** `docs: record the _board junction reversal in overview and design docs`

### Card 25: Correct the sandbox fabric suite before it is run

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/junctionnames.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the three scenarios in `tools/sandbox/SANDBOX-FABRIC-SUITE.md` that encode the junction, all of which instruct the operator to confirm a `_board` link exists and therefore produce a false FAIL if run unchanged.
  F8 requires `_lyx`, `.lyx` and `_board` to land as links inside `<warp>/<dir>/`: drop `_board` from that list and add the opposite check, that the warp anchored directory has no `_board` entry at all.
  F13 describes fabric-owned links as "those pointing into the paired weft worktree or the hub's `_board`": drop the board clause, matching card 18's narrowing of `linkIsFabricOwned`.
  F15 requires `/_board`, `/_lyx` and `/.lyx` to be present exactly once each in the warp `.git/info/exclude`: drop `/_board` from that list, leaving the exactly-once requirement on the surviving two.
  Do not edit `tools/sandbox/SANDBOX-REED-SUITE.md`.
  It names no `.lyx` path and needs no change;
  the reed suite's value here is that it passes unchanged, which is the end-to-end evidence that batch 1's log-directory move did not break the server.
  Both sandbox runner scripts must be run by hand before this task is called done — see this batch's `## Batch Tests` section, which names them.
- **Commit:** `docs(sandbox): correct F8, F13 and F15 for the deleted _board junction`

## Batch Tests

`verify:` runs the untagged tier for the two packages this batch changes plus `cmd/lyx`, then the integration tier for both — the tier where nearly all of this batch's coverage lives.

- `-tags integration ./internal/fabricengine/... ./internal/fabriccli/...` is load-bearing, not defensive: cards 19, 20, 21 and 22 all touch or create `//go:build integration` files (`hubreservedroutes_integration_test.go`, `remove_junctions_integration_test.go`, `livestate_manifest_test.go`, `cli_test.go`, `hubcontainment_integration_test.go`), every one of which is invisible to an untagged run.
- The two assertions that actually prove the deletion landed are card 20's exact `LinksRemoved` count (2, not 3 — it fails without card 16) and card 22's per-verb absence checks (they fail if any of card 14's three call sites survives).
- Card 21's re-homed refusal test is the regression guard for the CLI contract the deletion would otherwise have taken with it;
  it fails if `removePortal`'s ownership gate stops reaching `removeLink`.
- `cmd/lyx` is in scope for the repo-wide guards over this batch's new files and doc edits — `tierpurity_test.go`, `hermeticenv_test.go`, and the Markdown Link Integrity check.
- Not covered by `verify:` and required before the task is done: `sandbox/fabric-suite.cmd` and `sandbox/reed-suite.cmd`, run by hand per the Sandbox Suite Coverage invariant.
  The repo-wide `done_gate` (`go test ./... && go test -tags integration ./...`) catches any package outside these three that the deletion breaks.
