# Plan: board: move storage to weft:main

```yaml
task: 'board: move storage to weft:main'
slug: board
approved: false
started: 2026-07-29T06:42:36Z
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: 'fabricengine: CommitWeftAt primitive'
    file: 01-commit-weft-at.md
    depends-on: []
    verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 2
    name: 'fabricengine+fabriccli: _board as second weft worktree'
    file: 02-board-weft-topology.md
    depends-on: []
    verify: go test ./internal/fabricengine/... ./internal/fabriccli/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
  - number: 3
    name: 'boardengine: dual-store facade (notes.json, promote-note, single README, weft git-routing)'
    file: 03-board-dual-store-facade.md
    depends-on: [1]
    verify: go test ./internal/boardengine/... && go test -tags integration ./internal/boardengine/...
  - number: 4
    name: 'boardcli: notes CLI surface + promote-note command'
    file: 04-board-notes-cli.md
    depends-on: [3]
    verify: go test ./internal/boardcli/... && go test -tags integration ./internal/boardcli/...
  - number: 5
    name: 'cmd/lyx: board git-import guard + drift/help-tree/registration coverage'
    file: 05-cmd-lyx-board-guards.md
    depends-on: [2, 4]
    verify: go test ./cmd/lyx/... ./internal/fabriccli/... && go test -tags integration ./cmd/lyx/... ./internal/fabriccli/...
  - number: 6
    name: 'docs: CONSTRAINTS, overview, README, manifest, sandbox suites'
    file: 06-board-weft-docs.md
    depends-on: [1, 2, 3, 4, 5]
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: source of truth is `_mill/discussion.md`

- **Decision:** Every card's `Requirements:` below is a concretization of a decision already made and approved in `_mill/discussion.md` (5 review rounds, APPROVE). Where this plan introduces a concrete Go identifier (function/type/const name) that the discussion left as an implementation call, that name is now the pinned identifier — do not invent an alternative name at implementation time.
- **Rationale:** `_mill/discussion.md` is source-grounded against the actual codebase (each of its decisions cites real file:line evidence, corrected across 5 review rounds); this plan's job is to turn its prose decisions into buildable, testable card boundaries, not to re-litigate them.
- **Applies to:** all batches

### Decision: no import cycle from boardengine into fabricengine

- **Decision:** `internal/boardengine` imports `internal/fabricengine` (for `CommitWeftAt`/`PushWeftAt`); the reverse import never exists. This is a one-way dependency, not a new leaf/seam invariant — `fabricengine` already sits below `boardengine` in the existing import graph (boardengine is a CLI-facing feature package; fabricengine is a shared git-coordination module also used by builder/webster/perch).
- **Rationale:** Confirms the plan never asks an implementer to add a `boardengine`-side hook that `fabricengine` would need to call back into.
- **Applies to:** batch 3

### Decision: Go project verify commands never use the `PYTHONPATH=` prefix

- **Decision:** Every `verify:` field in this plan (batch frontmatter and the overview's per-batch entries) is a bare `go test ./...`-shaped command with no `PYTHONPATH=` prefix.
- **Rationale:** The `PYTHONPATH=` prefix rule in mill-plan's own skill instructions is scoped to Python/mill projects (it resets the test subprocess's `PYTHONPATH` so it does not inherit the mill cache scripts dir); this repo is a Go module, so the rule does not apply — `pipeline.done_gate: "go test ./..."` in `mill-config.yaml` already establishes the bare-command convention for this task.
- **Applies to:** all batches

### Decision: no config-key migration/back-compat shim

- **Decision:** `internal/boardengine/config.go`'s `home`/`sidebar`/`proposal_prefix` YAML keys are renamed/removed outright (to `readme`/`design_prefix`, with `sidebar` deleted) — no dual-key acceptance, no deprecation warning, no `lyx config reconcile` migration tooling added.
- **Rationale:** Per `_mill/discussion.md`'s Scope/Out: "there is no live end-user hub with old-style board data" — confirmed during planning: this worktree itself has no `_lyx/config/board.yaml` on disk (never `lyx init`-ed), so there is nothing to migrate, in this repo or any other, today.
- **Applies to:** batch 3

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path
across every batch, sorted alphabetically (Move **source** paths are
excluded — they disappear, like `Deletes:` tokens). Cards are the
source of truth; this section is the input `_plan_validate.py`'s
`all-files-touched-mismatch` check cross-references against the derived
union of every card's `Edits:`/`Creates:`/Move-target paths, to catch
drift between the hand/agent-maintained list here and that derived
union._

- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/boardguard_test.go`
- `cmd/lyx/exitcode_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main_integration_test.go`
- `docs/overview.md`
- `internal/boardcli/cli.go`
- `internal/boardcli/cli_test.go`
- `internal/boardcli/help_test.go`
- `internal/boardcli/notes_test.go`
- `internal/boardcli/promotenote_test.go`
- `internal/boardengine/board.go`
- `internal/boardengine/board_test.go`
- `internal/boardengine/boardtest/bench_test.go`
- `internal/boardengine/boardtest/concurrency_test.go`
- `internal/boardengine/boardtest/sync_test.go`
- `internal/boardengine/config.go`
- `internal/boardengine/config_test.go`
- `internal/boardengine/layer.go`
- `internal/boardengine/layer_test.go`
- `internal/boardengine/render.go`
- `internal/boardengine/render_test.go`
- `internal/boardengine/store.go`
- `internal/boardengine/store_test.go`
- `internal/boardengine/sync.go`
- `internal/boardengine/task.go`
- `internal/boardengine/task_test.go`
- `internal/boardengine/template.yaml`
- `internal/boardengine/template_test.go`
- `internal/configcli/configcli.go`
- `internal/configcli/configcli_test.go`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabricengine/boardweft.go`
- `internal/fabricengine/cleanup.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_adopt_test.go`
- `internal/fabricengine/clone_test.go`
- `internal/fabricengine/commitweftat_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/weftgit.go`
- `internal/gitrepo/doc.go`
- `internal/ideengine/menu_test.go`
- `manifest/designs/curation-triage.md`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/host-visibility.md`
- `manifest/designs/pattern.md`
- `manifest/designs/raddle.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
