# PATTERN-gitrepo-client-boundary

Running git or writing remote-tracking refs goes through `gitexec` and `gitrepo`, raw only at pinned sites, with fetch, pull and push under the remote-refs lock.

## Git client boundary

Rule: Running git: `gitexec.Run`/`runChecked`, raw only at pinned `//gitexec:raw` sites; in `internal/gitrepo`, local reads via `readGoGit` and `dotgit`; transients via `.git/info/exclude`, never `git add -f`. (test)

`internal/gitrepo` splits local from remote by client: go-git and the `dotgit` geometry read own local reads, and `gitexec` owns anything remote-authenticating or working-tree-mutating.

- The parity oracle `internal/gitrepo/internal/gitoracle` imports only the standard library and `gitexec`, never `gitrepo`, enforced by its leaf test.

### Checked calls

Running git goes through `gitexec.Run` or `runChecked`.
A raw `RunGit` or `r.run` call stands only at a site pinned with a `//gitexec:raw` marker.

### Never force-add

A transient file is kept out of the index through `.git/info/exclude`, and lym never runs `git add -f`.
`internal/gitrepo/noforceadd_test.go` enforces it.

### The go-git read helper

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

## Remote-refs lock

Rule: Writing remote-tracking refs by fetch, pull or push: under the repository's remote-refs lock via `gitrepo`, waiting while held; the pause gate, writes that take none and judges that hold it per the background. (test)

A fetch, pull or push that writes remote-tracking refs writes them under the repository's remote-refs lock, taken through `gitrepo`, and waits while another process holds it.

- The process passes the pause gate before it takes the lock, and its network calls all pass the gate at once until it releases the lock.
  A paused process therefore never holds the lock, and no other process, an exempt cancel included, waits on a pause through it.
- A push sends to the remote's push URL, which writes no remote-tracking ref, so its transfer, retries included, runs outside the lock.
  Once the transfer succeeded, the push takes the lock to write the branch's tracking ref, and the upstream setting of a first push.
  A tracking ref a later fetch already moved past the pushed commit stays where it is.
- A tag push, a `--refmap=` fetch and a probe-clone fetch write no remote-tracking ref and take no lock.
- A judge of origin against tracking refs holds the lock from reading origin to its verdict, so no tracking-ref write lands between the read and the verdict.
  A push transfer that lands meanwhile is judged against the origin read before it.

A test enforces it.
