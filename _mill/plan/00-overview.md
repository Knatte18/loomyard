# Plan: fabric: collapse external API surface onto Commit — stop leaking warp/weft

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
slug: fabric-collapse-external-surface
approved: false
started: '20260802-114601'
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. The DAG is a strict linear chain: every batch edits shared `internal/fabricengine` files (`doc.go`, `weftgit.go`, `commit.go`), and each batch depends on the previous both to avoid parallel-edit conflicts and because each unexport/deletion only becomes compile-safe after the prior batch removes the last external caller. Each batch leaves the whole module green (`go build ./...` + its scoped `go test`)._

```yaml
batches:
  - number: 1
    name: bolt-handle
    file: 01-bolt-handle.md
    depends-on: []
    verify: go test -tags integration ./internal/fabricengine/ ./internal/boardengine/ ./internal/fabriccli/
  - number: 2
    name: commit-migration
    file: 02-commit-migration.md
    depends-on: [1]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/buildercli/ ./internal/webstercli/ ./internal/perchcli/
  - number: 3
    name: remove-force-add
    file: 03-remove-force-add.md
    depends-on: [2]
    verify: go test -tags integration ./internal/gitrepo/ ./internal/fabricengine/
  - number: 4
    name: clean-healthy-renames
    file: 04-clean-healthy-renames.md
    depends-on: [3]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/loomengine/
  - number: 5
    name: delete-dead-methods
    file: 05-delete-dead-methods.md
    depends-on: [4]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/gitrepo/ ./internal/boardengine/
  - number: 6
    name: fabric-cli-collapse
    file: 06-fabric-cli-collapse.md
    depends-on: [5]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/ ./cmd/lyx/
```

## Shared Decisions

### Decision: unexport-not-delete-primitives

- **Decision:** The weft-git primitives that leave the external surface are *unexported in place*, not deleted: `CoalescePush`→`coalescePush`, `CommitWeftAt`→`commitWeftAt`, `PushWeftAt`→`pushWeftAt`, `CommitWeft`→`commitWeft`, `StatusWeft` (dropped after its verb repoints; unexport-or-delete since its only caller is that verb), `SnapshotWarpSHA`→`snapshotWarpSHA`. `CoalescePushBothAt` stays **exported** (still used by `fabriccli/weft_verbs.go:212`). Only three symbols are genuinely deleted: `SyncWeft`, `RevertWithWeft`, and their orphaned support (`warpSHAFromTrailer`, `SyncResult`, `RevertResult`, `ErrRevertRollbackFailed`).
- **Rationale:** Each of these keeps in-package callers after migration (`Bolt`, `CoalescePushBothAt`, `unwire.go`, in-package tests), so unexporting removes the external advertisement while preserving the internal capability. `resolveRevertTarget`/`classifyCorrespondence`/`revertResolution` are explicitly KEPT — they back the live `Fabric.Diff` (`diff.go:33`), verified not orphaned.
- **Applies to:** all batches

### Decision: commit-takes-positive-path-list

- **Decision:** `Fabric.Commit(files, msg, snapshotTags, opts)` takes a plain **positive** path list — the exact `ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName})` shape callers build today, minus every `:(exclude)` entry. `classifyPaths` splits it warp/weft by literal prefix; `git add -- <path>` then auto-skips anything in the repo's `.git/info/exclude`. No `:(exclude)` pathspec magic and no `git add -f` survives anywhere after this task.
- **Rationale:** Transient exclusion belongs to git's own local exclude file (`seedWeftArtifactExcludes`), not per-call pathspec magic. `classifyPaths` does no glob/magic parsing — a directory path like `<RelPath>/_lyx` is a valid positive entry that stages the whole subtree minus excluded files.
- **Applies to:** commit-migration, fabric-cli-collapse

### Decision: golang-comments-trim-touched-files

- **Decision:** Every card trims the doc/inline comments of the files it substantially edits to the millhouse#769 `golang-comments` shape: doc comments = what+why (not internal how); inline = why-only; no mandatory-per-step inline; file-level comments unchanged. Heavy targets called out in the discussion: `hostclean.go`, `drift.go`, `boardengine/sync.go`, and any long how-it-works doc comment in a touched file. No repo-wide sweep — touched files only.
- **Rationale:** The task mandates the new comment shape in every file it touches; folding the trim into the editing card (not a separate sweep) avoids re-touching files and cross-batch overlap.
- **Applies to:** all batches

### Decision: each-batch-stays-green-verify-integration

- **Decision:** Every batch's `verify:` runs the affected packages with `-tags integration` (the real-git tests are `//go:build integration`; the correctness this task guards — exclude-file behaviour, coalescing, RelPath depth — lives in those tagged tests). The module-wide `verify: go build ./...` runs at each batch boundary to catch cross-package production compile breaks from the shared-symbol unexports. mill-go's `done_gate` (`go test ./...`) is the final whole-tree gate.
- **Rationale:** "Verify over plan review" — real test execution, not slice-shape unit tests, is what proves the exclude-file and coalescing behaviour survives each migration.
- **Applies to:** all batches

### Decision: bookkeeping-already-satisfied

- **Decision:** The `fabric-v2-crucible` (#023) `depends_on` update named in the task body is **already done** — `loomyard/wiki/tasks.json` already lists `"fabric-collapse-external-surface"` in that task's `depends_on`. No card performs it. (It is wiki-daemon state in a separate repo, never a plain file edit from this worktree regardless.)
- **Rationale:** Recon confirmed the entry is present; adding a card would be a no-op and cannot be a tracked-file edit.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `docs/overview.md`
- `internal/boardengine/board.go`
- `internal/boardengine/sync.go`
- `internal/boardengine/sync_integration_test.go`
- `internal/buildercli/weft.go`
- `internal/buildercli/weft_integration_test.go`
- `internal/fabricengine/bolt.go`
- `internal/fabricengine/bolt_integration_test.go`
- `internal/fabricengine/checkout_index_refresh_test.go`
- `internal/fabricengine/coalesce.go`
- `internal/fabricengine/coalesce_test.go`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/commit_integration_test.go`
- `internal/fabricengine/commit_partial_integration_test.go`
- `internal/fabricengine/commitweftat_test.go`
- `internal/fabricengine/diff.go`
- `internal/fabricengine/diff_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/hostclean.go`
- `internal/fabricengine/index.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/revert.go`
- `internal/fabricengine/revert_test.go`
- `internal/fabricengine/snapshot.go`
- `internal/fabricengine/snapshot_integration_test.go`
- `internal/fabricengine/status.go`
- `internal/fabricengine/syncweft_integration_test.go`
- `internal/fabricengine/topology.go`
- `internal/fabricengine/unwire.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/weftgit_exclude_test.go`
- `internal/fabricengine/weftgit_pathspec_integration_test.go`
- `internal/fabricengine/weftgit_unborn_warp_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/weft_verbs.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitrepo/noforceadd_test.go`
- `internal/gitrepo/reset.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/preflight_integration_test.go`
- `internal/perchcli/run.go`
- `internal/perchcli/run_integration_test.go`
- `internal/webstercli/weft.go`
- `internal/webstercli/weft_integration_test.go`
- `manifest/designs/fabric-unified-view.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/reconcile_stale_removal_test.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
