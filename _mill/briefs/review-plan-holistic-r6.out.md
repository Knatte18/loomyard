MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-08
```

## Findings

### [NIT] pull.go's RelPath-blind-scope paragraph leaves a stale identifier name
**Location:** batch 2, card 7 **Issue:** The "RelPath-blind scope (documented limitation)" doc comment names the private const `patternDirName` ("the pathspec is the bare patternDirName..."); card 7 only instructs "adjusting... the path names inside" this paragraph, not explicitly the deleted identifier's own name. **Fix:** Explicitly note in card 7 that this paragraph's `patternDirName` mention must become `pattern.PathspecFile`/`PathspecDir`, alongside the `_pattern` path-text fix already called out.

### [NIT] add_test.go comment rewrite (card 21) only half-corrects a pre-existing inaccuracy
**Location:** batch 4, card 21 **Issue:** The comment at add_test.go:137-141 claims "_lyx/_pattern are rejected only via this injected pathspec"; per junctionnames.go's `IsReservedHubName`, `_lyx` is already unconditionally reserved via `structuralCommittedDirs` regardless of the injected pathspec, so that claim was already inaccurate for `_lyx` before this task — card 21 only flags the `_pattern` half as newly false. **Fix:** Have card 21 note the comment must also stop implying `_lyx`'s rejection depends on the injected pathspec.

## Verdict

APPROVE
Plan is exhaustively cross-checked against source (line numbers, DAG, decisions, file coverage all verified accurate); only trivial doc-comment nits remain.
MILL_REVIEW_END
