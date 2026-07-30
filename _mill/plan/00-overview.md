# Plan: fabric: warp-side commit lock + push coalescing

```yaml
task: 'fabric: warp-side commit lock + push coalescing'
slug: fabric-commit-lock-coalescing
approved: false
started: '2026-07-30T18:21:14Z'
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitrepo-rebase-free-push
    file: 01-gitrepo-rebase-free-push.md
    depends-on: []
    verify: go test -tags integration ./internal/gitrepo/... ./cmd/lyx/
  - number: 2
    name: fabricengine-coalescing-primitive
    file: 02-fabricengine-coalescing-primitive.md
    depends-on: [1]
    verify: go test -tags integration ./internal/fabricengine/...
  - number: 3
    name: fabric-commit-lock-and-wiring
    file: 03-fabric-commit-lock-and-wiring.md
    depends-on: [2]
    verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
  - number: 4
    name: boardengine-delegation
    file: 04-boardengine-delegation.md
    depends-on: [2]
    verify: go test -tags integration ./internal/boardengine/...
  - number: 5
    name: slice-3-design-doc-completion
    file: 05-slice-3-design-doc-completion.md
    depends-on: [3, 4]
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits. Full rationale for each lives in `_mill/discussion.md` under the matching `### <name>` heading; the discussion file is the authoritative design record and each batch builds directly on it._

### Decision: combined-commit-lock

- **Decision:** `Fabric.Commit` acquires one write lock (`.weft/weft.write.lock`, the existing `weftWriteLockFile`) whenever the call will actually commit something — the guard is `len(warpFiles) > 0 || weftSide` (where `weftSide == len(weftFiles) > 0 && !opts.SkipGit`). A fully degenerate no-op call (nothing on either side) takes no lock and runs no `ensureWeftLockDir`, exactly as today. This closes the warp-only unlocked race without introducing a second lock or lock-ordering.
- **Rationale:** See discussion `### combined-commit-lock`. `Fabric.Commit` is the sole warp-side writer via fabric, so one shared lock closes the only warp race; over-serializing a warp-only against an unrelated weft-only commit is negligible (both are fast local commits that rarely coincide).
- **Applies to:** fabric-commit-lock-and-wiring

### Decision: commit-lock-scoped-to-commit-only

- **Decision:** The combined write lock is held ONLY around the commit(s) — acquire, warp commit, weft commit, release — and is released BEFORE the async push child is spawned (`spawnDetachedPushFn`). The network push runs in the detached child under a SEPARATE absorbing push lock, never under the commit lock.
- **Rationale:** See discussion `### commit-lock-scoped-to-commit-only`. The commit lock protects a fast local critical section; holding it across the (even cheap) spawn couples it to the push concern the design keeps independent. This property is directly asserted by a test, so the release must be structural, not incidental to `defer` ordering.
- **Applies to:** fabric-commit-lock-and-wiring

### Decision: coalescing-loop-in-fabricengine-via-closures

- **Decision:** `internal/fabricengine` owns a generic coalescing primitive `CoalescePush(lockPath string, step func() (progressed bool, err error)) error`: it `AcquireWriteLock`s `lockPath` ONCE, holds it across the whole loop, calls `step` repeatedly, repeats while `step` returns `progressed == true`, and exits (releasing the lock) on the first `progressed == false` or on a non-nil `err` (which propagates). The primitive owns the absorbing lock; the caller supplies only the lock PATH and the step closure — never a stage/message data bundle. Board passes its existing `board.push.lock` path (name/location unchanged; only the acquire site moves into the primitive) plus a step returning `progressed = committed`; fabric passes a new `.weft/`-based push-lock path plus a two-sided rebase-free push step returning `progressed = (warp HEAD advanced) OR (weft HEAD advanced)` since that iteration's pre-push snapshot.
- **Rationale:** See discussion `### coalescing-loop-in-fabricengine-via-closures`. The `progressed`-bool return is the single exit contract unifying board's commit-driven terminator and fabric's HEAD-stability terminator; the rebase policy lives entirely in each caller's step closure, not in the loop.
- **Applies to:** fabricengine-coalescing-primitive, fabric-commit-lock-and-wiring, boardengine-delegation

### Decision: push-only-loop-exit-on-head-stability

