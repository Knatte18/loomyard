# Plan: Build internal/mux: the window to the world (overlay + strands + render)

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
slug: 'internal-mux'
approved: true
started: '20260702-170634'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: hubgeometry-dotlyx
    file: 01-hubgeometry-dotlyx.md
    depends-on: []
    verify: go test ./internal/hubgeometry/...
  - number: 2
    name: logger
    file: 02-logger.md
    depends-on: []
    verify: go test ./internal/logger/...
  - number: 3
    name: render
    file: 03-render.md
    depends-on: []
    verify: go test ./internal/muxengine/render/...
  - number: 4
    name: muxengine-carrier
    file: 04-muxengine-carrier.md
    depends-on: [1, 2, 3]
    verify: go test ./internal/muxengine/...
  - number: 5
    name: muxengine-operations
    file: 05-muxengine-operations.md
    depends-on: [3, 4]
    verify: go test ./internal/muxengine/...
  - number: 6
    name: muxcli
    file: 06-muxcli.md
    depends-on: [5]
    verify: go test ./internal/muxcli/...
  - number: 7
    name: cmd-lyx-integration
    file: 07-cmd-lyx-integration.md
    depends-on: [6]
    verify: go test ./cmd/lyx/... ./internal/configreg/... ./tools/sandbox/...
  - number: 8
    name: docs
    file: 08-docs.md
    depends-on: [7]
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions,
error-handling posture, test frameworks, style/lint constraints. One
subsection per decision. Batch-local decisions live in each batch file._

### Decision: Package layout — muxengine (domain) + muxengine/render (pure vocabulary leaf) + muxcli (cobra)

- **Decision:** three new packages. `internal/muxengine/render` is a pure leaf owning the closed display vocabulary (`Anchor`, `Display`, `Strand`, `Box`) and `Rules(strands, box) -> (layoutString, focusTarget)`. `internal/muxengine` is the domain kernel (overlay, strand bookkeeping, persistence, config, lifecycle ops); it **imports** `render` and maps its full persisted records into `[]render.Strand` when calling `Rules`. `internal/muxcli` is the cobra CLI; it imports `muxengine`.
- **Rationale:** the only import direction is `muxcli -> muxengine -> render`. Render never imports the engine, so `engine.ApplyLayout -> render.Rules` creates no cycle. The engine's persisted record carries fields render does not need (`cmd`, `resumeCmd`, `sessionId`, `worktree`, `name`); those stay out of `render.Strand`, which holds only what layout needs (`guid`, `parent`, `display`, `paneId`, `live`).
- **Applies to:** all batches.

### Decision: Domain-free strand contract — mux stores all fields, reads none semantically; no `type` field

- **Decision:** the persisted record is `{ guid, name, worktree, parent?, cmd, resumeCmd?, sessionId?, paneId, display{ anchor, focus, shrinkWhenWaitingOnChild } }`. `cmd`/`resumeCmd` are opaque strings mux never parses; `sessionId` is opaque metadata mux neither writes nor reads in v1; there is **no** domain `type` field. `--role`/`--round` are formatting-only inputs consumed at add-time to fill the `strand-name` template, never persisted and never branched on.
- **Rationale:** a `type` field would force mux to import its consumers' vocabulary (circular). Keeps mux provider- and domain-invariant.
- **Applies to:** muxengine-carrier, muxengine-operations, muxcli.

### Decision: GUID is the durable key; name is display-only; selectors are guid-only

- **Decision:** `guid` (128-bit `crypto/rand`, hex) is mux-generated at `AddStrand` and is the durable identity — parent links store the parent's guid, reconcile keys on guid, `UpdateStrand`/`RemoveStrand` mutate by guid. `--parent <guid>` and `remove <guid>` take the guid (printed in `add` JSON). `name` is a caller-supplied descriptive label used only for the pane title and `status` output; it is never a selector and has no uniqueness requirement. `sessionId` is demoted to opaque metadata (never identity).
- **Rationale:** guid is 100% unique; keeping selectors guid-only removes an ambiguity-resolution path and its failure mode.
- **Applies to:** muxengine-carrier, muxengine-operations, muxcli.

### Decision: Reuse muxpoc mechanics verbatim, change only the height policy

