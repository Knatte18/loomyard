I have all the evidence I need. Let me compile my findings.

Key issues identified:

1. **`board.go:43` comment is stale** — the comment still says "unless BOARD_SKIP_GIT=1" but `writeOp` now checks `!b.skipGit` (the resolved flag), not env directly. Minor doc inconsistency, not a blocking bug.

2. **`skipenv_internal_test.go:80-85` — "env unset" cases don't neutralize ambient env** — When `tt.skipGitEnv == ""`, the test does NOT call `t.Setenv("BOARD_SKIP_GIT", "")`. If the test binary runs with an inherited ambient `BOARD_SKIP_GIT=1`, the "both env unset" and "cfg.SkipPush=true, env unset" cases will see the env set and `applySkipEnv` will return `SkipGit=true` — failing the assertion `wantSkipGit: false`. The plan (card 10) says `skipenv_internal_test.go` legitimately uses `t.Setenv` to assert env-resolution behaviour, but it needs to explicitly clear the env for the "unset" cases too, not just skip the call.

3. **`add_test.go` comment/doc discrepancy** — line 6-7 says "tests build this via lyxtest.CopyPaired and pass AddOptions{SkipPush:true}" but the implementation correctly uses `CopyPairedLocal`. Minor stale comment, NIT.

4. **`test-suite-timing.md` timing numbers suggest the implementation slowed Tier 2 from ~42s to ~65s** — this is not a bug per se, the document explains the floor shift, but the plan's stated goal was to "speed up" the integration tier. The net wall-clock regressed. This is documented and explained in the timing block, and the plan explicitly says "no hard target", so this is within plan. Not a finding.

5. **`remove_test.go:127` stale comment** — says "weft repo exists (from CopyPaired)" but the fixture is now `CopyPairedLocal`. Minor NIT.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-23
```

## Findings

### [BLOCKING] skipenv test does not neutralize ambient env for "unset" cases

**Location:** `internal/board/skipenv_internal_test.go:78-85`
**Issue:** The test guard `if tt.skipGitEnv != "" { t.Setenv(...) }` means cases with `skipGitEnv: ""` (e.g. "both env unset", "cfg.SkipPush=true env unset") never call `t.Setenv("BOARD_SKIP_GIT", "")` to clear an ambient `BOARD_SKIP_GIT=1`. If the binary runs under an inherited `BOARD_SKIP_GIT=1` (the exact ambient-env-leakage scenario the plan was designed to fix), `applySkipEnv` returns `SkipGit=true` and those cases fail `wantSkipGit: false`.
**Fix:** For every case where `tt.skipGitEnv == ""`, explicitly call `t.Setenv("BOARD_SKIP_GIT", "")` to guarantee the env is cleared, mirroring how the populated cases call `t.Setenv` when non-empty.

### [NIT] writeOp comment references the removed env seam

**Location:** `internal/board/board.go:43`
**Issue:** The comment on `writeOp` still reads "unless BOARD_SKIP_GIT=1" but `writeOp` now checks `!b.skipGit` (the resolved cfg flag), not env — env is never consulted here after the card-3 change.
**Fix:** Change the parenthetical to "unless `b.skipGit` is set" or similar.

### [NIT] add_test.go package comment refers to CopyPaired after migration

**Location:** `internal/worktree/add_test.go:6-7`
**Issue:** File comment says "tests build this via lyxtest.CopyPaired" but the implementation now uses `CopyPairedLocal` at every call site in the file.
**Fix:** Update the comment to reference `CopyPairedLocal`.

### [NIT] remove_test.go stale comment references CopyPaired

**Location:** `internal/worktree/remove_test.go:127`
**Issue:** Comment inside the `NonexistentSlug` setup closure says "weft repo exists (from CopyPaired)" but the fixture is now `CopyPairedLocal`.
**Fix:** Change to "weft repo exists (from CopyPairedLocal)".

## Verdict

REQUEST_CHANGES
One blocking issue: `skipenv_internal_test.go` does not clear ambient env for "unset" cases, making it fragile under inherited `BOARD_SKIP_*=1`.
MILL_REVIEW_END
