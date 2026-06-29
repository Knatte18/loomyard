MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\cobra-cli-engine-sweep\_mill\discussion.md
date: 2026-06-29
```

## Findings

### [GAP] ide codeLauncher seam not split-addressed
**Section:** Decisions › ambiguous-file-placement / Technical context › ide
**Issue:** `internal/ide/cli_test.go` (white-box pkg `ide`, lines 25-27) swaps the unexported `codeLauncher` seam, which lives in `spawn.go` and is pinned to `ideengine`; once `cli_test.go` moves to `idecli` it can no longer reach `codeLauncher`, breaking the build — the exact seam class resolved for warp (`RemoveAll`) and ghissues (`RunGH`) but omitted for ide.
**Fix:** Decide ide's seam: export `codeLauncher` as a settable `ideengine.CodeLauncher = vscode.Launch` swapped cross-package by `idecli`'s `cli_test.go` (or relocate `TestRunCLISpawnDispatch` into `ideengine`), and add it to the ideengine export list.

### [NOTE] configreg.go doc comment references removed `update`
**Section:** Scope › comment-accuracy sweep
**Issue:** `internal/configreg/configreg.go:4` says "used by init, update, and config CLI commands"; `update` is removed by the fold, so this comment goes stale, and configreg.go is already edited in this task (importer retargets) yet is not in the comment-sweep list.
**Fix:** Add `internal/configreg/configreg.go` to the comment-accuracy sweep (drop/replace the `update` mention).

## Verdict

GAPS_FOUND
The ide cli/engine split omits the `codeLauncher` test-seam handling that warp and ghissues received.
MILL_REVIEW_END
