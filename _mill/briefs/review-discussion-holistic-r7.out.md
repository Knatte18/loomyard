MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-29
```

## Findings

### [NOTE] board/weft importer lists omit test-file importers
**Section:** Technical context (board, weft per-module importer lists)
**Issue:** `internal/initcli/initcli_test.go` imports `board`+`warp`+`weft` and `internal/configreg/configreg_test.go` imports `weft`, but the board/weft "Importers" lists only name configreg, ide/menu.go, configcli, and main.go — these test importers are unlisted (only warp's list is declared "exhaustive for production + test consumers").
**Fix:** Note that `initcli_test.go` (board, weft) and `configreg_test.go` (weft) also retarget; the build-green-after-each-batch gate is the explicit backstop, so this is completeness only, not a blocker.

## Verdict

APPROVE
All prior-round gaps resolved; verified seam/placement/export claims against source; one minor importer-completeness note.
MILL_REVIEW_END
