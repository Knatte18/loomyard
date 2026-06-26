MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\warp-test-speedup\_mill\discussion.md
date: 2026-06-26
```

Verified against source: `internal/lyxtest/lyxtest.go` (strip points `initRepo`/`initBareRemote` are correct; bare uses `hooks/` not `.git/hooks/`, which the decision names; leaf invariant holds, only `os`/`filepath` needed). Confirmed every cited test exists in `internal/warp/*_test.go`, that `TestWeftPrechecksHardRequireWeftRepo` is byte-for-byte the same setup/assertions as `TestAdd/NoWeftRepo` (group E valid), that hook tests reference no `*.sample` (strip is safe), and that `CopyWeft` is unused in the warp suite (inventory CopyPairedLocal/CopyPaired/CopyHostHub is complete). Decisions all carry rationale + rejected alternatives; scope in/out, constraints, failure modes, testing, and operator-run verification are all addressed.

## Findings

### [NOTE] Subtest names in tables don't match decision labels
**Section:** Decisions — group E, keep-list
**Issue:** The discussion writes `TestWeftPrechecks/HardRequireWeftRepo` and `RejectExistingWeftWorktree`, but the actual `t.Run` names are `TestWeftPrechecksHardRequireWeftRepo` / `TestWeftPrechecksRejectExistingWeftWorktree` (prefix repeated in the table `name` field).
**Fix:** Plan writer should target the literal subtest names to avoid mis-`-run` selection.

### [NOTE] Group A fold needs a per-case assertion hook
**Section:** Decisions — consolidate group A
**Issue:** `TestAdd` is table-driven (`add_test.go:30`) with no per-case assertion field; folding `TestAddDormant` + three `TestWeftSpawn*` assertions into the `HappyPath` row requires extending the struct (e.g. an `extraAssert func`) or a HappyPath-guarded block in the shared body — not a drop-in.
**Fix:** Note this mechanism in the plan so the fold doesn't apply weft-spawn assertions to every TestAdd row.

## Verdict

APPROVE
Decisions are source-grounded, fully reasoned, and self-contained; only minor naming/mechanism notes remain.
MILL_REVIEW_END
