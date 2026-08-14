# Plan: Move <hub>/.lyx into <hub>/_board

```yaml
task: "Move <hub>/.lyx into <hub>/_board"
slug: "hub-dotlyx-into-board"
approved: false
started: "20260814-165510"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: hub-scratch-move
    file: 01-hub-scratch-move.md
    depends-on: []
    verify: go test ./internal/fabricengine/... ./internal/reedengine/... ./internal/reedcli/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/... && go test -tags smoke ./internal/reedcli/...
  - number: 2
    name: board-junction-deletion
    file: 02-board-junction-deletion.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
```

## Shared Decisions

### Decision: constraints-are-the-spec

- **Decision:** `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant and Hub Containment Invariant were amended in the discussion commit (`7ef1f105`), ahead of the code, at the operator's explicit instruction.
  Treat both amended sections as the specification to implement, never as a discrepancy to report.
- **Rationale:** reviewers read `CONSTRAINTS.md`; a reviewer reading the superseded Durable-vs-Ephemeral bullet would treat this whole design as an invariant violation.
  The operator judged that risk larger than the cost of the temporary code/doc skew.
- **Applies to:** all batches

### Decision: told-never-derives

- **Decision:** `fabricengine.HubScratchDir(hub string) string` is the sole constructor of `<hub>/_board/.lyx`.
  No other package joins `_board` and `.lyx` itself; `reedengine.HubLogsDir` calls `HubScratchDir` and joins only its own `"logs"` leaf.
- **Rationale:** `TestEnforcement_GeometryLiterals` (`internal/lyxcwd/enforcement_test.go`) restricts the `"_board"` token to `internal/lyxcwd` and `internal/fabricengine` in path-construction context, so `reedengine` must obtain the segment from `fabricengine` regardless;
  a named constructor follows the pattern `StencilsDir` already established and gives future tenants one opening to hang off.
- **Applies to:** all batches

### Decision: reedengine-imports-fabricengine

- **Decision:** the new production import edge `reedengine → fabricengine` is accepted.
  `internal/fabricengine/clone_test.go` (which is `package fabricengine`, in-package) must import `reedengine` nowhere after batch 1, and that absence is what keeps the edge legal.
- **Rationale:** an in-package test file's imports compile into the `fabricengine` test binary and count as `fabricengine`'s own, so `fabricengine`[test] → `reedengine` → `fabricengine` is a cycle Go rejects at test-binary compile time.
  An external test package (`package fabricengine_test`) is a leaf nothing imports, so it may import both sides.
  The production binary is unaffected — `fabricengine` does not import `reedengine`, and nothing in its dependency set does.
- **Applies to:** hub-scratch-move

### Decision: no-migration

- **Decision:** no migration code of any kind, in either direction.
  Nothing moves an existing `<hub>/.lyx`, nothing deletes one, nothing sweeps an existing `_board` junction, and nothing unseeds a leftover `_board` line from a warp repo's `.git/info/exclude`.
- **Rationale:** verified on disk — only `/home/knatte/Code/lyx-test-HUB` carries `<hub>/.lyx` (four disposable tmux logs reed recreates), neither sandbox hub has a `_board` junction, and neither warp exclude carries a `_board` line.
  Cleanup code for a state that exists nowhere is dead code from birth.
- **Applies to:** all batches

### Decision: renames-through-git-mv

- **Decision:** the single file rename in this plan (`boardjunction_integration_test.go` → `hubreservedroutes_integration_test.go`) is performed with `git mv` first, then surgical edits.
  Never write the new file from scratch and delete the old one.
- **Rationale:** preserves git rename history and keeps the review diff to the lines that changed.
- **Applies to:** board-junction-deletion

### Decision: error-posture-of-the-new-seed-call

- **Decision:** `CloneHub`'s new `seedWeftArtifactExcludes(boardDir)` call is checked and fatal, routed through `teardownHub` like every other step-7 failure — never `_ = seedWeftArtifactExcludes(boardDir)`.
- **Rationale:** this is deliberately the opposite of `reconcile.go:311`'s best-effort posture and does not inherit its justification.
  That site swallows failures because the board worktree's artifact excludes are self-healing, which is false for the board: its commit path is `Bolt.Commit` → `commitWeftAt` → `gitrepo.StageAllAndCommit` (`weftgit.go:337-342`), which seeds nothing, unlike `Fabric.Commit`'s `commitBothSides` → `ensureWeftLockDir` path.
  A swallowed failure would silently reinstate exactly the exposure the call exists to close.
- **Applies to:** hub-scratch-move

### Decision: docs-land-with-their-code

- **Decision:** every prose surface is edited in the batch that changes the code it describes.
  There is no trailing docs-only batch.
- **Rationale:** `CLAUDE.md`'s Documentation Lifecycle requires the module doc, `docs/overview.md`, and `CONSTRAINTS.md` to move in the same commit as the change they describe.
- **Applies to:** all batches

### Decision: re-run-the-greps

- **Decision:** before finishing each batch, re-run the three repo-wide greps `_mill/discussion.md` records (hub-level `.lyx` phrasing; `_board` link/junction phrasing; a bare `\.lyx` scan over `internal/fabricengine`), excluding `_mill/`.
  The card inventories below are the enumeration as of planning time, not a guarantee.
- **Rationale:** the discussion's own inventory was assembled by spot checks twice and was incomplete twice;
  the greps are what made it exhaustive, and a site added between planning and implementation would otherwise be missed.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/uncontainedwrite_test.go`
- `docs/overview.md`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/fabric.go`
- `internal/fabriccli/unwire.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/add_test.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_test.go`
- `internal/fabricengine/destructivegaps_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/hubcontainment_integration_test.go`
- `internal/fabricengine/hubreservedroutes_integration_test.go`
- `internal/fabricengine/hubscratch_integration_test.go`
- `internal/fabricengine/hubscratch_test.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/junctionnames_test.go`
- `internal/fabricengine/livestate_manifest_test.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/slug.go`
- `internal/fabricengine/structuraldirs_test.go`
- `internal/fabricengine/unwire.go`
- `internal/reedcli/smoke_debuglog_test.go`
- `internal/reedcli/up.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/serverlog.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/fabric-windows-verification.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
