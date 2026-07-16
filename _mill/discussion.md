# Discussion: Built-in operator console pane in mux

```yaml
task: Built-in operator console pane in mux
slug: mux-operator-console
status: discussing
parent: main
```

> **⏳ IN PROGRESS** — written incrementally during the mill-start interview
> (after the original discussion thread was lost to an accidental Ctrl+C).
> Sections marked `⏳ OPEN` are not yet settled.

## Problem

During the session-fork-diversity-spike (2026-07-16), removing mux strands
down to the last one tore down the whole tmux session — tmux kills a session
when its last pane dies — which killed a running test mid-flight. The ad-hoc
fix was a manually added keepalive strand running a bare `bash` in the hub
root.

**Why now:** the teardown is a live footgun (it already broke a test). The
spike left an operator-endorsed design note (`docs/research/session-fork-spike.md:233-239`)
to make the keepalive a built-in mux feature.

## Scope

**In:**

- A new built-in **header** construct in mux: a small, always-present tmux/psmux
  pane pinned at the **top**, at a **config-driven fixed height** (default 1 row).
- The header pane runs a new command `lyx mux header`, which prints text defined
  by mux config (e.g. the hub path) and then **hangs** so the pane never exits.
- Purpose: keep the session structurally alive — the "remove last strand kills the
  session" edge becomes **unreachable**.
- The header is **NOT a strand**: separate construct, not in the `Strands` slice,
  excluded from all strand accounting (`status`, `resume`, `remove`, count).
- **One header per column** (see decision `header-per-column`).
- Header content is configurable via a template filled by the existing
  `internal/stencil` module (`{{.token}}` markers).
- New rendering: a pinned fixed-height **top band** in the render layer (no such
  anchor exists today).

**Out:**

- Interactive scratch-terminal use — **dropped**. The header is pure display; it only
  runs `lyx mux header` (print + hang), never a usable shell.
- Building a brand-new token-substitution module — **rejected** in favour of reusing
  `internal/stencil`.
- General pane naming/tagging as a mux feature (separate spike gap, not this task).

## Decisions

### header-is-not-a-strand (BINDING — user-stated)

- Decision: The header is a first-class but separate construct, **never** a strand.
  Persist its pane id outside the `Strands` slice (new `MuxState` field, e.g.
  `HeaderPaneID` — ⏳ per-column form TBD). Special-case it through boot / reconcile /
  apply / status.
- Rationale: User requires it not be a strand; keeps strand accounting clean.
- Rejected: hidden/special strand (user rejected); own-window keepalive (not a visible
  top band).

### pure-header (Q1)

- Decision: The header is display-only. `lyx mux header` prints the config-defined text
  (token-substituted) and then blocks so the pane stays alive. No interactive use.
- Rationale: A 1-row header cannot host manual jobs; the keepalive value comes from the
  long-lived process, not from operator interaction.
- Rejected: scratch terminal (would need a real shell + more rows).

### token-substitution via stencil (Q2)

- Decision: Reuse `internal/stencil` (`Fill(template []byte, values map[string]string)`)
  for header text. The header template uses `{{.token}}` markers (Go text/template), not
  `<TOKEN>` angle brackets.
- Rationale: `stencil` already does exactly "template + token map → filled text" with
  strict unfilled-marker detection (`stencil.go:36`). Reuse satisfies CONSTRAINTS; no
  duplicate module.
- Rejected: new `<TOKEN_NAME>` module; extending stencil with angle-bracket syntax.

### top-band-in-render (Q3)

- Decision: Add a new pinned fixed-height **top band** placement to the render layer for
  the header pane — positioned at the top, outside the strand stack, visible alongside
  strands. **One header per column.**
- Rationale: Matches "header at the top, visible over the strands." No `anchor:top` exists
  today (`render/types.go:23-39`), so this is new render work (new `policy.go` case +
  `layout.go`/`height.go` mechanics).
- Rejected: header in its own tmux window (`AnchorOwnWindow`) — keeps the session alive but
  is not a visible top band.

### header-per-column (Q3 follow-up — ⏳ needs column-model clarification)

