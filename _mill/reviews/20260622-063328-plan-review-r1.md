The remaining ide test files (spawn_test, vscode_test) do not spawn git; color_test's single match is incidental. So gating cli_test+menu_test does make `internal/ide` Tier-1 offline as the plan claims.

I have thoroughly verified the plan.

MILL_REVIEW_BEGIN
# Review: Optimise and slim the rest of the test suite — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-22
```

## Findings

### [NIT] Card 6 fallback trigger understates the tasks.json mismatch
**Location:** board-fixtures / Card 6
**Issue:** `CopyWeft` seeds `_lyx/config.yaml` only (no `tasks.json`), while the sync tests' `dirty`/`newSyncRepo` write and count `tasks.json`; the clean-tree no-op also depends on the initial `.gitignore`-only commit, so the `CopyBoardRepo` fallback is more likely needed than the "only if" phrasing implies.
**Fix:** None required — the fallback (`CopyBoardRepo` seeded with `tasks.json`) is fully specified; this is just a likelihood note for the implementer.

### [NIT] Card 8 RunCLI reachability vs CopyHostHub layout
**Location:** ide-fixtures / Card 8
**Issue:** `RunCLI` calls `paths.Resolve(cwd)` then `Spawn`; `CopyHostHub` yields a bare hub with no `_lyx`, so spawn-dispatch may resolve differently than the current bare `newTestGitRepo` — the assertions only check the dispatch path was reached, so this likely holds, but it is the exact case the fallback exists for.
**Fix:** None — Card 8 already authorises reverting to `newTestGitRepo` gated-but-unmigrated; behaviour is correctly hedged.

### [NIT] Batch 4 verify scope excludes boardtest
**Location:** board-prune / batch verify
**Issue:** `verify: go test ./internal/board` (no `/...`) skips `boardtest`, which is correct for Tier-1 unit folds but means the relocated integration tests aren't re-run here.
**Fix:** None — intentional and covered by batch 5's whole-repo two-tier gate.

## Verdict

APPROVE
DAG, numbering, constraints, and helper-collision claims all verify against source; risks are hedged with explicit fallbacks.
MILL_REVIEW_END