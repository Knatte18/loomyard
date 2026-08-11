# Batch: verb-table-and-expectations

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'verb-table-and-expectations'
number: 6
cards: 2
verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/ && go test ./internal/lyxcwd/ -run TestEnforcement
depends-on: [4, 5]
```

## Batch Scope

This batch delivers the verb table: the nine gate-reaching verbs with their `Arrange` fixtures, their per-cell expectations, their clean-state intended effects, and the seventeen hostile-input cases.
It is one batch because the expectation type, the scope table it derives outcomes from, and the verb cases that carry them are one design — splitting them would leave the expectation kinds defined against nothing and the verbs carrying an outcome nobody could derive.

The external interface batch 7 consumes is the `VerbCase` type, the `Expectation` type, and the exported verb table.

Batch-local decision: every cell's outcome is **derived** from the verified dirtiness-scope table below, never observed and then written down.
A cell written from observed behaviour asserts nothing;
that is the exact failure mode that let a green suite coexist with eight data-loss defects.

## Cards

### Card 16: `VerbCase`, `Expectation`, and the nine verbs

- **Context:**
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fabricengine/fabrictest/refusal.go`
  - `internal/fabricengine/fabrictest/states.go`
  - `internal/fabricengine/fabrictest/doc.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/slug.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/branchname.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/open.go`
  - `internal/fabriccli/clone.go`
  - `internal/weftname/weftname.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/verbs.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration` on the first line.
  **The types.**
  `VerbCase` is `struct { Name string; Arrange func(tb testing.TB, h *Hub) VerbFixture; Run func(tb testing.TB, h *Hub, f VerbFixture) error; States []string; Expect func(state string) Expectation }`, where `VerbFixture` carries whatever `Arrange` produced (a pair slug, an advanced upstream SHA, a broken remote) and the `StateTarget` the state builders need.
  `Arrange` is a **required** field, not an optional convenience: most verbs need a fixture no state builds, and folding that work into `Run` would be a correctness bug rather than a style choice — the before-manifest is captured before `Run`, so every arrangement mutation would show up in the diff as an unpermitted change and every such cell would fail.
  `States` is an optional restriction: **empty means the case inherits every state**, which is the default and what every ordinary verb uses, so a newly appended verb still gets the full matrix automatically.
  Only the hostile-input cases set it, to `clean`.
  The restriction is a property of the *case*, never a special path in the driver.
  `Expectation` carries the kind (`KindRefusedByGate`, `KindRefusedBefore`, `KindProceeds`), the `Check` or the substring the first two need, a `PermittedRoots []string` field present on **all three** kinds, and an `Effect func(tb testing.TB, h *Hub, f VerbFixture)` used by clean-state cells.
  A refusal is not assumed side-effect-free, which is why the refusal kinds carry permit roots too.
  **The verified dirtiness-scope table** — reproduce it verbatim in this file's doc comment, because every dirtiness cell's outcome is derived from it.
  It covers the declarations reachable in tranche 1 with `force=false` and is **not** an exhaustive index of every `pathRequest` in the package;
  `Remove`'s weft-side gate rows (`weftwiring.go:199` `dirtyScopeAll`, `:219` `dirtyCheckedOutBranch`), `Checkout`'s `checkout.go:200` and `Add`'s `add.go:268`/`:292` are deliberately absent because none of them changes a tranche-1 cell's derived outcome, and a later tranche that adds a force axis must extend the table rather than assume it already covers them.
  `Add` own pre-flight `add.go:43` `scopeTracked` prime warp worktree;
  `Checkout` own pre-flight `checkout.go:42` `scopeTracked` weft worktree;
  `Checkout` **no warp-side dirtiness probe exists** (`checkout.go:38-85` goes straight to `git switch`);
  `Remove` own pre-flight `remove.go:69` `scopeAll` the pair's warp worktree;
  `Remove` `refuseDirtyWeftWorktree` `remove.go:79`/`:144` `scopeAll` the pair's weft worktree;
  `Remove` gate `remove.go:196`/`:230` `dirtyScopeAll` the pair's warp worktree;
  `Remove` gate `launchers.go:172`/`:193` `dirtinessNA`;
  `Prune` own pre-flight `prune.go:214` `scopeTracked` weft path;
  `Prune` gate `prune.go:269`/`:292` `dirtyScopeTracked` stale pair paths;
  `Pull` own pre-flight `pull.go:143` `scopeTracked` prime warp worktree;
  `Pull` gate via `Fabric.ResetHard` `destroy.go:762` `dirtyScopeTracked` prime warp worktree;
  `Reconcile` own pre-flight `reconcile.go:301` `scopeAll` the board;
  `Cleanup` gate branch-shaped `dirtyCheckedOutBranch`;
  `UnwireJunctions` gate link-shaped `dirtinessNA`;
  `CloneHub{Reset}` gate `clone.go:585` `dirtinessNA`.
  **The derivation rule:** a `scopeTracked` verb against an untracked-only state expects `Proceeds`;
  a `scopeAll` verb against the same state expects a refusal;
  a verb with **no probe** for the dirtied side gets an explicit per-cell expectation, never a derived one.
  `dirtyWarpTracked × Checkout` is the only such case in tranche 1, and it is **git-decided**: `git switch` either carries the tracked modification across or refuses with "your local changes would be overwritten", so the cell asserts what must hold **either way** — the tracked modification is still on disk afterwards, and the pair is **not half-switched** (either both sides moved or neither did).
  Guessing which branch git takes would be asserting git's behaviour rather than fabric's;
  the disjunction still catches what matters, since a half-switched pair is exactly what `Checkout`'s documented all-or-nothing rollback exists to prevent.
  **The eight ordinary verbs**, each with its `Arrange`, `Run`, permit roots and clean-state `Effect`:
  `Topology.Add` — `Arrange` **breaks the warp origin remote** (`git remote set-url origin <nonexistent>`) so the push at the end of `Add` fails after the branch and worktree already exist, triggering `rollbackAdd`;
  this is the same injection `TestBranchOwnership_RefusalHoldsAtOtherDeletionSites` already uses, so it is a proven trigger rather than a guess, and without it `Add` never reaches the gate at all because `rollbackAdd` fires only at post-creation sites (`add.go:139-204`) and none of the ten states induces one.
  Clean-state effect: worktree, weft sibling, branch, junctions, launchers and portal all exist.
  `Topology.Remove` — `Arrange` adds a pair;
  clean-state effect: all of those gone, hub otherwise intact.
  Its dirty-refusal cells declare `PortalLink` and `LauncherDir` as permitted roots, because `remove.go:61-66` runs `removePortal` and `removeLaunchers` **before** the dirty pre-flight at `:68-76`, so a correctly-refusing cell has already destroyed them;
  add a comment at that permit naming it as the one tranche-1 refusal that is not side-effect-free and pointing at `doc.go`'s record of it.
  `Topology.Prune` — `Arrange` produces a **stale** pair;
  `Run` calls `Prune(l, true, false)`;
  clean-state effect: stale pairs gone, live pairs intact.
  `Topology.Cleanup` — `Arrange` produces an orphan managed branch;
  `Run` calls `Cleanup(l, true, false)`;
  clean-state effect: orphan managed branches gone, primary weft branch intact.
  `Topology.Checkout` — `Arrange` **adds a pair**, and `Run` passes that pair's warp branch (`t.cfg.BranchPrefix + slug`, see `add.go:51`) as the branch argument.
  Without this the cells are vacuous: a factory-built hub carries only the clone's own branch, so `Checkout` handed that branch no-ops and handed anything else errors on a nonexistent ref, which would hollow out `dirtyWeftUntracked × Checkout` — the sole justification for both the third expectation kind and the tenth state — and leave the clean-state effect unreachable.
  Clean-state effect: prime warp on the branch, weft on the corresponding `WeftBranchName` sibling.
  `Topology.Reconcile` — `Arrange` needs no fixture beyond the built hub;
  clean-state effect: returns pairs, no error, no mutation outside repair roots.
  `fabricengine.UnwireJunctions` — `Arrange` adds a pair;
  clean-state effect: junctions gone, worktree intact.
  `Fabric.Pull` (via `fabricengine.Open`) — `Arrange` **must push a new commit to that scenario's own warp bare**, or the entire column is vacuous: `pull.go:210-212` returns early on `localHEAD == upstreamSHA`, before the dirty check at `:216` and before any `ResetHard`, so without the advance the `dirtyWarpTracked × Pull` cell would expect `ErrWarpDirty` and **fail against a correct binary**.
  Record this as a precondition of the R2 scenario, not an implementation detail — R2's defect is only reachable on an advance path.
  Clean-state effect: warp advanced to upstream.
  **The `CloneHub{Reset: true}` column is re-scoped to the ownership axis and is not run against the ten states.**
  `resetHub` declares `dirtinessNA` (`clone.go:585`), so no dirtiness state can refuse it — total hub destruction on `--reset` is correct behaviour — which would make the permitted root the hub root itself and the manifest diff vacuous across all ten states.
  The gate is structurally unreachable for `Reset`: `resetHub` refuses a non-hub at its own pre-flight (`clone.go:573-577`, `!looksLikeHub`) **before** the `pathRequest` at `:579` is built, and the gate's `pathOwnershipFabricHub` predicate is that **same** `looksLikeHub` call (`destroy.go:346-350`), so a `RefusedByGate(CheckOwnership)` expectation here would fail against a correct binary.
  This column therefore uses `RefusedBefore("is not a fabric hub")` and drives exactly two targets: **(a)** a `<derived>-HUB`-named directory that is *not* a hub — refused at the pre-flight with its contents fully surviving, which is R4's `clone --reset` defect;
  and **(b)** a real hub — torn down and rebuilt, the positive case proving the column is not trivially always-refusing, with the rebuild asserted through `fabriccli.CloneAndWire` so junctions and the repo-wide `fabric.yaml` are present in the result.
  Both targets are named through `fabricengine.HubPath(cwd, fabricengine.DeriveWarpName(warpURL))`, since `resetHub` only ever targets that path and an arbitrary directory cannot be aimed at.
  Omit the dirtiness rows for this column **with the reason stated in the table**, never silently present-and-vacuous.
  **The seventeen hostile-input cases**, all with `States: []string{"clean"}` and both anchors:
  `Add` and `Remove` each take the same seven slug-shaped inputs — `""`, `.`, `..`, `../x`, a `weftname.Suffix`-suffixed name, a reserved hub name, and a leading `-`;
  `Checkout` takes two **branch-shaped** inputs (a nonexistent branch and a flag-shaped name), because `checkout.go:38-85` validates no branch at all;
  `UnwireJunctions` takes one — a junction name escaping its worktree (`../x`) — which is deliberately in scope despite taking neither a slug nor a branch, because its `names []string` is the only route to the link executor's containment check.
  Crossing these with the full state matrix would multiply out to hundreds of nearly-vacuous cells: `remove ..` refuses at `remove.go:45` before any hub state is consulted, so all but one of its state rows would assert exactly what that one does.
  Derive each input's expectation from what `validateWorktreeSlug` (`slug.go:30`) actually rejects — empty or whitespace-only, containing `/` or `\`, anything where `slug != filepath.Clean(slug)` plus `.` and `..` explicitly, anything ending in the weft suffix, and any reserved hub name — so `remove ..` uses `RefusedBefore` on the slug-validation message, **not** `RefusedByGate(CheckContainment)`, which would fail against a correct binary because the gate's containment refusal at `destroy.go:528` is unreachable for that input.
  **A leading `-` is *not* rejected by `validateWorktreeSlug`** — it is not on any of that function's five rejection rules — so no gate check owns it and neither refusal kind applies.
  Those cells, and the two `Checkout` hostile cells, are written against the **safe** expectation: the argument does not reach git as a flag, and nothing outside the permitted roots is destroyed.
  If the current code does let it through as a flag the cell fails, and that failure is instance nine, which is the point of the tranche — asserting whatever the current behaviour happens to be would assert nothing.
  The `Checkout` hostile cells additionally assert **no half-switched pair**.
  **The omission table.** Export a table naming every verb/state pair excluded from the cross product with its reason, so a green matrix can be audited against what it did not run.
  It starts with the structural-state omissions derived in the overview — `Cleanup` and `Checkout` all four (branch-shaped; their only gate call is `deleteBranch` at `cleanup.go:275` and `checkout.go:195-203`), `Pull` all four (its only gate call is `Fabric.ResetHard` at `destroy.go:762`, not a path executor), `Add` two (`trackedSymlinkAtWiredPath` and `staleWiredJunction`, whose paths do not exist at `Add`'s target before `Run`), `UnwireJunctions` one (`unrelatedGitCloneAtWeftNamedPath`, never visited by `removeLink` at `unwire.go:143-152`) — and grows with any dirtiness-state omission resolved here.
  An arbitrarily-planted state is worse than an absent one: it produces a green cell that proves nothing while reading like coverage.
- **Commit:** `fabrictest: add the tranche-1 verb table, expectations and hostile inputs`

### Card 17: per-verb clean-state wiring suite

- **Context:**
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fabricengine/fabrictest/states.go`
  - `internal/fabricengine/fabrictest/refusal.go`
- **Edits:**
  - `internal/fabricengine/fabrictest/doc.go`
- **Creates:**
  - `internal/fabricengine/fabrictest/verbs_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`, `package fabrictest`, one `t.Run` per verb case, each `t.Parallel()` on its own hub, both anchors.
  Drive **each verb case exactly once in the `clean` state** and assert that its `Arrange`, its `Run` and its clean-state `Effect` are all three wired correctly, before the cross product multiplies any mistake by nine.
  Each subtest runs the five phases in order (build, arrange, state, capture-before, run-then-capture-after) exactly as the driver will, so a phase-ordering mistake surfaces here rather than as a mysterious unpermitted-change failure across dozens of cells.
  Three assertions per verb: `Run` returns no error;
  the `Effect` closure passes;
  and the manifest diff reports nothing outside the cell's permitted roots.
  This is the tautology guard in its cheapest form — without a passing clean-state cell, a gate that refused every request would satisfy every refusal cell in the matrix, and slice 12 just rewired roughly 29 call sites into that gate.
  Add three targeted assertions the cross product cannot make for itself: that `Pull`'s `Arrange` really advanced the warp bare (local HEAD differs from upstream before `Run`), that `Add`'s `Arrange` really broke the origin remote (the configured URL does not resolve), and that `Checkout`'s `Arrange` really produced a second branch distinct from the one already checked out.
  Each of those three is a precondition whose silent failure would make a whole column vacuous while still passing.
  Also assert the hostile-input cases are restricted correctly: every case with a non-empty `States` restriction names only `clean`, and every ordinary verb's `States` is empty.
  Update `doc.go`'s omission-table section with the omission set card 16 produced, so the durable record and the code agree at the batch that fixes it.
- **Commit:** `fabrictest: prove every verb case is wired before the cross product multiplies it`

## Batch Tests

`verify:` runs card 17's suite via `go test -tags integration ./internal/fabricengine/fabrictest/`, which is the batch's substantive gate and also re-runs batches 2-5's suites, correctly — the verb cases are built on the factory, the manifest, the refusal helpers and the states, and a regression in any of them must fail here.
`go build ./...` catches a compile break in the default build, which matters because `doc.go` is untagged and card 17 edits it.
`go test ./internal/lyxcwd/ -run TestEnforcement` covers the geometry and vocabulary rules over `verbs.go`, the file most likely to reach for a raw `_portals`/`_launchers` literal — every such path must come from `fabricengine.PortalLink`/`fabricengine.LauncherDir` instead.
