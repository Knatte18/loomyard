# Batch: push

```yaml
task: 'gitrepo: generic, repo-agnostic git primitives'
batch: push
number: 2
cards: 3
verify: go test -tags integration ./internal/gitrepo/
depends-on: [1]
```

## Batch Scope

Delivers the push surface in a new `push.go`: `Push()` (single synchronous push with rebase-retry)
and `PushCoalesced()` (single-pusher lock + loop-until-nothing-unpushed, the board `sync.go`
replacement), sharing one rebase-retry helper, plus `push_test.go`. Both are push-only; committing
is the caller's `StageAndCommit` from batch 1. This batch touches only new files
(`push.go`/`push_test.go`) and reads `Repo` from batch 1 — it never edits `gitrepo.go`, so it runs
in parallel with the snapshot batch without file overlap.

## Cards

### Card 5: Push with shared rebase-retry helper

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/boardengine/git.go`
  - `internal/boardengine/sync.go`
  - `internal/gitrepo/gitrepo.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/push.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Repo) Push() error`: run `git push`; on success return nil; on a
  non-zero exit inspect stderr and, if it contains any of `non-fast-forward`, `rejected`, or
  `fetch first`, run `git pull --rebase` — on rebase failure run `git rebase --abort` and return an
  error; on rebase success retry `git push` once and return its result; any other push failure
  returns an error including stderr. Factor the retry logic into an unexported
  `func (r *Repo) pushWithRebaseRetry() error` reused by `PushCoalesced` in card 6. Document (in
  the method's doc comment) the **rebase-retry precondition**: `pull --rebase` aborts on dirty
  *tracked* files, so a clean tree w.r.t. tracked files is the caller's responsibility — `gitrepo`
  does not auto-stash.
- **Commit:** `feat(gitrepo): add Push with rebase-retry`

### Card 6: PushCoalesced with single-pusher lock

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/boardengine/sync.go`
  - `internal/lock/lock.go`
  - `internal/gitrepo/gitrepo.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/gitrepo/push.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `const pushLockFile = ".gitrepo-push.lock"` (package-level constant, the
  pinned repo-agnostic lock-file name from discussion.md). Add
  `func (r *Repo) PushCoalesced() error`: acquire the single-pusher lock with
  `lock.AcquireWriteLock(filepath.Join(r.path, pushLockFile))` (blocking; `defer` its `Release()`),
  then loop — call `hasUnpushed()`; if false, break; otherwise call `pushWithRebaseRetry()` and, on
  error, return it — repeating so a commit landing mid-push is caught. Add
  `func (r *Repo) hasUnpushed() (bool, error)`: run `rev-list --count @{u}..HEAD`; a non-zero exit
  means no upstream is configured → return `true` (so the first push, which sets upstream, still
  happens); otherwise return whether the trimmed count is not `"0"`. `gitrepo` does **not** manage
  `.gitignore` for the lock file (it is never staged under explicit-only staging).
- **Commit:** `feat(gitrepo): add PushCoalesced with single-pusher lock`

### Card 7: push and coalescing tests

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/warpengine/clone_integration_test.go`
  - `internal/boardengine/boardtest/git_test.go`
  - `internal/gitrepo/push.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/push_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`, package `gitrepo_test`. Two distinct fixtures, kept
  separate per discussion.md: **(a) cross-clone conflict / rebase-retry** — a bare remote
  (`git init --bare`) plus two clones; commit+push from clone A, then a commit in clone B whose
  `Push()` hits a non-fast-forward and recovers via the rebase-retry, ending with both commits on
  the remote; also assert a first `Push()` with no upstream succeeds and sets upstream.
  **(b) lock-blocking / coalescing** — a single clone (one worktree, one `.gitrepo-push.lock`):
  make several commits, run `PushCoalesced()` concurrently from two goroutines, and assert they
  serialize on the lock and everything lands unpushed→pushed with no error (the second finds
  nothing unpushed and returns). Also assert the **rebase-retry precondition**: with a dirty
  *tracked* file present, the rebase-retry path aborts and `Push()` returns an error (caller
  precondition, not a `gitrepo` bug). Do not conflate the two fixtures — two clones have two lock
  files and cannot exercise lock-blocking.
- **Commit:** `test(gitrepo): push and coalescing tests with clone and lock fixtures`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/` runs `push_test.go` alongside batch 1's
tests under the hermetic `TestMain`. The two-fixture split (one clone for lock-blocking, two clones
for cross-clone conflict) is the crux the plan reviewer should confirm is honored.
