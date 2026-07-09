# Module design docs — index & the execution stack

> **This is an index, not a module.** The other files in this folder are **per-module design
> docs** (one module each, deleted when the module lands per the
> [doc lifecycle](../overview.md#documentation-lifecycle)). This README is the **map**: how the
> modules fit together.

## The module docs

| Doc | Module | One-line role | CLI? |
|-----|--------|---------------|------|
| — | `internal/muxengine` | the window to the world: psmux overlay + **strand** bookkeeping + render (✅ built — see [overview.md#modules](../overview.md#modules) and the package documentation) | `lyx mux` |
| — | `internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli` | run **one** LLM agent via a swappable engine over the file contract (✅ built — see [overview.md#modules](../overview.md#modules) and the package documentation) | `lyx shuttle` |
| — | `internal/burlerengine` + `internal/burlercli` | one review+fix round: A-review → B-fix, no self-grading (✅ built — see [overview.md#modules](../overview.md#modules) and the package documentation) | `lyx burler run` (debug only, composed by `perch`) |
| — | `internal/perchengine` + `internal/perchcli` | the gate loop: run `burler` rounds → `APPROVED`/`STUCK` (+ operational `PAUSED`) + a holistic progress judge (✅ built — see [overview.md#modules](../overview.md#modules) and the package documentation) | `lyx perch run|pause` |
| [loom.md](loom.md) | `loom` | the phase machine: drive each phase through a perch gate | `lyx loom` |
| [hardener.md](hardener.md) | `hardener` | **DRAFT** — behavior-based hardening of live-substrate modules (post-loom, off-spine) | `lyx hardener`? |

`internal/proc` (cross-OS process spawn) is the OS base under `mux`; it has no doc of its own —
see the stack below and [shared-libs](../shared-libs/README.md).

**`warp` is not part of the execution stack below** — it is a *setup-track* module that
consolidates the host↔weft git topology (the former `internal/worktree` and
`internal/gitclone` modules plus the renamed `internal/gitexec` leaf). It owns the mirror
invariant: every lyx-managed host branch/worktree has a paired weft counterpart, kept
consistent on add / checkout / clone / reconcile. ✅ Implemented — its design rationale now
lives in the `internal/warpengine` package header (the design doc was deleted when the module
landed, per the [doc lifecycle](../overview.md#documentation-lifecycle)).

## Why a stack at all

Spawning an agent is **not** a plain `exec`. Agents must run as **interactive psmux sessions** (an
economic constraint — see the `internal/shuttleengine` package documentation),
so "run one agent" decomposes into: *start a process → make it a visible/interactive pane → run the
LLM in it → decide the result.* Each layer knows only the one below it.

```
internal/proc      start an OS process (windowless / detached, cross-OS)   [knows: the OS]
internal/mux       the window to the world — overlay + strands + render    [knows: psmux]
internal/shuttle   run one LLM agent in a strand, get a result file        [knows: prompts & engines]
─────
burler             one review+fix round: A-review (+cluster) → B-fix        [knows: rubrics & a round]
perch              run burler rounds on one artifact → APPROVED|stuck       [knows: convergence & stuck]
loom               drive phases, each through a perch gate                  [knows: phases]
```

`hardener` (**DRAFT**, [hardener.md](hardener.md)) is **off this spine** — a separate, on-demand,
post-loom behavior-based reviewer that shares only the `burler` round discipline.

The control stack runs **headless** (auto mode): panes exist (the interactive-session
requirement), agents run, output files are read, and nobody need watch.

## mux is three things in one

Earlier drafts split the model and the view into separate `shed` / `glance` modules. With **one
terminal per worktree** and a **closed, generic display vocabulary**, all three collapse cleanly
into mux without dragging domain knowledge in:

1. **Overlay** over psmux — every psmux command, env hygiene, resume, hooks, named server.
2. **Strand bookkeeping** — a strand (see the `internal/muxengine` package documentation) is one
   tracked process (a metadata record: session, worktree slug, parent, generic `display` spec),
   persisted to `.lyx/mux.json`.
3. **Render** (`internal/muxengine/render`) — a pure
   function `layout = rules(strands)` over the generic vocabulary. Kept an internal sub-package so
   it can split back out if mux bloats.

The key discipline: callers hand mux `{cmd, display}` where `display` is **generic** (anchor,
height, focus) — never a domain `type`. mux never learns what a "phase" or a "cluster" is; the
caller maps its domain to the generic vocabulary (the CSS model: `position: sticky`, not "navbar").

## Following one spawn down the stack

loom wants a plan-reviewer for worktree `feature-x`:

1. `loom` → `perch.Run(profile, "feature-x")` — "review this plan against the discussion until clean."
2. `perch` → `burler.Run(profile, priorFiles)` — "run one review+fix round."
3. `burler` → `shuttle.Run(prompt, engine)` — "run one handler agent."
4. `shuttle` → `mux.AddStrand{ cmd:"claude …", worktree:"feature-x", display:{anchor:below-parent, focus:true} }`.
5. `mux` records the strand in `.lyx/mux.json`, runs the command via `proc` in a pane, re-renders
   the layout (`layout = rules(strands)`), and applies it.
6. The `Stop` hook fires → mux notes the edge → shuttle reads the output file → returns to burler →
   burler writes review/fixer-report + verdict → perch reads it, decides loop or exit → on a clean
   round returns `APPROVED | stuck` → loom advances.

## The disambiguating test

- About **the OS**? → `proc`.
- About **a psmux mechanic, a strand, or how it's laid out**? → `mux`.
- About **running an LLM and getting its answer**? → `shuttle`.
- About **one review+fix round**? → `burler`.
- About **whether an artifact passes (loop rounds until clean/stuck)**? → `perch`.
- About **hardening a live-substrate module by running it** (post-loom, off-spine)? → `hardener` (DRAFT).
- About **what to run next**? → `loom`.
