# Plan: Fix Bouncer anchor-path and run-dir clearing

```yaml
task: "Fix Bouncer anchor-path and run-dir clearing"
slug: "loom-bouncer-anchor-rundir-fix"
approved: false
started: "20260826-063631"
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
    name: anchor-root
    file: 01-anchor-root.md
    depends-on: []
    verify: go test ./internal/hubgeom/... ./internal/shedrecipe/... ./internal/burlercli/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/burlerengine/...
  - number: 2
    name: rundir-clear
    file: 02-rundir-clear.md
    depends-on: [1]
    verify: go test ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/loomrecipe/...
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [2]
    verify: null
```

## Shared Decisions

### Decision: two defects, one task, three batches

- **Decision:** batch 1 fixes the wrong-root defect, batch 2 fixes the stale-run-directory defect, batch 3 lands the two documents neither code change falsifies (`manifest/designs/loom.md`, `manifest/roadmap.md`).
  Every doc comment or yaml comment a code change *falsifies* lands in the same batch as that change, not in batch 3.
- **Rationale:** `_mill/discussion.md` folds the two defects into one task because they touch the same rows and functions, but the file sets barely overlap: batch 1 is `shedrecipe`/`hubgeom`/`burlercli`/`loomcli`, batch 2 is `shedadapters` alone.
  The one shared file is `contracts/recipes/loom-recipe.yaml` (batch 1 edits the two defect-1 comment sites, batch 2 edits the `Plan-Revalidate` comment), which is why batch 2 depends on batch 1 rather than running beside it.
- **Applies to:** all batches

### Decision: comment dispositions come from the two Scope inventories, verbatim

- **Decision:** every comment edit in this plan is one row of one of `_mill/discussion.md`'s two stale-assertion inventories, and carries that row's stated disposition (delete / reword / rewrite / leave).
  No card invents a comment edit the inventories do not name, and no card skips one they do.
- **Rationale:** the inventories were produced by a stated, reproducible enumeration method, and the discussion's Scope section states that a file appearing only in an inventory is In on that basis.
  Treating them as the authority is what makes the edit set auditable rather than asserted.
- **Applies to:** all batches

### Decision: `internal/burlerengine` and `internal/shedengine` semantics are untouched

- **Decision:** `burlerengine.Geometry.WorktreeRoot` keeps its name and its resolution logic; `standalonegeom.BurlerGeometry` keeps its divergence; `shedengine`'s episode and budget model is not changed.
  Only `burlerengine.Geometry.WorktreeRoot`'s *doc comment* changes, and only to record what hub mode now tells it.
- **Rationale:** the `burlerengine-geometry-field-keeps-its-name` and `second-generation-runs-on-the-burlers-leftover-budget` Decisions in `_mill/discussion.md` both settle this.
  Standalone genuinely fills `WorktreeRoot` with the reviewed target directory, distinct from `AnchorPath`; collapsing them would push a hidden `.lyx` tree into the reviewed folder.
- **Applies to:** all batches

### Decision: divergent-roots fixtures are mandatory in every root assertion

- **Decision:** any test asserting which root a value resolves against must use a fixture whose `AnchorPath` and `WorktreeRoot` are two *different* absolute directories (in `hubgeom`, an `AnchorRel` that is deliberately not `"."`).
- **Rationale:** the two roots coincide at today's default, so an assertion made with coincident roots proves nothing and would pass equally before and after the fix.
  `internal/hubgeom/hubgeom_test.go`'s own file doc calls this the refactor's one silent failure mode, and `internal/hubgeom/webstergeom_test.go` already carries the shape to follow.
- **Applies to:** anchor-root

### Decision: TDD per card, never a red commit

- **Decision:** each card that changes behavior writes its test first and watches it fail, then implements until it passes, and commits test plus implementation together in that card's single commit.
- **Rationale:** `_mill/discussion.md`'s Testing section names TDD candidates, and every card produces its own commit — splitting a test card from its implementation card would push a knowingly-failing commit onto the branch.
- **Applies to:** all batches

### Decision: markdown files use semantic line breaks

- **Decision:** every `.md` file this plan touches is written one sentence per line, with additional breaks at internal independent-clause boundaries, and never hard-wrapped at a fixed column.
- **Rationale:** the repo's `CLAUDE.md` mandates it for every `.md` file in the repo, not only newly-written ones.
- **Applies to:** docs

## All Files Touched

- `contracts/recipes/loom-recipe.yaml`
- `internal/burlercli/cli.go`
- `internal/burlercli/wiring.go`
- `internal/burlerengine/geometry.go`
- `internal/hubgeom/hubgeom.go`
- `internal/hubgeom/hubgeom_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomrecipe/revalidate_test.go`
- `internal/shedadapters/archive.go`
- `internal/shedadapters/archive_test.go`
- `internal/shedadapters/bouncer.go`
- `internal/shedadapters/bouncer_clear_test.go`
- `internal/shedadapters/bouncer_commit_test.go`
- `internal/shedadapters/bouncer_replay_test.go`
- `internal/shedadapters/burler.go`
- `internal/shedadapters/doc.go`
- `internal/shedrecipe/entries_bouncer.go`
- `internal/shedrecipe/entries_bouncer_test.go`
- `internal/shedrecipe/recipe.go`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
