# PATTERN-gogit-read-helper

Every go-git read in `internal/gitrepo` runs through `readGoGit` in `gogit.go`, the only production caller of `goGit()`.

- A cached go-git handle can read a stale pack index after a repack, so a read that fails with `plumbing.ErrObjectNotFound` is retried whole once, after a reindex, instead of per object lookup.
  A `Log` walk, a tree diff or `Worktree.Status()` reaches objects no single lookup wrapper covers.
- `readGoGit` runs the read under the shared lock.
  On object-not-found it takes the exclusive lock around the pack-fingerprint check alone, compares against the fingerprint snapshotted before the read began, and reindexes only when the pack set changed and no concurrent caller already reindexed for it.
  The read then reruns under the shared lock.
  Comparing against the snapshot, never against the last stored fingerprint alone, is what lets a caller that lost the race to a concurrent reindex still rerun.
- The retry happens at most once per call, only on object-not-found, so a genuinely absent object costs at most one reindex while the pack set stays unchanged.
- A read closure resolves everything it needs from the handle it is given and calls no other `Repo` method, because the lock is not reentrant.
  A method that expects a missing object maps the not-found after `readGoGit` returns.
- Enforced by `internal/gitrepo/gogitcaller_test.go`, an AST scan that fails when any function other than `readGoGit` calls `goGit()`.
  The regression test for the stale index is `TestMixedBackend_RepackBetweenCommitAndRead`.