- Decision (stated by user): **at most one header per column.**
- ⏳ OPEN: mux's render is a single vertical below-parent stack today; the "column" concept
  needs defining before this can be planned. _batch 2 Q5._

### header-config-block (Q4)

- Decision: Add a `header:` block to the mux config (`muxengine/config.go` +
  `template.go ConfigTemplate`): the text template + `height_rows` (default 1).
- Rationale: One place, follows the existing mux-config pattern.
- Rejected: separate config file.

## Technical context

Full codebase map: `.scratch/mux-console-exploration.md`. Highlights:

- **`internal/stencil`** — `Fill(template, values)` (`stencil.go:36`), strict unfilled-marker
  error. Reused for header text.
- **No `anchor:top` band exists** — `render/types.go:23-39` vocab is
  `below-parent | own-window (deferred) | hidden`. Top band = new render work.
- **Most dangerous seam:** `reconcile.go:93-113` — `planReconcile` kills any live pane not in
  `boundPaneIDs`. The header pane id must be threaded into an exemption set or it gets reaped.
- **Accounting exclusion points:** `lifecycle.go:415` (`UpResult.Strands`), `lifecycle.go:922-925`
  (`Status`), `noSessionMessage`/`strandCount` (`lifecycle.go:842-847`).
- **Last-pane kill site:** `strand.go:485-487` (`RemoveStrand`'s explicit kill-pane loop);
  `reconcile.go:59-79` `keptDeadPane` (partial guard, made moot by a permanent header).
- **Pane creation:** `launchStrandLocked` splits with **no `-c`** cwd (`spawn.go:110-146`); only
  boot `new-session -c <layout.Cwd>` (`lifecycle.go:294-302`). Header wants hub root → new
  `split-window -c <e.layout.Hub>`.
- **Hub root:** `hubgeometry.Layout.Hub = filepath.Dir(WorktreeRoot)` (`hubgeometry.go:117`).
- **tmux layer:** `overlay.go` `TmuxCmd` is the only subprocess seam. Panes identified by `#{pane_id}`.
- **Boot:** header created during `ensureServerAndSessionLocked` / `Up`, re-created if missing across `up`.

## Constraints

- CONSTRAINTS.md: **Hub Geometry Invariant** — all cwd/geometry via `internal/hubgeometry`
  (`layout.Hub`, never recompute). **CLI / Cobra Invariant** — `lyx mux header` needs a `Short`,
  the `Command()`/`RunCLI` seam, help-tree tests. **lyxtest Leaf Invariant** for fixtures.
  **Documentation Lifecycle** — update `docs/modules/` + `docs/overview.md` same commit; record any
  new cross-cutting invariant in CONSTRAINTS.md.
- Seam `muxcli → muxengine → render` is one-directional; the header work must not break it.

## Testing

Candidates (refine as decisions settle):

- Header text render (TDD): `lyx mux header` / the render helper produces the stencil-filled
  template; unknown/missing token surfaces the stencil error.
- Keepalive regression: flip `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds`
  (`contract_integration_test.go:332`) — with a header present, removing the last strand leaves
  the session + header alive, strand count reaching zero.
- Reconcile exemption: mirror `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`
  (`smoke_lifecycle_test.go:237`) — header pane survives reconcile even with strands bound.
- Top-band render (hermetic): header placed at top, fixed height, strand stack below unaffected.

## Q&A log

- **Q:** Bash scratch terminal or pure header? **A:** Pure header — fixed height, `lyx mux header`
  prints config-defined text and hangs. Not an interactive shell.
- **Q:** Is the header a strand? **A:** No — separate construct, excluded from strand accounting.
- **Q:** How is header content configured? **A:** A config-defined template filled by `internal/stencil`
  (`{{.token}}` markers). Reuse, not a new module.
- **Q:** How is the fixed top header realised? **A:** New pinned fixed-height top-band placement in the
  render layer; header is its own thing; **one header per column**.
- **Q:** Where does header config live? **A:** A `header:` block in the mux config (template + height rows,
  default 1).
