MILL_REVIEW_BEGIN
# Review: board: use gitrepo as its git operator

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [NOTE] doc.go Push-surface still asserts "no add -A"
**Section:** wildcard-stage-method-shape / Documentation
**Issue:** `internal/gitrepo/doc.go` asserts wildcard staging never enters gitrepo in TWO places — the Scope-boundaries section AND the Push-surface section ("so a wildcard `add -A` never enters gitrepo"); the discussion only names the Scope-boundaries section for update.
**Fix:** Note that both statements must be reconciled with `StageAllAndCommit`, not just the Scope-boundaries line, so the second one does not go stale/false.

### [NOTE] sync.go file-level doc describes removed hasUnpushed path
**Section:** Technical context / Documentation Lifecycle
**Issue:** `sync.go`'s package/file comment ("pushes all unpushed commits, looping until nothing is left") describes the `hasUnpushed` short-circuit the migration deletes; each iteration now pushes unconditionally (the acknowledged no-op-push consequence).
**Fix:** Note that `sync.go`'s own doc comment needs updating alongside the `board.go` package-doc fold, so it reflects unconditional-push-per-iteration.

## Verdict

APPROVE
Round-1 locking gap resolved; scope, decisions, and testing are coherent — only minor doc-consistency notes remain.
MILL_REVIEW_END
