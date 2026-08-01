# Plan: fabric: warp-rebase / remote-reconcile recovery

```yaml
task: 'fabric: warp-rebase / remote-reconcile recovery'
slug: fabric-rebase-reconcile
approved: true
started: 20260801-163350
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: gitrepo-primitives
    file: 01-gitrepo-primitives.md
    depends-on: []
    verify: go test ./internal/gitrepo/ && go test -tags integration ./internal/gitrepo/ -run 'TestFetch|TestIsAncestor|TestPush' && go test -run TestGitrepoBoundary_PinnedRunCallSites ./cmd/lyx/
  - number: 2
    name: fabric-pull
    file: 02-fabric-pull.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/ -run TestReachableAnchor && go test -tags integration ./internal/fabricengine/ -run TestPull
  - number: 3
    name: cli-pull
    file: 03-cli-pull.md
    depends-on: [2]
    verify: go test -tags integration ./internal/fabriccli/
  - number: 4
    name: docs-sandbox
    file: 04-docs-sandbox.md
    depends-on: [2, 3]
    verify: null
```

## Shared Decisions

### Decision: language-native verify, no PYTHONPATH prefix

- **Decision:** This is a Go project. All `verify:` commands use `go test` directly, with no `PYTHONPATH= ` prefix (that prefix is Python/mill-specific). Plain-string verify implies `cwd: git_root`, which is correct here — every command is git-root-relative and this is not a nested layout (`_paths.resolve_hub_path() == _paths.resolve_git_root()`).
- **Rationale:** matches every existing Go batch convention; the `verify-not-isolated` validator check is language-conditional and does not require the prefix for Go.
- **Applies to:** all batches.

### Decision: gitrepo Client Boundary Invariant is same-commit

- **Decision:** The three new/renamed `gitexec`-bound methods on `internal/gitrepo.Repo` (`Fetch`, `IsAncestor`, and the promotion of `hasUnpushed` → exported `HasUnpushed`) MUST land together with (a) the pinned set update in `cmd/lyx/gitrepoboundary_test.go`'s `gitrepoPinnedRunBoundMethods`, and (b) the CLI-bound method list in `CONSTRAINTS.md`'s **gitrepo Client Boundary Invariant** — all in batch 1's commits. Widening the CLI-bound set without both updates is itself a violation (CONSTRAINTS.md).
- **Rationale:** the boundary guard is set-equality on `r.run`-containing method names; a new CLI-bound method not reflected in the pinned set fails the guard, and the invariant text is what a reviewer checks a new call against.
- **Applies to:** gitrepo-primitives.

### Decision: reachability, never object-existence

- **Decision:** Rebase detection and the nearest-older anchor walk both use ancestry (`git merge-base --is-ancestor`, surfaced as `gitrepo.Repo.IsAncestor`), NEVER `gitrepo.Repo.SHAExists`. `SHAExists` is object-existence only and `git fetch` never prunes objects, so a rebased-away commit's object survives fetch and `SHAExists` would report `true` post-fetch — detection would never fire.
- **Rationale:** discussion.md `rebase-detection-scope` and `warp-refresh-primitives`; the whole slice hinges on this distinction.
- **Applies to:** gitrepo-primitives, fabric-pull.

### Decision: weft-first ordering; report-not-rollback partial failure

- **Decision:** `Fabric.Pull` pulls **weft first** (`PullWeft` fast-forward), then does all warp fetch/inspect/reconcile work. If the weft ff-pull fails, warp is never touched and the call returns immediately (weft-failed, warp-untouched). If weft succeeds but warp-side work fails, the call returns a typed `*PartialPullError` (weft succeeded / warp failed) and never unwinds the completed weft pull. This mirrors `Fabric.Commit`'s shipped `*PartialCommitError` report-not-rollback precedent (`commit.go`), with the two sides' roles swapped to match Pull's weft-first ordering.
- **Rationale:** discussion.md `unified-pull-dispatch` (ordering) and `pull-partial-failure-contract`. Weft-first avoids the self-induced-divergence hazard where a warp-first anchor commit would make weft's own ff-pull fail.
- **Applies to:** fabric-pull, cli-pull.

### Decision: reconcile reuses existing weft-commit machinery

- **Decision:** The reconcile anchor commit is written via the existing empty-commit-with-`Warp-SHA`-trailer mechanism: acquire the fabric weft write lock (`ensureWeftLockDir` + `lock.AcquireWriteLock` on `weftWriteLockFile`), compose the message with `appendWarpSHATrailer(msg, newWarpHEAD)`, then call the existing `commitEmptySnapshot(message, newWarpHEAD)` (weftgit.go) which lands the empty weft commit and calls `RecordCorrespondence`. No new commit primitive is added. Warp advance (both clean-ff and reconcile-reset) reuses the existing `gitrepo.Repo.ResetHard(sha)`.
- **Rationale:** discussion.md `safe-vs-unsafe-reconcile` and `warp-refresh-primitives` — `ResetHard` and the `snapshotTags` empty-commit mechanism already do exactly this; no new primitive is warranted.
- **Applies to:** fabric-pull.

### Decision: no LLM, no raddle, no new CLI verb name

- **Decision:** This slice produces the `PullResult` document (Go struct fields, surfaced via the CLI JSON envelope) and never spawns an LLM or calls raddle regeneration. The existing `lyx fabric pull` verb is extended in place — no new verb name, and deliberately NOT the name `reconcile` (which already repairs host↔weft topology, `internal/fabriccli/fabric.go`).
- **Rationale:** discussion.md `out-of-scope-llm-and-raddle`, `pattern-conflict-reporting`, Scope "Out"; `reed` sits above fabric so an LLM spawn would be a circular dependency.
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/gitrepoboundary_test.go`
- `docs/overview.md`
- `internal/boardengine/sync.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/fabricengine/anchor.go`
- `internal/fabricengine/anchor_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/pull.go`
- `internal/fabricengine/pull_integration_test.go`
- `internal/gitrepo/ancestry.go`
- `internal/gitrepo/ancestry_integration_test.go`
- `internal/gitrepo/ancestry_test.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/fetch_integration_test.go`
- `internal/gitrepo/gogit.go`
- `internal/gitrepo/gogit_test.go`
- `internal/gitrepo/pull.go`
- `internal/gitrepo/push.go`
- `internal/gitrepo/push_test.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