- **Decision:** The fabric async push step is push-only (no commit step). Each iteration snapshots warp HEAD and weft HEAD, rebase-free-pushes each side that has commits, re-reads both HEADs, and returns `progressed = (warp HEAD moved) OR (weft HEAD moved)` during the push window. A side whose HEAD is unborn (`gitrepo.ErrNoCommits`) is skipped for that iteration (nothing to push). The loop never loops on raw `hasUnpushed` — that is the documented infinite-spin hazard.
- **Rationale:** See discussion `### push-only-loop-exit-on-head-stability`. Keying exit on "did a new local commit appear during the last push" both absorbs genuinely-new concurrent work and terminates deterministically (a rejected or no-new-work push leaves HEAD unmoved → exit).
- **Applies to:** fabricengine-coalescing-primitive, fabric-commit-lock-and-wiring

### Decision: rebase-free-async-push

- **Decision:** The fabric async push uses a NEW `gitrepo` rebase-free push primitive `(*Repo).PushRebaseFree()` — a plain `git push -c push.autoSetupRemote=true`, never `git pull --rebase`. On a non-fast-forward rejection it returns the exported sentinel `gitrepo.ErrPushRejected` (checkable via `errors.Is`); any other failure returns a wrapped error. The fabric push step maps `ErrPushRejected` to `progressed = false, err = nil` plus a `logger.Warn` line (commits left unpushed), so the loop exits cleanly; a genuine error (network/auth) propagates. Board's push step keeps the existing rebase-retry path (`PushWeftAt` → `PushCoalesced` → `pushWithRebaseRetry`), unchanged.
- **Rationale:** See discussion `### rebase-free-async-push`. Only `git pull --rebase` mutates the working tree; a plain push never does, so an async rebase-free push is safe to run while the calling thread keeps editing warp. Remote reconciliation is slice 6, out of scope here.
- **Applies to:** gitrepo-rebase-free-push, fabricengine-coalescing-primitive, fabric-commit-lock-and-wiring

### Decision: no-host-root-gitrepo-push-lock

- **Decision:** The fabric async coalescing push uses the lock-free `PushRebaseFree` for BOTH warp and weft (serialization comes from fabric's own absorbing push lock under `.weft/`), so `gitrepo.PushCoalesced`'s `.gitrepo-push.lock` is never created at the pristine host (warp) worktree root on the warp-via-fabric path. `gitrepo.PushCoalesced` stays in use unchanged for board (`board.push.lock` in the board dir) and for the synchronous `PushWeftAt`/`PushWeft` weft path (weft-side, git-excluded). No `gitrepo` lock-placement parameter is added and no host-side git-exclude is seeded.
- **Rationale:** See discussion `### lock-artifacts-never-at-worktree-root`. Eliminating the artifact on the fabric path is cleaner than relocating a geometry-blind `gitrepo` lock and touches no `gitrepo` boundary.
- **Applies to:** gitrepo-rebase-free-push, fabricengine-coalescing-primitive, fabric-commit-lock-and-wiring

### Decision: lock-artifact-under-weft

- **Decision:** The new absorbing push lock lives under `.weft/`, named by a pinned constant `weftPushLockFile = "fabric.push.lock"` added alongside `weftWriteLockFile` in `weftgit.go` — never an inline literal, never at a worktree root. No new git-exclude entry is added: `seedWeftArtifactExcludes` already seeds the whole-directory exclude `weftLockDirName + "/"` (`.weft/`) at `weftgit.go:131`, which covers this new lock file. The constant is still required for lock-PATH construction.
- **Rationale:** See discussion `### lock-artifacts-never-at-worktree-root` (as amended in the round-2 fixer report: the constant is needed, the extra exclude is redundant). `.weft` is fabricengine's own lock-dir name (not a `hubgeometry` geometry token), so constructing under it does not implicate the Hub Geometry Invariant.
- **Applies to:** fabricengine-coalescing-primitive

### Decision: go-test-verify-no-pythonpath

- **Decision:** This is a Go module; every `verify:` command is a native `go test`/`go build` invocation with NO `PYTHONPATH= ` prefix (that prefix is Python/mill-repo-specific and the `verify-not-isolated` validator check is language-conditional). Concurrency and real-git tests are `//go:build integration` tagged (Test Tier Purity Invariant); pure closure/loop-skeleton tests stay untagged. The hub root equals the git root here, so plain-string `verify:` (implying `cwd: git_root`) is correct — no `cwd: hub` mapping is needed.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/gitrepoboundary_test.go`
- `internal/boardengine/sync.go`
- `internal/boardengine/sync_integration_test.go`
- `internal/boardengine/testmain_test.go`
- `internal/fabricengine/coalesce.go`
- `internal/fabricengine/coalesce_integration_test.go`
- `internal/fabricengine/coalesce_test.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/commit_lock_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/gitrepo/push.go`
- `internal/gitrepo/push_test.go`
- `manifest/designs/fabric-unified-view.md`
