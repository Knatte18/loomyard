# Discussion: Built-in operator console pane in mux

```yaml
task: Built-in operator console pane in mux
slug: mux-operator-console
status: discussing
parent: main
```

> **⏳ IN PROGRESS** — this file is being written incrementally during the
> mill-start interview (after the original discussion thread was lost to an
> accidental Ctrl+C). Sections marked `⏳ OPEN` are not yet settled.

## Problem

During the session-fork-diversity-spike (2026-07-16), removing mux strands
down to the last one tore down the whole tmux session — tmux kills a session
when its last pane dies — which killed a running test mid-flight. The ad-hoc
fix was a manually added keepalive strand running a bare `bash` in the hub
root, which turned out to double as a handy operator scratch terminal whose
prompt shows which hub the mux serves.

**Why now:** the teardown is a live footgun (it already broke a test), and the
spike left an operator-endorsed design note (`docs/research/session-fork-spike.md:233-239`)
to make the keepalive a built-in mux feature.

## Scope

**In:**

- A new built-in **header** construct in mux: one small, always-present tmux/psmux
  pane pinned at the top of the mux view, at a **config-bar (fixed) height** (default
  1 row for now).
- The header pane runs a new command `lyx mux header`, which prints a descriptive
  text and then **hangs** (blocks) so the pane never exits.
- The header exists to keep the tmux session structurally alive: it makes the
  "remove last strand kills the session" edge **unreachable** — strand churn can
  never empty the session.
- The header is **NOT a strand**: it is a separate construct, not stored in the
  `Strands` slice, and excluded from all strand accounting (`status`, `resume`,
  `remove`, count).
- The header content is **configurable** via a template file with `<TOKEN_NAME>`
  placeholders.
- A new **token-substitution module**: takes a template file (with `<TOKEN>`
  placeholders) + a token→value map, returns the filled-in text. Modelled on the
  existing `yamlengine` module's approach.

**Out:**

