# Batch: drop-adoption-and-reap-chokepoint

```yaml
task: 'reed: pane reap isn''t applied consistently across up/add''s mutating paths'
batch: 'drop-adoption-and-reap-chokepoint'
number: 2
cards: 4
verify: go test ./internal/reedengine/ -run 'TestPlanPaneTarget|TestLaunchStrandLocked|TestPlanReconcile|TestReconcileLocked|TestValidateSplitCreatedNewPane'
depends-on: [1]
```

## Batch Scope

This batch delivers `spawn.go`'s share of the fix: pane adoption is deleted outright, and `launchStrandLocked` becomes the reap-before-allocate chokepoint so that "reap before allocate" holds by construction on every strand-realizing path — `AddStrand`, `UpdateStrand`, `Resume`, and any future one.
It is one batch because both changes land in `planPaneTarget` / `soleAliveNonHeaderPane` / `launchStrandLocked`, which are three functions in one file that call each other, and because the chokepoint is what makes dropping adoption safe: once the reap disposes of the session's initial pane like any other untracked pane, a fresh split is idle by construction and reed-owned end to end.

It depends on batch 1 because the chokepoint is inert without batch 1's relaxed gate: with zero strands bound, today's `anyBoundPresent`-only gate refuses to reap, so a reconcile inserted before `planPaneTarget` would kill nothing and M16 would survive.

The external interface batches 3 and 4 consume: `planPaneTarget` returns one value plus an error, `soleAliveNonHeaderPane` no longer exists, and the operator-facing no-panes error string is `"session has no panes to split"`.

Batch-local decisions beyond `## Shared Decisions`: the ordering assertion is pinned at the untagged unit tier via the `execHook` fake, not only by batch 4's smoke regressions — a smoke test proves the outcome on one backend, not the ordering, and it does not run on every `go test`.

## Cards

### Card 4: Delete pane adoption from planPaneTarget

