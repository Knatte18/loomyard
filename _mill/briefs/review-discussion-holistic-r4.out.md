MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] No arrange phase; manifest capture point undefined
**Section:** cross-product-shape / survival-assertion-mechanism
**Issue:** `VerbCase{Name; Run}` and `State{Apply}` give no per-verb arrangement hook, yet most verbs need a fixture the state does not build (`Remove`/`Unwire` need an added pair, `Prune` needs a stale pair, `Cleanup` needs an orphan managed branch, `Pull` needs an upstream advance); folding that into `Run` makes the "before" manifest — captured before `Run` — include none of the arranged state, so every arrangement mutation reports as an unpermitted change.
**Fix:** Add a third `VerbCase` field (an `Arrange` step run before the before-manifest is captured) and state explicitly where in the state/arrange/capture/run/capture order each phase sits.

### [BLOCKING:design] Pull column is vacuous without a seeded upstream advance
**Section:** tranche-1-verb-table / dirty-what-per-cell / clean-state-effect-assertions
**Issue:** `pull.go:210-212` returns early when `localHEAD == upstreamSHA`, *before* the dirty check at `:216` and before `ResetHard`; the bare-remote template strategy never advances the warp bare after cloning, so every `Pull` cell short-circuits — the `dirtyWarpTracked × Pull` cell would expect `ErrWarpDirty` and fail against a correct binary, and the clean-state effect "warp advanced to upstream" is unreachable.
**Fix:** State that the `Pull` case pushes a new commit to the warp bare (per scenario, on its own bare copy) before running, and record it as a precondition of the R2 scenario.

### [BLOCKING:design] Structural states have no per-verb placement rule
**Section:** tranche-1-state-matrix / dirty-what-per-cell
**Issue:** `dirty-what-per-cell` fixes *which checkout* a dirtiness state dirties, but the four structural states (`trackedSymlinkAtWiredPath`, `foreignDirAtFabricOwnedPath`, `unrelatedGitCloneAtWeftNamedPath`, `staleWiredJunction`) get no equivalent rule — a wired junction path exists on both the prime warp and each pair worktree, and a fabric-owned path could be the pair worktree, its weft sibling, `_portals` or `_launchers`, so most of their 8×4 cells are undefined or vacuous.
**Fix:** Add a per-verb resolution rule for the structural states mirroring `dirty-what-per-cell` — plant at the path the verb under test actually acts on.

### [BLOCKING:design] Add's rollback path has no stated trigger
**Section:** tranche-1-verb-table
**Issue:** The table lists `Topology.Add` "(its rollback path)" and claims "every executor gets driven", but `rollbackAdd` fires only on a *post-creation* failure (`add.go:139-204`), and no named state is stated to induce one — `Add`'s only reachable probe in the nine states is its `scopeTracked` pre-flight at `add.go:43`, so `Add` may never reach `destroy.go` at all.
**Fix:** Name the state or arrangement that makes an `Add` step fail after `warpTok` is minted, or state that `Add`'s rollback is unexercised in tranche 1 with the reason.

### [BLOCKING:consistency] Hostile-input cell count contradicts its own refinements
**Section:** tranche-1-verb-table
**Issue:** "7 hostile inputs × 3 verbs × 1 state × 2 anchors = 42" counts the seven slug-shaped inputs against `Checkout`, which the same section says takes a *branch-shaped* set of two; and `UnwireJunctions`' `../x` hostile input is described one paragraph earlier yet excluded from the "only `Add`, `Remove`, `Checkout`" scoping and from the total — so the 190 figure is not derivable from the stated rules.
**Fix:** Recompute from the per-verb input sets actually stated, and give `UnwireJunctions`' hostile input an explicit in-or-out disposition.

### [NIT:consistency] Scope table mislabels Remove's gate rows
**Section:** three-expectation-kinds-and-the-scope-table
**Issue:** The row "`Remove` | gate (`remove.go:196`, `:230`) | `dirtyScopeAll` | worktree / launchers" is half wrong — both cited lines are `removeWarpWorktreeDir`'s primary and fallback requests against the warp worktree, while `launchers.go:172`/`:193` declare `dirtinessNA`.
**Fix:** Drop "launchers" from that row, or give launchers its own `dirtinessNA` row.

## Verdict

REQUEST_CHANGES
Arrangement phase, Pull precondition, structural-state placement and cell arithmetic all unresolved.
MILL_REVIEW_END
