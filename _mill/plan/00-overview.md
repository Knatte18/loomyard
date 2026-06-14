# Plan: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep

```yaml
task: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep
slug: docs-stale-sweep
approved: false
started: "20260614-060158"
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
    name: board-config-fixes
    file: 01-board-config-fixes.md
    depends-on: []
    verify: go build ./... && go vet ./...
  - number: 2
    name: muxpoc-doc
    file: 02-muxpoc-doc.md
    depends-on: []
    verify: go build ./... && go vet ./...
  - number: 3
    name: tree-sweep
    file: 03-tree-sweep.md
    depends-on: [2]
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: docs-and-doc-comments-only

- **Decision:** This task changes Markdown files and Go **doc-comments** only. No
  production logic, no test files, no behaviour change. If any card appears to require a
  code/behaviour change to make a doc accurate, stop and leave a note — do not change
  behaviour; that is a separate task.
- **Rationale:** The task is a stale-docs sweep; the shipped code is correct and the docs
  are what lag. `go build`/`go vet` must stay green precisely because nothing functional
  changes.
- **Applies to:** all batches

### Decision: never-modify-line-endings

- **Decision:** Do not reflow, reformat, or normalise line endings in any `.go` file. Edit
  only the specific doc-comment lines. The repo stores LF and `git config core.autocrlf` is
  `true`, so the working tree is CRLF; this is a checkout artifact, not a defect. Never run
  `gofmt -w` and never let an editor convert CRLF↔LF wholesale — the committed diff must be
  content-only (the changed comment lines), nothing else.
- **Rationale:** Every `.go` file shows `gofmt -l` "dirty" purely because of CRLF; the repo
  content is already gofmt-clean. Touching line endings would produce a massive noise diff
  and fight autocrlf.
- **Applies to:** all batches (batches 1 and 2 touch `.go` files)

### Decision: mirror-worktree-reference-pattern

- **Decision:** The corrected board config doc-comments must mirror the already-fixed
  worktree module: a module **delegates config resolution to `internal/config`** and **never
  names config-file layout** (no `.mhgo/`, no "three-layer", no "layered YAML files") in its
  own doc. The reference files are `internal/worktree/cli.go` (lines 1–11) and
  `internal/worktree/config.go` (lines 1–6); `docs/modules/worktree.md` is the reference for
  the markdown shape.
- **Rationale:** The worktree task already established this wording; board must match it, and
  module docs must not re-document the shared config grammar (that lives in
  `docs/shared-libs/config.md`).
- **Applies to:** board-config-fixes

### Decision: muxpoc-coexist-framing

- **Decision:** `internal/muxpoc` is a shipped proof-of-concept module documented on par with
  board/worktree. The planned clean `internal/mux` (described in `docs/modules/mux.md`,
  roadmap milestone 5) is **still unbuilt** — muxpoc and mux **coexist**. muxpoc proved the
  risky parts of milestones 6 (subprocess/reviewer panes) and 7 (daemon crash-recovery)
  ahead of the clean module. Do NOT mark any mux milestone Done and do NOT rewrite mux.md to
  "implemented"; add cross-references only.
- **Rationale:** User decision (see discussion Q&A). muxpoc is dispatched in
  `cmd/mhgo/main.go` and real, but it is explicitly a POC, not the final mux module.
- **Applies to:** muxpoc-doc, tree-sweep

### Decision: preserve-runtime-state-mhgo-references

- **Decision:** `.mhgo/` references that name the **gitignored runtime-state dir** are
  correct and must be preserved, not "fixed": `internal/board/init.go` (gitignore managed
  block contains `.mhgo/`), `internal/muxpoc/state.go` (`.mhgo/muxpoc-state.json`,
  `.mhgo/muxpoc-state.lock`), and the explicit "now-removed config layer" mentions in
  `docs/shared-libs/state.md` and `docs/shared-libs/config.md`. Only the **config-layer**
  description of `.mhgo/` is stale.
- **Rationale:** `.mhgo/` has two distinct roles; only the config-layer role was removed. The
  runtime-state role is live and documented correctly.
- **Applies to:** all batches

## All Files Touched

- `docs/benchmarks.md`
- `docs/modules/board.md`
- `docs/modules/mux.md`
- `docs/modules/muxpoc.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/config.md`
- `docs/shared-libs/state.md`
- `internal/board/cli.go`
- `internal/board/config.go`
- `internal/config/config.go`
- `internal/muxpoc/cli.go`
