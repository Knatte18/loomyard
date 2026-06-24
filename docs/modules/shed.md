# Module: shed (design)

> **Status: Design — not built.** Per the [documentation lifecycle](../overview.md#documentation-lifecycle),
> this file is deleted when the module lands and its durable parts fold into the package
> header and `overview.md`.

The name is from weaving: the **shed** is the opening between the warp threads through which
the shuttle passes — the active space where weaving actually happens. (Sibling to
[`weft`](../overview.md#weft-overlay-model), the overlay repo.)

shed is the semantic layer directly above [`mux`](mux.md). Where mux speaks **panes and
windows**, shed speaks **worktrees and agent roles**. It owns the mapping from
`{worktree, role}` to a live pane, the layout policy that arranges those panes, the focus
decision when a human is needed, and the cluster window. [`loom`](loom.md) and
[`agent`](agent.md) address shed in domain terms — raw psmux pane IDs never leak above it.

## Why a separate module from mux

mux is a thin, testable psmux **adapter** — its job is "make psmux do a thing" with no
opinion about *why*. The moment you ask "which column does this worktree get, and which pane
should the operator be looking at right now" you are making **policy**, and policy that knows
about worktrees and loom roles does not belong in the adapter. Splitting keeps mux small and
keeps loom free of geometry. The boundary is **mechanism (mux) vs. policy (shed)**:

- mux: `NewPane`, `KillPane`, `ApplyLayout(body)`, `SendKeys`, `CapturePane`, env hygiene,
  resume, the named server. The *capability*.
- shed: column-per-worktree, role placement, bottom-dominant stacking, focus, cluster
  windows — the *decisions*, expressed by calling mux.

## What shed owns

- **The lifecycle ledger.** `{worktree → {role → paneID}}`, persisted to `.lyx/shed.json`
  via [`internal/state`](../shared-libs/README.md). This is the source of truth for "who is
  running where," rebuilt from live psmux on startup (reconcile, not trust).
- **`SpawnAgent(role, worktree)` / `KillAgent(role, worktree)`** — the API loom and agent
  use. shed computes the placement, calls `mux.NewPane`, records the mapping, re-renders the
  layout, and returns a pane handle the caller can drive.
- **Layout policy.** One full-height column per worktree; agent panes stack downward within
  the column (orchestrator on top, handler/builder/reviewer below); the active bottom pane
  dominates the height. shed computes the layout **body** string and hands it to
  `mux.ApplyLayout` (mux owns the checksum + atomic apply — see [mux.md](mux.md#design-model-the-load-bearing-decisions)).
  Recomputed on every spawn/kill.
- **Focus.** On a `Stop` hook that signals needs-input (loom waiting at Discussion or a
  stuck-escalation), shed `select-pane`s the foreground pane for that worktree. At most one
  pane per worktree ever needs keyboard input at a time, so "which pane is the operator's"
  is a single, well-defined choice.
- **Visibility.** All agent panes stay visible even when idle — the operator watches the run
  and can intervene. Visibility is a first-class requirement, not a side effect.
- **Cluster window.** N parallel cluster-reviewers land in a separate named psmux **window**
  (not stacked in the worktree column — that would explode the pane count). **Deferred** until
  cluster-reviews are implemented; until then there is no cluster window.

## What shed does NOT own

- Raw psmux scripting, env hygiene, the named server, resume mechanics, layout-string
  checksum/apply → [`mux`](mux.md).
- Prompt injection, engine selection, what "done" means → [`agent`](agent.md).
- Phase order, review gates, stuck detection → [`loom`](loom.md).

## State: `.lyx/shed.json`

shed's ledger is **ephemeral and machine-local** — pane IDs and a psmux layout are
meaningless on another machine, so they live in `.lyx/` (untracked, in `.git/info/exclude`),
**not** in the weft-synced `_lyx/`. See the
[durable-vs-ephemeral split](../overview.md#durable-vs-ephemeral-state-_lyx-vs-lyx). On
startup shed reconciles the ledger against live `mux` state and `claude agents --json`:
mapped panes with no live session are recovery candidates; live panes with no ledger entry
are orphans.

## Dependencies

- [`internal/mux`](mux.md) — pane/window primitives, layout apply, hook wiring
- `internal/state` — `.lyx/shed.json`
- `internal/paths` — worktree geometry (which worktrees exist, their columns)