- **Context:**
  - `internal/reedengine/apply.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `planPaneTarget`'s signature from `func planPaneTarget(strands []Strand, live []LivePane, headerPaneID string) (adoptID, splitTargetID string, err error)` to `func planPaneTarget(live []LivePane, headerPaneID string) (splitTargetID string, err error)`.
  Delete the `anyBound` computation and the `if !anyBound { if sole, ok := soleAliveNonHeaderPane(...); ok { return sole, "", nil } }` branch entirely — `planPaneTarget` now always returns a split target.
  Dropping the `strands` parameter is part of this card, not a follow-up: the `anyBound` loop is its only reader, so after that deletion the function no longer consults the strand table at all.
  The surviving split-target rules are a pure function of `live` and `headerPaneID`.
  Delete the function `soleAliveNonHeaderPane` and its doc comment.
  Keep the three surviving split-target rules exactly as they are: prefer the tallest alive non-header pane, fall back to any present non-header pane (a corpse), and fall back to `live[0]` (the header itself) when no non-header pane exists at all.
  Change the no-panes error string from `"session has no panes to adopt or split"` to `"session has no panes to split"`.
  This string is also quoted verbatim in one smoke test and in the sandbox reed suite, both of which are corrected by later batches;
  leave those two files untouched from this card.
  Rewrite `planPaneTarget`'s doc comment rather than merely trimming it.
  The replacement states that the function always yields a split target and never adopts an existing pane, and records why the seam was removed: the sole-alive-non-header-pane heuristic could not distinguish reed's own initial pane from a foreign one and produced two live findings (R4-F5, adopting the previous header pane after a `reed.json` scrub; M16, adopting an operator's `split-window`), and once the untracked reap is authorized by an alive header the initial pane is disposed of like any other untracked pane, so a fresh split — idle by construction — costs one `kill-pane` plus one `split-window` and buys correctness.
  Update the call site in `launchStrandLocked` to the new signature: bind `splitTargetID, err := planPaneTarget(live, st.HeaderPaneID)`, dropping the `st.Strands` argument, delete the `paneID := adoptID` assignment and the `if paneID == ""` guard that wrapped the split, and make the `split-window` call plus its `validateSplitCreatedNewPane` check unconditional.
  Keep `-c e.geom.PaneCwd` on the `split-window` argv and keep the comment explaining why that pin is load-bearing.
  Keep `validateSplitCreatedNewPane` at this call site.
- **Commit:** `fix(reedengine): never adopt an existing pane when realizing a strand`

### Card 5: Rewrite spawn_test.go's planPaneTarget table for the split-only contract

- **Context:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/lock_test.go`
- **Edits:**
  - `internal/reedengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestPlanPaneTarget`, delete both the `wantAdoptID` and the `strands` fields from the table struct — card 4 removed the `strands` parameter, so no case can still supply one — and update the call and assertion bodies to the new signature `planPaneTarget(tt.live, tt.headerPaneID)`.
  Convert every case that asserted adoption to assert the split target the new code picks instead: `FreshSession_AdoptsTheAliveInitialPane`, `AllStrandsPaneless_AdoptsFirstAlivePane`, `HeaderPresentNoStrandBound_HeaderNeverAdopted`, and `SeveralAlivePanesButOnlyOneNonHeaderAlive_StillAdopts`.
  Rename each of those cases so its name states the split-only contract rather than the deleted one, and rewrite each one's comment for the same reason.
  Between them they must still cover every pane-set shape the old adoption branch used to catch — a sole alive non-header pane with no header set, a sole alive non-header pane beside a live header, and a sole alive non-header pane beside a corpse — now asserting that a split target is returned.
  Dropping the `strands` field collapses two pairs of cases into literal duplicates, since each pair differed only in the strand table it supplied.
  Merge each pair into one case rather than leaving two identical rows: `FreshSession_AdoptsTheAliveInitialPane` with `AllStrandsPaneless_AdoptsFirstAlivePane` (both reduce to a single alive pane and no header), and `OneStrandHoldsAPane_SplitsTheTallestAlive` with `TinyActiveBand_SplitTargetsTheTallestNotTheFirst` (both reduce to a 2-row pane beside a 47-row pane and no header).
  Carry the surviving case's comment forward from whichever of the pair explains the rule better — for the second pair that is `TinyActiveBand_SplitTargetsTheTallestNotTheFirst`, whose comment records the session-target split defect this planner replaces.
  Keep `SoleCorpseUnbound_NeverAdopted_SplitOffTheCorpse`, `DeadPaneNeverTheSplitTargetWhileAnyAlive`, `NoPanesAtAll_Errors`, `HeaderPresentWithStrand_HeaderNeverTheSplitTarget`, `SeveralUntrackedAlivePanes_SplitsRatherThanGuessingWhichToAdopt`, and `HeaderIsSolePane_SplitTargetFallsBackToHeader` with their current expectations — the split-target policy is unchanged, and these are what pin it.
  Rename `SoleCorpseUnbound_NeverAdopted_SplitOffTheCorpse` and `SeveralUntrackedAlivePanes_SplitsRatherThanGuessingWhichToAdopt` only if their names assert adoption semantics that no longer exist;
  their expectations must not change either way.
  After the merges the table must still contain a case for every surviving rule, and no two cases may share an identical `live` plus `headerPaneID` pair.
  Rewrite the file-header comment of `internal/reedengine/spawn_test.go`: it currently describes an adopt-vs-split decision, a header exclusion from adoption, and a sole-candidate narrowing on adoption, all of which are gone.
  The replacement describes the split-target policy the table now pins (tallest alive non-header, present-corpse fallback, header-as-last-resort) and the header's exclusion from being the *preferred* split target.
  Do not touch `TestLoadOrInitStateLocked_AbsentFileInitializesFromEngineIdentity`, `TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIdentity`, `TestSendKeysLiteralArg`, `TestValidateSplitCreatedNewPane`, or `TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims` in this card — none of them depends on adoption.
- **Commit:** `test(reedengine): retable planPaneTarget for the split-only contract`

### Card 6: Reconcile before allocating a pane inside launchStrandLocked

