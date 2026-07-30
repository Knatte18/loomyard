# Plan: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
task: 'fabric: Fabric.Commit classify+dispatch + unified diff/status'
slug: fabric-commit-api
approved: false
started: '2026-07-29T20:51:11Z'
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: commit-foundations
    file: 01-commit-foundations.md
    depends-on: []
    verify: go test -tags integration ./internal/fabricengine/
  - number: 2
    name: async-push-plumbing
    file: 02-async-push-plumbing.md
    depends-on: []
    verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
  - number: 3
    name: fabric-commit
    file: 03-fabric-commit.md
    depends-on: [1, 2]
    verify: go test -tags integration ./internal/fabricengine/
  - number: 4
    name: unified-diff-status
    file: 04-unified-diff-status.md
    depends-on: []
    verify: go test -tags integration ./internal/gitrepo/ ./internal/fabricengine/
  - number: 5
    name: docs
    file: 05-docs.md
    depends-on: [1, 2, 3, 4]
    verify: go build ./internal/fabricengine/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits. Batch-local decisions live in each batch file._

### Decision: relpath-is-dot-for-slice-2

- **Decision:** `Fabric.Commit`'s classifier and the weft-side dispatch treat the caller's paths as worktree-root-relative with `RelPath == "."` — the weft junctions live at the worktree root (`_lyx`, `_pattern`). The pure classifier `classifyPaths` still takes an explicit `relPath` parameter and is unit-tested with both `"."` and `"sub"`, but `Fabric.Commit` passes `"."`.
- **Rationale:** Multi-subpath support is explicitly slice 4+ (`manifest/designs/fabric-unified-view.md` "Out"), so a hub sits at the worktree root in slice 2; `New(warpPath, weftPath)` is constructed at the two worktree roots (see `internal/fabriccli/weft_verbs.go`'s `fabricengine.New(l.WorktreeRoot, l.WeftWorktree())`). Keeping `relPath` a parameter of the pure function future-proofs it without pulling layout/cwd I/O into slice 2.
- **Applies to:** commit-foundations, fabric-commit

### Decision: warp-first-then-weft-under-one-lock

- **Decision:** A two-sided `Fabric.Commit` acquires the weft write lock **before** the warp commit and holds it across both, but **only** when the classifier routes at least one path to weft **and** `!opts.SkipGit`. Warp commits first (bare message, plain `git add`/`commit`, no trailer, no correspondence), then weft commits under the already-held lock via a new `commitWeftLocked` inner helper (Warp-SHA + Snapshot trailers, correspondence). A warp-only commit takes no lock.
- **Rationale:** The weft commit's `Warp-SHA` trailer must name the warp commit that includes this call's warp files, so warp commits first; the held lock closes the warp-commit→weft-trailer-read window against other lyx actors. Gating on `!opts.SkipGit` preserves `CommitWeft`'s existing offline early-out. See the `warp-first-ordering` decision in `_mill/discussion.md`.
- **Applies to:** commit-foundations, fabric-commit

### Decision: partial-failure-report-three-outcomes

- **Decision:** No cross-repo transaction, no rollback. Warp-commit failure returns the warp error with nothing landed (before the push step). A landed warp + failed weft returns a populated `CommitResult` plus a typed `*PartialCommitError`. The error models three weft outcomes — didn't-commit / committed-and-recorded / committed-but-unrecorded — the last (weft commit lands but `RecordCorrespondence` fails, `CommitWeft` returns `(sha, true, err)`) surfaced with `WeftCommitted=true`; recovery is an explicit `RebuildIndex` that rescans the landed weft commit's `Warp-SHA` trailer (the index's source of truth), since `WeftSHAForWarpSHA`'s own one-shot rebuild fires only on a stale *hit*, not on the index *miss* a never-written entry produces.
- **Rationale:** A landed warp commit is ordinary host git and must not be unwound. See `partial-failure-report-not-rollback` in `_mill/discussion.md`.
- **Applies to:** fabric-commit

### Decision: async-push-both-sides-via-detached-child

- **Decision:** `Fabric.Commit` commits synchronously, then fires a detached, fire-and-forget push of **both** repos through an engine-level spawn helper (`fabricengine.SpawnDetachedPush`) that mirrors `boardengine.spawnSync`. The child re-enters `lyx fabric` in bypass mode via companion hidden `--warp-path`/`--weft-path` flags and pushes each supplied path with `PushWarpAt`/`PushWeftAt`. Skip-env gating (`WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`) is **helper-internal** (no child forked when set), matching `fabriccli.spawnPush`. The push fires for whatever landed even on a partial failure; only a warp-commit failure returns before it.
- **Rationale:** Board's model — fast local commit, deferred network push. `PushCoalesced` no-ops when a side has nothing ahead of its upstream, and (when no upstream is configured) performs a harmless first push that establishes tracking rather than no-opping — `gitrepo.hasUnpushed` returns true in the no-upstream case, so the push proceeds — which is why "push whatever landed" is self-scoping. See `async-push-both-sides-detached` in `_mill/discussion.md`.
- **Applies to:** async-push-plumbing, fabric-commit

### Decision: push-invocation-seam-for-tests

- **Decision:** `Fabric.Commit` invokes the async push through a package-level `var spawnDetachedPushFn = SpawnDetachedPush` seam. Every `Fabric.Commit` integration test swaps this seam for a recorder/no-op so no real detached child is spawned from the test binary and the "push fires for whatever landed" ordering is asserted deterministically. Tests that swap the seam must not run with `t.Parallel()`.
- **Rationale:** A detached child launched from a test binary would re-exec the test binary with `fabric … push` args (garbage) and is racy to observe; the var seam is the standard Go test seam and makes push-fired-or-not observable per outcome. Env-gating of the real `SpawnDetachedPush` is unit-tested separately.
- **Applies to:** fabric-commit

### Decision: go-git-worktree-status-for-Fabric.Status

- **Decision:** `Fabric.Status`'s per-repo uncommitted working-tree changed-file list is backed by a new read-only `gitrepo.Repo` method built on go-git's `Worktree.Status()` — a pure on-disk read, so it stays in go-git's half of the **gitrepo Client Boundary Invariant** and needs **no** invariant change and **no** pinned-CLI-set edit. No new `gitexec` call is introduced in `internal/gitrepo`.
- **Rationale:** `ChangedFilesSince` is commit-vs-commit and `StatusWeft` yields only a `dirty` bool; a gitexec `git status --porcelain` backing would require pinning the call and editing `CONSTRAINTS.md`. See `unified-diff-status-warp-anchor` in `_mill/discussion.md`.
- **Applies to:** unified-diff-status

### Decision: go-test-tiers-and-tags

- **Decision:** The pure `classifyPaths` classifier and the pure trailer/tag-validation helpers are untagged **Tier-1** tests (no git spawn), per the Test Tier Purity Invariant. Everything that spawns git — two-sided commit, partial-failure, diff/status, the CLI bypass push — is `//go:build integration` and lands in the same package alongside the existing `*_integration_test.go` files (which already have a `HermeticGitEnv` `TestMain` via `testmain_test.go`). Integration fixtures reuse the existing helpers (`newPlainWarpRepo`, `commitWarp`, `newFabric`, `currentSHA`, `commitMessageAt`, `lyxtest.CopyWeft`, `writeWeftConfigContent`).
- **Rationale:** Enforced by `cmd/lyx/tierpurity_test.go` and `cmd/lyx/hermeticenv_test.go`. See the Constraints section of `_mill/discussion.md`.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `internal/fabriccli/pushbypass_integration_test.go`
- `internal/fabriccli/spawn.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/classify.go`
- `internal/fabricengine/classify_test.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/commit_gating_integration_test.go`
- `internal/fabricengine/commit_integration_test.go`
- `internal/fabricengine/commit_partial_integration_test.go`
- `internal/fabricengine/diff.go`
- `internal/fabricengine/diff_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/spawn.go`
- `internal/fabricengine/spawn_test.go`
- `internal/fabricengine/trailer.go`
- `internal/fabricengine/trailer_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/gitrepo/gogit.go`
- `internal/gitrepo/worktree.go`
- `internal/gitrepo/worktree_test.go`
- `manifest/designs/fabric-unified-view.md`
```
