MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-27
```

## Findings

### [NOTE] weft bypass is not wholly a PersistentPreRunE move
**Section:** Technical context → shared-pre-dispatch gotcha (weft)
**Issue:** The discussion says "preserve the bypass branch in the PersistentPreRunE," but weft's `--weft-path` path also forks the *push handler itself*: bypass calls `Push(weftPath, SyncOptions{})` directly (`cli.go:76`) while normal push does `Commit`+`Push` (`cli.go:123-132`) — so the push `RunE` must branch on the flag too, not only the PreRunE.
**Fix:** Note that only the resolution-skip + "non-push rejected" gate lives in PreRunE; the bypass `Push`-only call stays in the push `RunE`, which selects behaviour on whether `--weft-path` is set.

## Verdict

APPROVE
Scope, decisions, and constraints are complete and verified against source; one non-blocking nuance.
MILL_REVIEW_END