- **Decision:** the layout checksum (`layoutChecksum`, `internal/muxpoccli/cmd.go:279`), the `window_layout` string format, and the pane-id/parse plumbing are ported **verbatim** from `internal/muxpoccli`. What changes is the **height policy**: muxpoc's fixed `activePaneShare=55%` is replaced by the derived policy (`topBandRows` bands, `collapsedStripRows` strips, active + `shrink:false` panes split the remainder equally, remainder rows to the active/bottom pane, clamp rule). The pinned checksum fixture `acd7` for body `220x50,0,0[220x15,0,0,1,220x15,0,16,4,220x18,0,32,3]` is preserved.
- **Rationale:** the checksum math is the risky, psmux-verified part; reusing it verbatim keeps it stable while the domain-facing policy stays small and total.
- **Applies to:** render, muxengine-operations.

### Decision: On-demand re-render, single mux operation lock at exactly one layer

- **Decision:** v1 is daemonless — the layout recomputes in-process on each mutation and on-demand on every CLI verb (reconcile + apply). There is no live `pane-died` listener. The whole `read -> mutate -> persist -> render -> apply(select-layout)` cycle is guarded by one **mux operation lock** at `<worktree>/.lyx/mux.lock` (via `internal/lock`), acquired **once at the engine-op boundary** and never by CLI verbs (gofrs/flock is non-reentrant across handles even in-process on Windows — a CLI-locks-then-engine-locks path would self-deadlock). Lock ordering is strict outer `mux.lock` -> inner `state`'s `mux.json.lock`. Engine ops compose from unexported, unlocked helpers and never call each other while holding the lock.
- **Rationale:** each CLI verb is its own process; shuttle drives `AddStrand` in-process concurrently. Locking only the JSON write lets two mutations clobber each other's layout, so v1 serializes the full cycle.
- **Applies to:** muxengine-operations, muxcli.

### Decision: Go testing conventions; live-psmux tests behind a build tag

- **Decision:** per-file unit tests next to source. Pure logic (render, parse, checksum, naming, env, state round-trip, reconcile planning) is tested without psmux. Any test needing a real psmux server is guarded by `//go:build smoke` so the default `go test ./...` (and every batch `verify:`) stays fast and hermetic. Drive the CLI through the `RunCLI(&out, args)` seam and assert on the parsed JSON envelope (`ok` true/false).
- **Rationale:** keeps `verify:` deterministic and CI-safe; the risky I/O shells are the only smoke-tagged surface.
- **Applies to:** all batches with a runnable surface.

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path
across every batch, sorted alphabetically._

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/jsonhelp_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/registration_test.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `cmd/lyx/unknown_subcommand_test.go`
- `docs/modules/mux.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_unit_test.go`
- `internal/logger/logger.go`
- `internal/logger/logger_test.go`
- `internal/muxcli/add.go`
- `internal/muxcli/attach.go`
- `internal/muxcli/cli.go`
- `internal/muxcli/cli_test.go`
- `internal/muxcli/remove.go`
- `internal/muxcli/resume.go`
- `internal/muxcli/smoke_test.go`
- `internal/muxcli/status.go`
- `internal/muxcli/up.go`
- `internal/muxengine/apply.go`
- `internal/muxengine/apply_test.go`
- `internal/muxengine/config.go`
- `internal/muxengine/config_test.go`
- `internal/muxengine/doc.go`
- `internal/muxengine/env.go`
- `internal/muxengine/env_test.go`
- `internal/muxengine/lifecycle.go`
- `internal/muxengine/lifecycle_test.go`
- `internal/muxengine/lock.go`
- `internal/muxengine/lock_test.go`
- `internal/muxengine/name.go`
- `internal/muxengine/name_test.go`
- `internal/muxengine/naming.go`
- `internal/muxengine/naming_test.go`
- `internal/muxengine/overlay.go`
- `internal/muxengine/parse.go`
- `internal/muxengine/parse_test.go`
- `internal/muxengine/reconcile.go`
- `internal/muxengine/reconcile_test.go`
- `internal/muxengine/render/checksum.go`
- `internal/muxengine/render/checksum_test.go`
- `internal/muxengine/render/focus.go`
- `internal/muxengine/render/height.go`
- `internal/muxengine/render/height_test.go`
- `internal/muxengine/render/layout.go`
- `internal/muxengine/render/policy.go`
- `internal/muxengine/render/policy_test.go`
- `internal/muxengine/render/rules.go`
- `internal/muxengine/render/rules_test.go`
- `internal/muxengine/render/types.go`
- `internal/muxengine/state.go`
- `internal/muxengine/state_test.go`
- `internal/muxengine/strand.go`
- `internal/muxengine/strand_test.go`
- `internal/muxengine/template.go`
- `internal/muxengine/template.yaml`
- `tools/sandbox/SANDBOX-SUITE.md`
