MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] "run lyx init" message twins under-enumerated
**Section:** Technical context (config.go bullet) / init-dissolves-to-fabric-verbs
**Issue:** The discussion names only "shuttle/reed/board engines" as twins of the `not initialized here; run "lyx init"` message, but that exact string lives in EIGHT config.go files (webster, builder, shuttle, board, perch, reed, loom, fabric) plus reconcile.go:230,235 detail strings — a plan following the scope would leave stale help pointing to a deleted command, violating the CLI/Cobra help-accuracy obligation.
**Fix:** List all eight `*/config.go` occurrences and the reconcile.go raw-adopt detail strings as the full update set.

### [GAP] Stale-junction-removal lacks a failed/absent-pathspec guard
**Section:** declarative-junction-reconcile / Testing (Reconcile convergence)
**Issue:** Reconcile removes on-disk junctions absent from the repo-wide `pathspec`, but the discussion gives no guard for when the repo-wide `fabric.yaml` is missing or fails to load — an empty/errored pathspec would strip ALL junctions across EVERY worktree (Resolve gets a marker-absent fallback; the pathspec load gets none).
**Fix:** Specify that a config-load failure or absent repo-wide `fabric.yaml` aborts stale-removal (error, touch nothing), never treated as an empty "remove-everything" set.

### [NOTE] On-disk junction enumeration + reserved-name safety unspecified
**Section:** declarative-junction-reconcile
**Issue:** `checkJunctionHealth` (reconcile.go:340) only iterates the config name-set via `HostJunctionsHere(names)`; the new stale step must instead scan actual on-disk link entries, and the discussion neither names that scan source nor states that hub-structural reserved names (`HubReservedNames()`: `_board`/`_portals`/`_launchers`/`_raddle`) are excluded from removal.
**Fix:** State the on-disk enumeration source and require excluding `HubReservedNames()` before unwiring.

### [NOTE] CloneHub anchor-echo return change unstated
**Section:** first-clone-validates-subpath-exists / output.go bullet
**Issue:** `CloneHub` returns `(hubPath, err)`; echoing the resolved `anchor` in the JSON result requires the engine to also return the anchor, which the discussion does not mention.
**Fix:** Note that `CloneHub`'s signature gains a returned anchor value for the CLI to place in `output.Ok`.

## Verdict

GAPS_FOUND
Two gaps: undercounted stale-help twins and an unguarded stale-removal-on-load-failure hazard.
MILL_REVIEW_END
