MILL_REVIEW_BEGIN
# Review: Extract yamlengine and migrate config via lyx update — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [BLOCKING] Batch 6 batch-depends omits batch 5
**Location:** 00-overview.md `batches:` (batch 6 `depends-on: [1, 3, 4]`); cards 19, 21
**Issue:** Card 19's `initcli_test` asserts `board.LoadConfig`/`worktree.LoadConfig`/`weft.LoadConfig` succeed on a freshly-init'd dir, and card 21 rewrites `cmd/lyx` fixtures "so strict `config.Load` (batch 5, Card 8) succeeds" — both require batch 5's engine-backed wrappers/strict Load, since the reconciled files use the new `${env:...}` grammar that the pre-batch-5 (old `$env:` regex) wrappers do not expand. Batch 5 is not in batch 6's depends-on, so the DAG permits batch 6 to land/verify without 5, where these assertions are unfounded.
**Fix:** Add `5` to batch 6's `depends-on` (→ `[1, 3, 4, 5]`).

### [NIT] Card 17 write-condition prose is ambiguous on precedence
**Location:** 06-lyx-update-init.md, Card 17
**Issue:** "when `apply` is true AND the file is absent OR `len(added)+len(removed) > 0`" is precedence-ambiguous; the intended logic is `apply && (absent || delta>0)`.
**Fix:** Parenthesize as `apply && (absent || len(added)+len(removed) > 0)`.

### [NIT] Card 5 init_test.go header comment will drift
**Location:** 04-templates-live-yaml.md, Card 5 (and the deletion in card 19)
**Issue:** Card 5 removes the "fully commented" loops but the plan does not mention updating `init_test.go`'s top doc comment ("board.yaml + worktree.yaml (fully commented)"); harmless since card 19 deletes the file, but in batch 4 the stale comment lingers.
**Fix:** Optionally note the comment update; non-load-bearing.

### [NIT] Migration gap for pre-existing weft worktrees not called out
**Location:** Card 14 / batch 5 scope
**Issue:** After batch 5 strict Load, `weft.LoadConfig` (weft/cli.go:95) errors on any existing weft worktree lacking `_lyx/config/weft.yaml`; `lyx update` must be run there too, but `update`/`init` operate on the host baseDir (`WorktreeRoot+RelPath`), not the weft sibling.
**Fix:** Confirm `lyx update` reaching the weft `_lyx` via the host junction is sufficient, or document the weft-side migration step.

## Verdict

REQUEST_CHANGES
Batch 6's depends-on must include batch 5; its key assertions rely on the batch-5 engine.
MILL_REVIEW_END