- ⏳ OPEN — interactive scratch-terminal use of the pane. The original brief framed
  this as an operator scratch terminal for manual jobs; the recalled design is a
  **pure 1-line header** that just prints text and hangs. Confirm the scratch-terminal
  aspect is dropped (a 1-row header can't host manual jobs). _To confirm in batch 1._
- ⏳ OPEN — pane naming/tagging as a general mux feature (separate gap noted in the spike).

## Decisions

### header-is-not-a-strand (BINDING — user-stated)

- Decision: The header is a first-class but separate construct, **never** a strand.
  Persist its pane id in `MuxState` (new field, e.g. `HeaderPaneID`) rather than in
  the `Strands` slice. Special-case it through boot / reconcile / apply / status.
- Rationale: User explicitly requires it not be a strand; keeps strand accounting
  (count, `status`, `remove`, `resume`) clean and unaware of it.
- Rejected: (a) modelling it as a hidden/special strand — user rejected; (b) reusing
  the reserved `AnchorOwnWindow` to put it in its own tmux window — that would not be
  a visible top header.

### header-command (`lyx mux header`)

- Decision: New CLI verb `lyx mux header` that renders the configured template
  (token-substituted) to stdout, then blocks forever so the pane stays alive.
- Rationale: A real long-lived process in the pane is what keeps the session alive;
  putting the render logic behind a mux subcommand keeps it self-contained and testable.
- ⏳ OPEN: exact block mechanism (sleep-infinity vs read-blocking), redraw-on-resize
  behaviour, what happens on SIGTERM/detach. _batch 1._

### token-substitution module (new)

- Decision: New module that fills `<TOKEN_NAME>` placeholders in a template file from
  a token→value map and returns the completed text. Modelled on `yamlengine`.
- Rationale: The header content must be configurable via a token vocabulary; this is
  reusable beyond the header.
- ⏳ OPEN: module name/location; whether an existing helper already does this
  (grep found `<TOKEN>`-style patterns in `internal/muxengine/name.go`,
  `internal/builderengine/spawn.go` + `template_test.go`, `internal/shuttleengine/spec.go`);
  token vocabulary (which tokens exist: hub path, worktree, session name, …); unknown-token
  and missing-value policy. _batch 1/2._

## Technical context

Full codebase map: `.scratch/mux-console-exploration.md`. Highlights:

- **No `anchor:top` / fixed-height top band exists today.** `render/types.go:23-39`
  closed anchor vocabulary is `below-parent | own-window (deferred) | hidden`. A pinned
  fixed-height top band is **new render-layer work** (new `policy.go` case + `layout.go`
  mechanics), not reuse.
- **Most dangerous seam:** `reconcile.go:93-113` — `planReconcile` kills any live pane
  not in `boundPaneIDs` (built from `Strand.PaneID`) the moment any strand is bound. The
  header pane id must be threaded into an exemption set here or it gets reaped.
- **Accounting exclusion points:** `lifecycle.go:415` (`UpResult.Strands` count),
  `lifecycle.go:922-925` (`Status` loop), `noSessionMessage`/`strandCount` (`lifecycle.go:842-847`).
- **Last-pane kill site:** `strand.go:485-487` (`RemoveStrand`'s explicit `kill-pane` loop
  — what destroys the session today); `reconcile.go:59-79` `keptDeadPane` (existing partial
  guard, made moot by a permanent header).
- **Pane creation:** `launchStrandLocked` splits tallest via `split-window` with **no `-c`**
  cwd (`spawn.go:110-146`); only boot `new-session -c <layout.Cwd>` passes a cwd
  (`lifecycle.go:294-302`). Header wants hub root → new `split-window -c <e.layout.Hub>`.
- **Hub root:** `hubgeometry.Layout.Hub = filepath.Dir(WorktreeRoot)` (`hubgeometry.go:117`).
- **tmux layer:** `overlay.go` `TmuxCmd` is the only subprocess seam. No pane tagging exists;
  panes identified by `#{pane_id}`.
- **Boot:** header should be created during `ensureServerAndSessionLocked` / `Up` so it
  exists before any strand, and re-created if missing across `up` cycles.

## Constraints

- CONSTRAINTS.md: **Hub Geometry Invariant** — all cwd/geometry via `internal/hubgeometry`
  (use `layout.Hub`, never recompute). **CLI / Cobra Invariant** — `lyx mux header` needs a
  `Short`, must slot into the module `Command()`/`RunCLI` seam and pass help-tree tests.
  **lyxtest Leaf Invariant** for test fixtures. **Documentation Lifecycle** — update
  `docs/modules/` + `docs/overview.md` in the same commit if module surface changes; record
  any new cross-cutting invariant in CONSTRAINTS.md.
- Seam direction `muxcli → muxengine → render` is one-directional; the token module must not
  break it.

## Testing

⏳ OPEN — to be filled as decisions settle. Candidates:

- Token-substitution module: hermetic unit tests (TDD candidate) — placeholder fill, repeated
  tokens, unknown-token / missing-value policy, no-token passthrough.
- Header render: `lyx mux header` output is the token-substituted template (hermetic).
- Keepalive regression: flip `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds`
  (`contract_integration_test.go:332`) — with a header present, removing the last strand
  must leave the session AND header alive, strand count reaching zero.
- Reconcile exemption: mirror `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`
  (`smoke_lifecycle_test.go:237`) — header pane survives reconcile even with strands bound.

## Q&A log

- **Q:** Is the keepalive a bash scratch terminal or a pure header? **A:** A pure header —
  a fixed-height (default 1 row) pane running `lyx mux header`, which prints descriptive text
  and hangs. Not an interactive scratch shell.
- **Q:** Is the header a strand? **A:** No — explicitly not a strand; separate construct,
  excluded from strand accounting.
- **Q:** How is header content configured? **A:** Via a template file with `<TOKEN_NAME>`
  placeholders, filled by a new token-substitution module (modelled on `yamlengine`).
