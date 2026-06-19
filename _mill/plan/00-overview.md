# Plan: Reconcile stale design docs (stateless + weft model)

```yaml
task: "Reconcile stale design docs (stateless + weft model)"
slug: reconcile-stale-docs
approved: true
started: "20260619-105219"
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: docs-durable-core
    file: 01-docs-durable-core.md
    depends-on: []
    verify: null
  - number: 2
    name: docs-sweep-and-moves
    file: 02-docs-sweep-and-moves.md
    depends-on: [1]
    verify: null
  - number: 3
    name: code-headers
    file: 03-code-headers.md
    depends-on: []
    verify: go build ./... && go vet ./... && go test ./...
```

## Shared Decisions

### Decision: durable-docs-are-overview-and-roadmap

- **Decision:** The only durable design docs are `docs/overview.md` (principles, naming, the module/lib map, the weft contract, and the new Documentation-lifecycle convention) and the not-yet-landed portion of `docs/roadmap.md`. Mechanical per-module docs are deleted once their module lands; the implementation + tests are the source of truth.
- **Rationale:** Per-module prose duplicates code+tests and rots. See discussion.md `doc-lifecycle-convention`.
- **Applies to:** all batches

### Decision: portals-are-deprecated-but-present

- **Decision:** Portal code still exists (`internal/worktree/portals.go`; `paths.Layout` methods `PortalsDir`/`PortalLink`/`PortalTarget`). Docs describe portals as **deprecated-but-present**, removed in task 006 (weft engine). Do NOT describe portals as gone, and do NOT remove portal code.
- **Rationale:** Weft has zero implementation; portals are still the live mechanism. See discussion.md `weft-framing`.
- **Applies to:** all batches

### Decision: weft-is-decided-but-not-built

- **Decision:** The weft overlay model is the decided **target** architecture. Docs state it as the design but must make unambiguous (status markers + roadmap) that weft is **not built yet**.
- **Rationale:** discussion.md `weft-framing`.
- **Applies to:** docs-durable-core, docs-sweep-and-moves

### Decision: no-code-logic-changes

- **Decision:** `.go` edits touch **doc-comments only** — no functions, types, signatures, or behavior. No tests added or modified.
- **Rationale:** discussion.md `code-headers-carry-the-why` + Constraints. The full Go suite (`go build && go vet && go test`) must pass unchanged.
- **Applies to:** code-headers

### Decision: stateless-worktree-no-registry

- **Decision:** The worktree module is stateless: there is no worktree registry and no `.lyx/local-state.json` in code; `lyx worktree list` is a thin `git worktree list --porcelain` wrapper. `internal/state` landed (`ba81abf`) as a generic helper with no consumer — it is **not** "(planned)". Every kept-doc mention of a *worktree* registry / `local-state.json` / "lands with mux" is stale and must go. Exception: `docs/modules/mux.md` may keep references to mux's *own* future state document (see discussion.md `mux-registry-semantics`).
- **Rationale:** discussion.md Technical context + `mux-registry-semantics`.
- **Applies to:** all batches

## All Files Touched

_Union of every `Edits:` + `Creates:`, sorted. `Deletes:` paths are intentionally
excluded: the 9 deleted mechanical docs (`docs/modules/{board,worktree,ide,muxpoc}.md`,
`docs/shared-libs/{git,lock,fsx,gitignore,state}.md`) and the 4 move sources
(`docs/modules/mux-{exploration,hooks-exploration,proposal}.md`,
`docs/vendor/psmux_scripting.md`)._

- `CONSTRAINTS.md`
- `docs/benchmarks/board-performance.md`
- `docs/modules/mux.md`
- `docs/overview.md`
- `docs/reference/psmux_scripting.md`
- `docs/research/mux-exploration.md`
- `docs/research/mux-hooks-exploration.md`
- `docs/research/mux-proposal.md`
- `docs/roadmap.md`
- `docs/shared-libs/README.md`
- `docs/shared-libs/config.md`
- `docs/shared-libs/paths.md`
- `internal/board/board.go`
- `internal/ide/cli.go`
- `internal/muxpoc/cli.go`
- `internal/worktree/cli.go`
