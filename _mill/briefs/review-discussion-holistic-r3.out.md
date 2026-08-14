MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Seed-excludes error posture rests on a false premise
**Section:** `### seed-excludes-at-clone` (Rejected clause)
**Issue:** The rejection of a fatal call ("the existing best-effort posture at `reconcile.go:311` is deliberate and should not diverge") inherits that site's justification — `reconcile.go:308-310`'s "the board worktree's artifact excludes are self-healing (every weft-git verb re-seeds them)" — which is false for the board: `Bolt.Commit` → `commitWeftAt` → `gitrepo.StageAllAndCommit` (`weftgit.go:337-342`) seeds nothing, unlike `Fabric.Commit`'s `ensureWeftLockDir` path (`commit.go:178`). A silently-swallowed seed failure at clone therefore leaves the stage-all at `fabriccli/clone.go:59` unprotected — the exact exposure the decision exists to close — with no later re-seed before it.
**Fix:** State the new call's error posture explicitly and on its own footing (fatal via `teardownHub`, or non-fatal with a stated reason that does not appeal to board-side self-healing), since the plan writer must choose `_ = seed…` versus a checked call.

### [NIT:scope] Scope "In" omits three enumerated edits
**Section:** `## Scope` vs `## Technical context` / `## Testing`
**Issue:** `internal/fabricengine/slug.go:4` (production doc comment naming `<Hub>/.lyx`), `cmd/lyx/uncontainedwrite_test.go:72-74` (allowlist reason string), and `cmd/lyx/constructoranchoring_test.go:96,144` + header comment are all verified real and required, but appear only in the later sections; the In-list names only the reed prose, sandbox suite, and two docs.
**Fix:** Add the three to the Scope "In" list so the work inventory is complete when read alone.

### [NIT:consistency] Testing plan drops one named fabriccli test
**Section:** `## Testing`, `**internal/fabriccli**`
**Issue:** The inventory names `cli_test.go:768-791` (`TestRunCLI_Unwire_ReportsBoardJunctionRemoval`, verified standalone), but the Testing bullet lists only `envelopecontract_integration_test.go` and `cli_test.go:887-896`.
**Fix:** Name the 768-791 test's deletion in the Testing section too.

### [NIT:scope] Stale junction prose in the live-state manifest test
**Section:** `## Technical context` → prose inventory
**Issue:** `internal/fabricengine/livestate_manifest_test.go:96` describes "warp/_board to the hub's `_board`" as one of fabric's wired junctions in the rationale for the no-descend walk rule; it goes stale with the deletion and is not listed.
**Fix:** Add it to the enumerated prose inventory.

## Verdict

REQUEST_CHANGES
One decision's error posture rests on a self-healing claim that is false for board commits.
MILL_REVIEW_END
