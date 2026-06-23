I have now reviewed all files thoroughly. Let me compile my findings.

**Summary of what I verified:**

1. **Batch 1 (network-tests-removal):** Both deleted files are confirmed absent. `doc.go` references only `git_test.go` and `sync_test.go`. `board-performance.md` has a one-line note replacing the SyncGit content. `running-tests.md` no longer mentions "real GitHub remote". `cmd/testtiming/main.go` prints "real local git" not "+ network".

2. **Batch 2 (board-skip-seam-parallelize):** `Config` has `SkipGit`/`SkipPush`; `Board` stores and threads them. `applySkipEnv` is in `cli.go` and is the single `os.Getenv` site. `Sync(boardPath, skipGit, skipPush bool)` and `CommitPush(..., skipPush bool)` have correct signatures. `git.go` no longer imports `os`. All board tests use `cfg.SkipGit/SkipPush` instead of `t.Setenv`. All `boardtest` tests call `t.Parallel()`. `skipenv_internal_test.go` is `package board` and tests `applySkipEnv` in white-box style.

3. **Batch 3 (lean-fixture-worktree):** `CopyPairedLocal` exists in `lyxtest.go`, omits weft-bare copy and doesn't rewrite weft-prime origin. All 10 `SkipPush:true` call sites in worktree tests use `CopyPairedLocal`. `TestWeftSpawnPushesWeftBranch` uses full `CopyPaired`.

4. **Batch 4 (record-timing):** `test-suite-timing.md` has a dated block with equivalence guardrail, per-package table, wall-clock numbers, and the "Current best times" section updated.

MILL_REVIEW_BEGIN
# Review: Speed up and stabilize the integration test tier — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-23
```

## Findings

### [NIT] Stale comment in add_test.go references CopyPaired, not CopyPairedLocal
**Location:** `C:\Code\loomyard\wts\optimize-integration-tier\internal\worktree\add_test.go:32`
**Issue:** The struct field comment says "setup performs scenario-specific prep on top of the fresh CopyPaired fixture" but every call site was switched to `CopyPairedLocal` in card 12.
**Fix:** Change the comment to reference `CopyPairedLocal`.

### [NIT] skipenv_internal_test.go has no cfg.SkipGit=true preservation case
**Location:** `C:\Code\loomyard\wts\optimize-integration-tier\internal\board\skipenv_internal_test.go:58-75`
**Issue:** Card 10 requires testing that an already-true field is not cleared by an unset env var; the table covers `cfg.SkipPush=true` but has no parallel `cfg.SkipGit=true` case.
**Fix:** Add a `{name: "cfg.SkipGit=true, env unset", cfgSkipGit: true, wantSkipGit: true}` row for symmetry and complete coverage.

## Verdict

APPROVE
Implementation is correct and complete across all four batches; two minor nits, no blocking issues.
MILL_REVIEW_END
