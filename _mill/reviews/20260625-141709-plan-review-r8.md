MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [NIT] cli.go arg-parse omitted from clone-fold context
**Location:** Batch 2 / Card 4
**Issue:** Card 4 folds `runClone` arg-parsing from `internal/gitclone/cli.go`, but the old `RunCLI` (cli.go) calls `cloneHub` returning `(hubPath, resolvedBoardURL, err)` — the JSON shape uses `resolvedBoardURL`, not the raw `boardURL` arg; the card's `{hub, host, weft, board}` description should make clear `board` is the resolved value.
**Fix:** Note in card 4 that the `board` field carries `cloneHub`'s resolved board URL (default-derived when omitted), matching current behaviour.

### [NIT] init no-pairing early-return changes lyx init contract silently
**Location:** Batch 4 / Card 14
**Issue:** Making `RunInit` skip `configsync.ReconcileAll` when no `<base>-weft` sibling exists changes `lyx init` from "always scaffold config" to "scaffold only when paired" — a real behaviour change for any non-warp repo that runs `lyx init`, beyond the warp module's scope.
**Fix:** Confirm/state that `lyx init` is only ever run inside a warp hub (no standalone-repo use), or gate the early-return so unpaired repos still reconcile; otherwise this regresses plain `lyx init`.

### [NIT] configreg package doc comment update under-specified
**Location:** Batch 3 / Card 9
**Issue:** Card 9 says "update the package doc comment listing modules" for `configreg.go` (comment reads `(board, worktree, weft)` at line 4 and in the `Module.Name` example at line 17), but only names the module-list entry; the two doc-comment occurrences of `worktree` are not individually called out.
**Fix:** Name both doc-comment sites (`// Provides a neutral registry … (board, worktree, weft)` and the `Name` field example) as part of the `worktree`→`warp` rename so no stale `worktree` text remains.

## Verdict

APPROVE
Plan is constraint-compliant, source-accurate, DAG-clean; only minor clarifications.
MILL_REVIEW_END