- **Context:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `launchStrandLocked`, insert a reconcile between the existing `listPanes` call and the `planPaneTarget` call, following the same kill-then-re-enumerate-then-plan ordering `reconcileApplyPersistLocked` already treats as load-bearing: call `e.reconcileLocked(st, live)`, and when it reports a non-empty killed slice, re-run `e.tmux.listPanes(session)` and rebind `live` to the fresh result before calling `planPaneTarget`.
  Wrap a reconcile error as `fmt.Errorf("reconcile: %w", err)` and a failed re-enumeration as `fmt.Errorf("list panes after reconcile: %w", err)`, matching `reconcileApplyPersistLocked`'s existing wording.
  When nothing was killed, do not re-enumerate — the pane set is unchanged and a second `list-panes` would be a redundant round trip.
  The post-reap `live` slice must be the one passed to both `planPaneTarget` and `validateSplitCreatedNewPane`, so the split target is chosen from the surviving pane set and the new-pane guard compares against it.
  Do not add a `SaveState` call inside `launchStrandLocked`, and do not add a reconcile call to `AddStrand`, `UpdateStrand`, or `addStrandLocked`/`updateStrandLocked` — the whole point is that the single chokepoint makes this true for every present and future realization path.
  Rewrite `launchStrandLocked`'s doc comment: it currently says the helper adopts the initial pane or splits the tallest alive one.
  The replacement states that it reconciles first, then always splits, and that it sets only `s.PaneID` with `Live` derived from pane binding downstream.
  Add a comment recording two things the discussion settled.
  First, why reaping here is safe for the strand being launched, stated per path rather than as one universal claim.
  On the `AddStrand` and `UpdateStrand` paths the strand reaches this helper with `PaneID == ""`, so reconcile can neither clear nor kill anything belonging to it.
  On `Resume` that is not universally true — `planResumeLaunches` selects strands whose pane is absent from `aliveIDSet`, which includes a strand still bound to a dead-but-present pane — and the comment must say so rather than overstate.
  It is harmless there: that binding names a corpse, so reconcile either kills it as a dead pane and clears the binding, or spares it as the kept dead pane, and either way the helper overwrites `s.PaneID` with the freshly split pane a few lines later.
  Also record that during `Resume`'s per-strand loop the already-relaunched strands are bound to alive panes and are therefore exempt from the untracked reap.
  Second, the accepted destructive-then-unpersisted window: `AddStrand` and `UpdateStrand` reach `SaveState` only after this helper returns nil, so a `split-window` or `send-keys` failure now returns with panes already killed and other strands' binding clears living only in memory.
  That is accepted rather than closed because it is self-healing — `reed.json` is left exactly as it was, `Status` and `toRenderStrands` derive liveness from the live pane set rather than the persisted binding, and the next mutating verb's reconcile clears the stale bindings — whereas calling `SaveState` here would persist the half-added strand record on the `add` path and turn a clean failure into a phantom strand `Resume` would later try to launch.
  State in that comment that the right shape, if this window ever matters, is reaping before the strand record is appended, not persisting a partial one.
  Also rewrite the file-header comment of `internal/reedengine/spawn.go`, whose first sentence says `launchStrandLocked` creates "(or adopts)" a tmux pane.
- **Commit:** `fix(reedengine): reap untracked panes before allocating a strand's pane`

### Card 7: Pin the reap-before-allocate ordering at the unit tier

- **Context:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/lifecycle_test.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a test to `internal/reedengine/spawn_test.go` — name it `TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget` — driving `launchStrandLocked` directly through the `newTestEngine` fixture and an `e.tmux.execHook` fake, in the style `lifecycle_test.go`'s `TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure` already uses (switch on `args[0]`, script `list-panes` and `split-window`, capture the split argv).
  The fixture is a `ReedState` with an alive header pane, zero strands bound to a present pane, and one untracked alive pane;
  the strand passed in has `PaneID == ""`, mirroring how `addStrandLocked` appends it.
  The hook records the ordered sequence of tmux verbs it is asked to run and answers `list-panes` with the pre-reap pane set on the first call and the post-reap pane set (the untracked pane gone) on subsequent calls, so the re-enumeration is observable.
  Assert three things: a `kill-pane` naming the untracked pane is issued before any `split-window`;
  a second `list-panes` separates the `kill-pane` from the `split-window`;
  and the `split-window` target argument is not the reaped pane's id.
  Add a companion test — name it `TestLaunchStrandLocked_SkipsTheRedundantReEnumerationWhenNothingIsReaped` — whose fixture has nothing to reap (an alive header plus a strand already bound to a present alive pane), asserting that no `kill-pane` is issued at all and that exactly one `list-panes` precedes the `split-window`.
  Both tests stay untagged and pure: no `exec.Command`, no `hubforge.NewHub`, no sleep — `newTestEngine` points `cfg.Tmux` at a nonexistent binary, so any unfaked shell-out fails loudly rather than reaching a real server.
  Update the file-header comment of `internal/reedengine/spawn_test.go` again if card 5's rewrite left it claiming `launchStrandLocked` is exercised only through its decision seam and never invoked directly — these two tests invoke it directly.
- **Commit:** `test(reedengine): pin reap-before-allocate ordering in launchStrandLocked`

## Batch Tests

`verify: go test ./internal/reedengine/ -run 'TestPlanPaneTarget|TestLaunchStrandLocked|TestPlanReconcile|TestReconcileLocked|TestValidateSplitCreatedNewPane'` covers `internal/reedengine/spawn_test.go`'s retabled `TestPlanPaneTarget` (card 5), card 7's two `TestLaunchStrandLocked_*` ordering tests, and `TestValidateSplitCreatedNewPane` — the guard card 4 keeps at the now-unconditional split call site, which is worth re-running when that call site changes shape.

`TestPlanReconcile` and `TestReconcileLocked*` from batch 1 are re-run because card 6 adds a second production caller of `reconcileLocked`;
a regression there would otherwise only surface at batch 4.

The scope is deliberately narrower than the package.
`go test` builds the whole package before applying the `-run` filter, so a compile break anywhere in `internal/reedengine` — including the several test files that reference `planPaneTarget` indirectly — still fails this gate, and the overview's module-wide `verify:` covers everything outside the package.
