MILL_REVIEW_BEGIN
# Review: fabric: cutover -- rewire consumers onto fabric, delete warp/weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-07-26
```

## Findings

### [GAP] Stale in-code comments + grep-gate matching semantics
**Section:** Scope (docs list) / Testing (grep-clean gate) / Technical context (importer list)
**Issue:** The importer list dismisses non-import warp/weft hits as "comments," but several production comments carry the full deleted import path and are not scheduled for cleanup — `internal/codeintelcli/cli.go:34` (`internal/weftcli.Command`) and `internal/lyxtest/doc.go:2-3,10` (`internal/warpengine, internal/warpcli, internal/weftengine, internal/weftcli`) — plus bare-name comment refs in `perchengine/doc.go`+`engine.go`, `websterengine/audit.go`, `reedengine/config.go`, `lyxtest/hermetic.go`; a naive grep-clean gate matching `internal/weftcli` etc. would flag the full-path comments and fail the acceptance gate.
**Fix:** Add these stale comment fix-ups to the doc scope (the Documentation-Lifecycle "no rot" rule applies) and specify whether the grep-clean gate matches imports-only (AST/import-line scoped) or any occurrence.

### [NOTE] New() error dropped in the CommitWeft rewrite shorthand
**Section:** Technical context (call-site map row `undo.go:90`; "Proven pattern")
**Issue:** The table writes `f,_ := fabricengine.New(hostPath, weftWorktree)` discarding the error, but `New` returns `(*Fabric, error)` and yields a nil `*Fabric` when a path is absent (`requireDir`), so a discarded error becomes a nil-deref panic in `f.CommitWeft`; the cited reference `fabriccli/weft_verbs.go` actually checks that error.
**Fix:** State that each of the four rewrite sites must propagate `New`'s error (as `fabriccli` does), not discard it.

## Verdict

GAPS_FOUND
One gap: stale deleted-module comments and unspecified grep-gate scope may fail the acceptance gate.
MILL_REVIEW_END
