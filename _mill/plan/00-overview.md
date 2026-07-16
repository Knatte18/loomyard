# Plan: Built-in operator console pane in mux

```yaml
task: "Built-in operator console pane in mux"
slug: mux-operator-console
approved: false
started: "20260716-114948"
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches._

```yaml
batches:
  - number: 1
    name: hubgeometry-repo
    file: 01-hubgeometry-repo.md
    depends-on: []
    verify: go test ./internal/hubgeometry/...
  - number: 2
    name: tokenvocab-module
    file: 02-tokenvocab-module.md
    depends-on: [1]
    verify: go test ./internal/tokenvocab/...
  - number: 3
    name: header-text-pipeline
    file: 03-header-text-pipeline.md
    depends-on: [2]
    verify: go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
  - number: 4
    name: header-pane-and-render
    file: 04-header-pane-and-render.md
    depends-on: [3]
    verify: go test -tags integration ./internal/muxengine/... ./internal/muxcli/...
```

## Shared Decisions

### Decision: header-is-not-a-strand

- **Decision:** The header pane is a first-class but separate construct, never a `Strand`. It is
  persisted as a new `MuxState.HeaderPaneID` field (outside the `Strands` slice) and excluded from
  every strand accounting, adoption, reconcile, and layout path. It is always-on and structural —
  no config toggle disables it.
- **Rationale:** discussion.md `header-is-not-a-strand` (BINDING, user-stated). The keepalive
  guarantee holds only if the header always exists and is never counted/removed/adopted as a strand.
- **Applies to:** header-pane-and-render

### Decision: header-command-two-modes

- **Decision:** `lyx mux header` has two modes. Default renders the header text and returns it via
  the `internal/output` envelope (`output.Ok`; `--json` available) — a normal, smoke-testable CLI
  command needing no carve-out. `--blocking` prints the rendered text then blocks forever; only this
  flag-gated mode is exempt from the JSON envelope. The mux header pane boots `lyx mux header
  --blocking`.
- **Rationale:** discussion.md `three-part text pipeline` (Q13/GAP-1). Keeps the CLI/Cobra invariant
  satisfied for the default path; the `--blocking` exemption is a narrow extension of the existing
  interactive-handoff exception (CONSTRAINTS.md), recorded in the same commit.
- **Applies to:** header-text-pipeline, header-pane-and-render

### Decision: tokenvocab-shared-leaf

- **Decision:** `internal/tokenvocab` is a general/shared module (loom will also consume it). It
  imports only stdlib, `internal/hubgeometry`, and `internal/stencil`. A `leaf_enforcement_test.go`
  (allowlist-only) and a CONSTRAINTS.md "Tokenvocab Leaf Invariant" entry enforce this, mirroring
  `internal/modelspec`.
- **Rationale:** discussion.md `three-part text pipeline` item 2 + r3 NOTE. `stencil` stays a pure
  stdlib leaf; the vocabulary is deliberately not inside stencil.
- **Applies to:** tokenvocab-module, header-text-pipeline

### Decision: shell-seam-and-executable

- **Decision:** The header pane's launch command string (`<lyx-binary> mux header --blocking`) is
  built through `internal/shell` (`shell.ForGOOS().Quote`/`Invoke`) — never raw shell syntax — and
  the binary path comes from `os.Executable()`. The command-string assembly is a pure helper
  (fake exe + shell injected) so it is host-testable; `os.Executable()` is called only at the boot
  site. The header pane's cwd is `layout.Hub` (`split-window -c <layout.Hub>`).
- **Rationale:** Shell Mechanics Seam + Hub Geometry Invariant (CONSTRAINTS.md). `os.Executable`
  precedent: `internal/weftcli/spawn.go`, `internal/boardengine/spawn.go`.
- **Applies to:** header-pane-and-render

### Decision: eager-header-validation

- **Decision:** The header template is validated eagerly at `up`/config-load — a bad template or
  unresolvable token surfaces as `output.Err` with a non-zero exit, loud and early, before the
  session boots. Render errors are never silently swallowed. The `--blocking` pane only
  prints-the-error-and-keeps-blocking as a rare last-resort fallback (keepalive preserved).
- **Rationale:** discussion.md `pure-header` error path (GAP-3). Errors are fixed loudly.
- **Applies to:** header-text-pipeline, header-pane-and-render

### Decision: go-verify-scoping-and-tiers

- **Decision:** This is a Go repo — `verify:` uses `go test` directly (no `PYTHONPATH=` prefix),
  scoped to the packages the batch touches. Real-tmux tests in `contract_integration_test.go` are
  `//go:build integration`, so batch 4's verify passes `-tags integration` (the tests skip via
  `exec.LookPath` when tmux is absent). Build-tagged smoke tests (`//go:build smoke`) and the
  `--blocking` pane are exercised only via the named smoke commands in each `## Batch Tests`, never
  inline in the per-round `verify:`. Untagged unit tests build `hubgeometry.Layout` struct literals
  rather than calling `Resolve` (Test Tier Purity Invariant).
- **Rationale:** CONSTRAINTS.md Test Tier Purity + Hermetic Git Test Environment invariants; keep the
  per-round verify fast.
- **Applies to:** all batches

### Decision: docs-and-pinned-sets-same-commit

- **Decision:** Documentation and machine-pinned sets are updated in the same batch/commit as the
  surface they describe: the new `tokenvocab` module doc + `docs/overview.md` table + CONSTRAINTS.md
  leaf entry land in batch 2; the CLI envelope-exemption extension + `docs/overview.md#modules`
  rationale + `cmd/lyx/helptree_test.go` pinned mux-subcommand set land in batch 3; the mux module
  doc's header/render section lands in batch 4.
- **Rationale:** CLAUDE.md Documentation Lifecycle ("update docs as part of the same commit");
  CONSTRAINTS.md CLI/Cobra ("update the pinned sets in the same commit").
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `docs/modules/mux.md`
- `docs/modules/tokenvocab.md`
- `docs/overview.md`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/hubgeometry/hubgeometry_unit_test.go`
- `internal/muxcli/cli.go`
- `internal/muxcli/header.go`
- `internal/muxcli/header_test.go`
- `internal/muxcli/smoke_lifecycle_test.go`
- `internal/muxengine/apply.go`
- `internal/muxengine/config.go`
- `internal/muxengine/contract_integration_test.go`
- `internal/muxengine/header.go`
- `internal/muxengine/header-template.md`
- `internal/muxengine/header_test.go`
- `internal/muxengine/headerpane.go`
- `internal/muxengine/headerpane_test.go`
- `internal/muxengine/headertemplate.go`
- `internal/muxengine/lifecycle.go`
- `internal/muxengine/reconcile.go`
- `internal/muxengine/reconcile_test.go`
- `internal/muxengine/render/height.go`
- `internal/muxengine/render/height_test.go`
- `internal/muxengine/render/layout.go`
- `internal/muxengine/render/policy.go`
- `internal/muxengine/render/rules.go`
- `internal/muxengine/render/rules_test.go`
- `internal/muxengine/render/types.go`
- `internal/muxengine/spawn.go`
- `internal/muxengine/spawn_test.go`
- `internal/muxengine/state.go`
- `internal/muxengine/template_posix.yaml`
- `internal/muxengine/template_windows.yaml`
- `internal/tokenvocab/doc.go`
- `internal/tokenvocab/leaf_enforcement_test.go`
- `internal/tokenvocab/render.go`
- `internal/tokenvocab/tokenvocab.go`
- `internal/tokenvocab/tokenvocab_test.go`
