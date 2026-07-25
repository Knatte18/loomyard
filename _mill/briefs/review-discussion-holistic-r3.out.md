MILL_REVIEW_BEGIN
# Review: fabric: unify warp + weft into one git-coordination module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Clone's board-repo (boardURL) handling unaddressed
**Section:** Scope / Decisions / Technical context (§ CloneHub)
**Issue:** Warp's `CloneHub(cwd, hostURL, weftURL, boardURL) (hubPath, resolvedBoardURL, err)` clones a THIRD repo (the board) into `<name>-HUB`, but fabric's scope, the `Fabric{Warp, Weft}` type, and the `clone` verb never mention the board — yet the differential test asserts "fabric clone vs warp clone" equivalent end state, which includes board setup.
**Fix:** State whether fabric `clone` replicates warp's board clone (boardURL/resolvedBoardURL), and where that lives given `Fabric` holds only `Warp`/`Weft` (e.g. a package-level clone function returning `resolvedBoardURL`, not a struct field).

### [NOTE] Exported helper surface for parallel build unscoped
**Section:** § Self-contained junction/portal/launcher mechanics; Scope
**Issue:** Warp's consumer-facing helpers `PairInSync` and `HostClean` (loom preflight) are named in Technical context/blast-radius but no decision says whether fabric implements them in the parallel build (dead until cutover) or defers them; junction/hook mechanics are covered, these two are not.
**Fix:** Note explicitly that `PairInSync`/`HostClean` (and any other loom-preflight helpers) are deferred to cutover, or included now, so a plan writer knows the exported surface to build.

### [NOTE] "Exactly one construction site" for branch naming is inaccurate
**Section:** § Technical context — Branch naming today
**Issue:** The claim "exactly one construction site — warpengine/add.go:89" is false; `warpengine/remove.go:49` also builds `branch := w.cfg.BranchPrefix + slug`, and the same sentence then says branches are mirrored "everywhere (add, checkout, reconcile)" — self-contradictory.
**Fix:** Change to "the sole branch-derivation formula (BranchPrefix+slug), applied at several sites (add, remove, checkout, reconcile)"; the `-weft` scheme being new still holds.

## Verdict

GAPS_FOUND
One unaddressed dimension: fabric clone's handling of the board repo; two minor notes.
MILL_REVIEW_END
