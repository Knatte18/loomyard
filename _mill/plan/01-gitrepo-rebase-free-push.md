# Batch: gitrepo-rebase-free-push

```yaml
task: 'fabric: warp-side commit lock + push coalescing'
batch: gitrepo-rebase-free-push
number: 1
cards: 2
verify: go test -tags integration ./internal/gitrepo/... ./cmd/lyx/
depends-on: []
```

## Batch Scope

Adds a rebase-free push primitive to `internal/gitrepo` that the fabric async coalescing loop (batch 2) consumes: a plain `git push` that never runs `git pull --rebase` and returns a distinguishable sentinel on a non-fast-forward rejection. This is the foundation the whole slice sits on. Because the new method adds an `r.run` call site inside `gitrepo`, the gitrepo Client Boundary Invariant requires updating both the pinned boundary test and the `CONSTRAINTS.md` entry in the SAME commit as the method — card 1 does all three together so no intermediate commit leaves the boundary guard red. The external interface batch 2 consumes: `func (r *Repo) PushRebaseFree() error` and `var ErrPushRejected error`.

Batch-local decision: the sentinel-error shape (`ErrPushRejected`) is chosen over a `(rejected bool, err error)` return, per discussion `### rebase-free-async-push` which permits either; the sentinel composes with `errors.Is` at the fabric call site with no extra return value.

## Cards

### Card 1: Add `PushRebaseFree` + `ErrPushRejected` to gitrepo, pin the boundary, update CONSTRAINTS

- **Context:**
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/push.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/gitrepo/push.go`, add an exported sentinel `var ErrPushRejected = errors.New("gitrepo: push rejected (remote diverged)")` (add the `errors` import). Add its godoc explaining it signals a non-fast-forward rejection distinguishable from a genuine failure.
  - In `internal/gitrepo/push.go`, add `func (r *Repo) PushRebaseFree() error`: run `r.run("-c", "push.autoSetupRemote=true", "push")`; on a non-nil spawn `err` return it; on `code == 0` return nil; on a non-zero code whose stderr matches `containsAny(stderr, rebaseRetryTriggers)` return `ErrPushRejected`; on any other non-zero code return `fmt.Errorf("gitrepo: git push: %s", stderr)`. Reuse the existing package-level `rebaseRetryTriggers` and `containsAny` (both in this file). Do NOT call `pull --rebase`, acquire any lock, or create `.gitrepo-push.lock`. Godoc must state: single plain push, never `pull --rebase`; establishes upstream on first push via `push.autoSetupRemote=true`; returns `ErrPushRejected` (checkable via `errors.Is`) on non-fast-forward, wrapped error otherwise; lock-free because callers that need serialization provide their own absorbing lock (per discussion `### no-host-root-gitrepo-push-lock`).
  - In `cmd/lyx/gitrepoboundary_test.go`, add `"PushRebaseFree": true,` to the `gitrepoPinnedRunBoundMethods` map — the new method contains an `r.run(` call and the boundary guard asserts set-equality on the `r.run`-bound method set. (The `gitexecTotal == 1` assertion is unaffected: `PushRebaseFree` calls `r.run`, not `gitexec.` directly.)
  - In `CONSTRAINTS.md`, under `## gitrepo Client Boundary Invariant` → Statement bullet, add `PushRebaseFree` to the enumerated CLI-bound method set (the list currently naming `StageAndCommit, StageAllAndCommit, Push, PushCoalesced, Pull, ResetHard, CheckoutDetached, RestoreBranch, SetSnapshotSHA's push, SnapshotSHA's fetch, and hasUnpushed`), so the pinned list and the doc stay in lockstep as the invariant requires.
- **Commit:** `feat(gitrepo): add rebase-free PushRebaseFree with ErrPushRejected sentinel`

### Card 2: Integration test for `PushRebaseFree`

- **Context:**
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/push_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Extend `internal/gitrepo/push_test.go` (package `gitrepo_test`, already `//go:build integration`) with tests for `PushRebaseFree`, reusing the file's existing `newBareRemote` / `newRepoWithRemote` fixture helpers and `lyxtest.MustRun`.
  - Assert: (a) a first push against a no-upstream checkout succeeds and establishes the tracking branch (proving `push.autoSetupRemote=true` is applied) and the bare remote's HEAD advances to match the local HEAD; (b) against a diverged remote (a second clone pushed a commit the first clone lacks), `PushRebaseFree` returns an error satisfying `errors.Is(err, gitrepo.ErrPushRejected)`, the local working tree is unchanged (no `pull --rebase` ran — assert a dirty tracked file left in place is untouched, and no rebase-in-progress state), and the local HEAD is unchanged.
  - Do not assert internal git command strings; assert observable ref/tree state, matching the existing push tests' style.
- **Commit:** `test(gitrepo): cover PushRebaseFree first-push and rejection paths`

## Batch Tests

`verify: go test -tags integration ./internal/gitrepo/... ./cmd/lyx/` runs the new `PushRebaseFree` integration tests (tagged, in `internal/gitrepo`) plus `cmd/lyx/gitrepoboundary_test.go`'s `TestGitrepoBoundary_PinnedRunCallSites` (untagged, in `cmd/lyx`), which must stay green after card 1's pinned-set edit. The `./cmd/lyx/` scope is included specifically because the boundary guard lives there, not in `internal/gitrepo`; it is a fast untagged run. Card 1 and card 2 are separate commits but the boundary/CONSTRAINTS edits are folded into card 1's single commit so the guard is never transiently red.
