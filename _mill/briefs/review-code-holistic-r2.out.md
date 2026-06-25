MILL_REVIEW_BEGIN
---
verdict: REQUEST_CHANGES
summary: >
  The implementation is well-structured, thorough, and coherent across all 9 batches.
  One blocking correctness bug was found in cleanup.go: the orphan-check fails
  silently for any deployment where BranchPrefix is non-empty, making live weft branches
  appear as orphans under cleanup --apply. The test suite masks this with BranchPrefix: "".
  One NIT regarding integration build tags on gitexec_test.go is noted.
---

findings:

- id: CLEANUP-BRANCH-PREFIX-MISMATCH
  severity: BLOCKING
  file: internal/warp/cleanup.go
  lines: 91-110
  title: Orphan-check fails for non-empty BranchPrefix — live branches deleted by cleanup --apply
  detail: |
    `Cleanup` builds `hostSlugs` by taking `filepath.Base(hostPath)` for each registered
    host worktree. For a worktree at `.../hub/my-task`, the slug key is `"my-task"`.

    `Add` names the weft branch as `w.cfg.BranchPrefix + slug` (add.go line 89).
    When BranchPrefix is `"hanf/"`, the weft branch is named `"hanf/my-task"`.

    The orphan check on cleanup.go line 110 is:

        if hostSlugs[branch] {

    where `branch` is the full weft branch name from `listWeftBranches`. With any
    non-empty BranchPrefix, `hostSlugs["hanf/my-task"]` is `false` even when
    `"my-task"` is a live, registered host worktree. Every live weft branch therefore
    appears as an orphan, and with `--apply --force` all of them are deleted.

    The test `TestCleanup_LiveBranchNeverDeleted` (cleanup_test.go line 232) explicitly
    uses `Config{BranchPrefix: ""}`, so the bug is not exercised.

    Correct fix: strip the BranchPrefix from the weft branch name before the `hostSlugs`
    lookup. The lookup key should be `strings.TrimPrefix(branch, w.cfg.BranchPrefix)`.
    A companion test with `Config{BranchPrefix: "hanf/"}` and a live pair must be added.

- id: GITEXEC-INTEGRATION-TAG
  severity: NIT
  file: internal/gitexec/gitexec_test.go
  lines: 1
  title: Integration build tag on gitexec_test.go — batch 1 verify command silently skips tests
  detail: |
    The entire `gitexec_test.go` file carries `//go:build integration`. The batch 1
    verification step instructs running `go test ./internal/gitexec/` without
    `-tags integration`, so zero tests execute. No code defect; the build tag is the
    correct choice for this package, but the verify command should include
    `-tags integration`.
MILL_REVIEW_END
