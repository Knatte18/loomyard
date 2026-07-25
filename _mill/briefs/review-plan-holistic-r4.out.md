MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-25
```

## Findings

### [NIT] Golden-fixture clean case needs referenced target files present
**Location:** Batch 2, Card 9
**Issue:** The worked-example golden fixture is used as the zero-findings clean case, but its cards reference on-disk target paths (`internal/boardcli/list.go`, `internal/boardengine/rows.go`, `list_test.go`, `//internal/output/envelope.go`, `//cmd/lyx/helptree_test.go`) and a `Moves:` dest `rowsjson.go`; unless the `t.TempDir()` worktreeRoot materializes the sources/edits/context files and omits `rowsjson.go`, `path-missing`/`move-source-missing`/`move-target-collision` fire and "zero findings" is false. Card 9's "populated with the fixture's real on-disk files" is ambiguous about these target files vs. the plan `.md` files.
**Fix:** Have Card 9 state explicitly that the tempdir must contain the referenced Edits/Context/Moves-source target files and must NOT contain the `Moves:` destination, so the happy-path yields zero findings.

### [NIT] Card 25 does not call out updating stale state.go header doc
**Location:** Batch 7, Card 25
**Issue:** `state.go`'s package doc comment (and the `BatchState.Digest`/`State.ChainStartSHAs` field docs) describe "each deferred-verify chain's rollback anchor SHA" and the `builderengine.Digest` carry-forward; the card drops those fields/imports but does not mention reconciling the now-stale doc comment in the same file it edits.
**Fix:** Add a note to Card 25 to update `state.go`'s header/field godoc to drop chain/deferred-verify and builderengine.Digest prose alongside the schema change.

## Verdict

APPROVE
Plan is complete, constraint-aligned, and DAG-sound; only two minor precision gaps.
MILL_REVIEW_END
