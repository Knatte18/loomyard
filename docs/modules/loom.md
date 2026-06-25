# Loom: the phased orchestrator

> **Status: Design — not built.** This is a plan draft. Per the [documentation
> lifecycle](../overview.md#documentation-lifecycle), when the modules land the durable
> parts of this doc fold into `overview.md` and the package headers, and this file is
> deleted. Until then it is the single design reference for the loom orchestration model.

## What it is

Loom is the orchestrator that takes a task from intent to a merged change through a
fixed sequence of **phases**, each guarded by a uniform **review gate**. The control
flow — phase order, the review round-loop, gate decisions, resume — lives in Go
(`lyx loom run`). The judgment — discussing, planning, building, reviewing, fixing —
lives in agents spawned one-shot per step. Go owns the machine; the LLM owns the thinking.

The orchestrator is the **`loom`** module (`lyx loom run`); the gate engine is the separate,
generic **`review`** module ([`lyx review`](review.md)) — independent of loom but used by it
between every phase. The `/ly-*` skill layer shrinks to thin human-facing
wrappers over these. The everyday call has a convenience alias: **`lyx run` → `lyx loom run`**.
(Naming: `lyx` is the binary, `loom`/`review` are modules, `ly-*` are the skills — see
[overview.md](../overview.md).)

## Why — the inversion

Today the LLM **is** the orchestrator: the `mill-start` / `mill-go` skills encode the
entire machine in prose (every round, gate, branch, handoff, error path) because the
model *executes* it by reading the text. That is why those skills are so long — the
length is structural, not poor writing, as long as control flow lives in a prompt.

Move the machine into Go and orchestration leaves *every* prompt. Each agent collapses
to one job over a file contract:

- Plan producer: "read `discussion.md`, write `plan.md`." Nothing else.
- Review handler: "read `plan.md` (against `discussion.md`), write review + fixer-report."

No agent knows about rounds, gates, N-caps, finalize, or the others. Each phase becomes
a **pure function over files** — input file in, output file out — runnable and testable
in isolation with a fixture. That independence is the real prize: it is also what makes
resume, swapping, and parallelizing easy, because there is no hidden state in a context
window to reconstruct.

**The one discipline that delivers the independence:** the file contract must be the
*only* channel between phases. The moment a phase needs "something from the conversation"
that is not in its input file, the independence is gone and the prompts grow back. So
the design effort moves from writing long skills to pinning the contracts
(`discussion.md` → `plan.md` → diff/report).

## The phase machine

```
Setup
Discussion → [Discussion-review] ─ approved ↓   stuck ─┐
Plan       → [Plan-review]       ─ approved ↓   stuck ─┤
Builder    → [Builder-review]    ─ approved ↓   stuck ─┤
Finalize                                               │
                                       (stuck handler)─┘
```

Setup validates geometry and preconditions (cwd/Hub/Prime via `internal/paths`, clean
worktree, weft pairing present **and in sync** — host branch == weft branch, via
[`warp`](warp.md#drift-detection--when) — no half-finished prior run). Each producing phase emits
a draft artifact and is followed by a review gate. `approved` advances to the next
phase; `stuck` routes to the stuck handler (bounce back to an earlier phase, or escalate
to a human) — never "keep fixing symptoms."

## The gate

Each producing phase is guarded by a **review gate**, and from loom's view that gate is a
**black box with two exits — `APPROVED` or `stuck`.** loom calls it, and on `APPROVED`
advances to the next phase; on `stuck` it routes to the stuck handler (bounce back to an
earlier phase, or escalate to a human) — never "keep fixing symptoms." loom does not see the
rounds, the handler/fixer, the cluster reviewers, or the progress-judge inside.

That black box is its **own module — [`review`](review.md)** (`lyx review`), a generic
profile-driven gate engine reused for every phase (discussion / plan / builder) and standalone.
The whole point of the black-box boundary is that loom drives all phases **identically** because
the verdict contract is invariant; only the review *profile* (rubric + fasit) differs per phase.
See [review.md](review.md) for the round-loop, the combined handler/fixer, stuck detection, and
the profile schema.

## `loom` — the autonomous driver

`lyx loom run` (alias `lyx run`) is the phase machine, and it is essentially autonomous. It reads loom's
**status file** in `_lyx/`, sees which phase (and review sub-state) the task is on, and
continues from there. It is idempotent and re-entrant: **stop anywhere — Ctrl-C, crash,
close the laptop — and the next `lyx run` continues where it left off.**

This is the lyx model applied to orchestration: one-shot, daemonless, file-coordinated,
resume-from-disk. `lyx run` is a pure function of {status file + artifact files} with no
hidden process state. Because the status lives in the weft repo (git-synced), resume
works across machines too. It is per-task and cwd-authoritative ([Principle 4](../overview.md#principles)).

**Human boundaries.** `lyx run` drives every phase it *can* drive **unattended** — the
agents are interactive psmux sessions, but no human sits in them ([Agent execution](#agent-execution)).
When it reaches an inherently interactive boundary — Discussion input, or a `stuck`
escalation — it stops cleanly, writes the next action to the status file, and exits. The
human does the interactive part (which advances the status), and the next `lyx run`
resumes unattended. So `lyx run` is autonomous for everything it can advance and yields
only at the human gates.

**Auto mode.** A run can be told to *never* yield — `lyx run --auto`. The phase machine is
unchanged; the only difference is that at a would-be human gate the agent is instructed to **make
its own best guess and proceed** instead of asking (and the `AskUserQuestion` guardrail —
[mux.md](mux.md#completion-and-hooks-live-in-shuttle-not-mux) — already forbids it from blocking on a dialog). Auto mode
does **not** turn off the view: mux still shows every strand (incl. the `lyx loom status` line),
because you still want to watch. The difference is in loom's *yielding*, not in whether anyone is
looking.

### State & contracts

- **The status file in `_lyx/` is the single source of truth** for orchestration state:
  current phase, current review block + round, and the verdict history the progress-judge
  needs. Nothing orchestration-relevant lives anywhere else.
- **It also carries a human-readable *current-activity* narration** — not just the machine enum,
  but "*now:* spawned plan-handler round 2, waiting on Stop hook / *last:* round 1 BLOCKING, 3
  findings / *wait:* —". This is what the `lyx loom status --watch` strand prints (a 1-line pane at
  the top, [mux.md](mux.md#the-contract-callers-hand-mux-cmd-name-display)) so the operator sees what
  the Go driver is *doing*, not only what the agents are saying. The driver writes the file; the
  status strand reads and prints it — mux never parses it, it just hosts the pane.
- **Round-level resume.** Handler/fixer artifacts are already on disk, so resuming inside
  a review block continues at the current round rather than restarting the phase.
- **Separation of state.** `lyx review` owns its block's round state in the block's files;
  `lyx run`'s status only needs phase + the block's outcome. When `lyx review` returns
  `APPROVED | stuck`, `lyx run` advances.

### Crash recovery — resume on output files, not live processes

After a crash, a restarted `lyx run` cold-starts from the `_lyx/` status file and must reconcile
its logical state with whatever agents may or may not still be alive. The discipline that makes
this tractable: **loom resumes on output FILES, not on live processes.** The file contract means
"was the work done" is decoupled from "is the process alive." For the step it was on:

1. **Is there a complete output file?** → the step finished; read it and advance. (The agent's
   process may be long dead — its result survived. This is the common case.)
2. **Else, is the agent's session still alive?** (via [`mux`](mux.md)'s `.lyx/mux.json` → session
   id → `claude agents --json`) → *working*: re-attach, just wait on its `Stop` hook (do **not**
   respawn — that would duplicate). *blocked*: it is a human gate / stuck — surface it.
3. **Else (dead, no output):** respawn a **fresh** agent for the step, hydrated from the prior
   round's on-disk artifacts. The round is idempotent, so a fresh handler is deterministic.

loom therefore **never depends on `claude --resume` for correctness** — an unfinished step is
respawned, not resumed (mux's `--resume` is finicky for programmatically-driven sessions, and a
never-conversed session has nothing to resume). mux's pane-`--resume` is a *separate, non-critical*
layer that restores the **visible** sessions for the operator
([mux.md](mux.md#resume-after-crash--native---resume-with-env-hygiene)); loom's correctness rests on
files. A dead claude with a finished output file is, to loom, a **done step** — not a problem.

## Module decomposition

| Piece | Form | Notes |
|-------|------|-------|
| `loom` (`lyx loom run`) | new Go module | the phase machine / autonomous driver |
| `review` (`lyx review`) | new Go module | the gate engine: Handler+fixer + optional cluster + progress-judge loop |
| producers (discussion / plan / builder) | prompt/profile files | **not** modules — just a prompt + profile fed to `shuttle.Run` |
| `lyx loom status` | a loom subcommand | the 1-line status view; runs as a [strand](mux.md#the-strand-model) (`anchor:top`), not a separate module |
| execution stack | existing/new infra | [`proc`](README.md) → [`mux`](mux.md) → [`shuttle`](shuttle.md) — built once, used by both modules above |
| Setup | uses existing modules | `worktree`, `weft`, `board` |
| `/ly-*` skills | thin wrappers | over `lyx loom run` |

The new Go specific to loom is the **two modules** (`loom`, `review`) plus the `lyx loom status`
subcommand; beneath them is the shared [execution stack](README.md) (`proc`, `mux`, `shuttle`); and
everything else is prompt files, profiles, and the existing lyx modules. The display is **not** a
module — it is `lyx loom status` running in a strand that [`mux`](mux.md) hosts and arranges.

## Entry point — the session bootstrap

Today: launch `claude` in a terminal, then `/mill-start` — an interactive LLM session drives
everything. Loom inverts this: `lyx loom run` (alias `lyx run`) is the **session bootstrap** —
more than the driver alone. Run in a worktree's pane, it:

```
lyx loom run:
  1. ensure the worktree's psmux session is up           (mux)
  2. add the status strand                                (mux.AddStrand "lyx loom status --watch",
                                                           display: anchor:top, height:fixed(1))
  3. spawn the loom driver DETACHED                       (internal/proc — it needs no TTY;
                                                           it reads/writes files, drives strands via mux)
  4. attach the current terminal to the psmux session     (mux takes the foreground)
```

So **loom goes to the background and the psmux session takes the window.** loom needs no terminal —
it coordinates through files and drives strands via mux — so the screen is free for the mux view
(the status line on top, agents below as they spawn). loom and the view are independent: loom writes
the `_lyx/` status file; the status strand reads and prints it; neither blocks the other.

**The run-launcher.** A double-click shortcut makes this one click: `lyx worktree add` drops a
small `.lyx/lyxrun.cmd` (machine-local, untracked — it embeds an absolute path) in the worktree
that just does `cd <worktree>` then `lyx loom run`. Because everything is
[cwd-authoritative](../overview.md#principles), the launcher needs no arguments — geometry resolves
from cwd, so you cannot run it from the wrong place. It reuses the
[launcher geometry](../overview.md#path-invariants) already in `internal/paths`.

**One terminal per worktree.** Scope for now is exactly that — each worktree its own terminal /
psmux session. The cross-worktree multi-column view (all worktrees in one window) is a deferred mux
feature ([mux.md](mux.md#scope-one-terminal-per-worktree-now-cross-worktree-columns-later)) — cheap
when it comes (a `worktree` strand field + a grouping rule), but not now.

## Agent execution

Every agent loom spawns — producers, the review handler, cluster reviewers, the
progress-judge — runs through the [`internal/shuttle`](shuttle.md) layer as an **interactive
psmux session, never headless `claude -p`** (an economic constraint; see
[shuttle.md](shuttle.md#interactive-never-headless--the-economic-constraint)). **I/O still rides
the file contract** — the agent writes its output files and Go reads them — so the
file-contract design above is unchanged; only the *spawn + completion-detection* mechanism
differs from a headless model.

The consequence for loom: it sits on top of the [`proc → mux → shuttle`](README.md) stack, so that
stack is on loom's critical path. loom (via [`review`](review.md)) calls `shuttle.Run` per spawn and
stays ignorant of strands, layout, and engines — those belong to [`mux`](mux.md) (the strand
bookkeeping + render: which pane is which, layout, focus, the cluster window where N reviewers go)
and [`shuttle`](shuttle.md) (the swappable provider engine). What loom owns is everything in this
document: the phase machine, the gate wiring, and the status contract.

## Principle alignment

- **One-shot, daemonless, file-coordinated** ([Principle 3](../overview.md#principles)) — `lyx run`
  and `lyx review` are processes that read state, act, and exit; they cooperate through
  files and the status file, not a server.
- **cwd-authoritative** ([Principle 4](../overview.md#principles)) — `lyx run` operates on the
  current worktree's task.
- **Correctness by tool-design** ([Principle 6](../overview.md#principles)) — moving control flow
  into Go makes the correct sequence the only sequence: the machine cannot forget a phase,
  skip a gate, or miscount rounds the way a prose-driven LLM orchestrator can.

The through-line: **the more of the orchestration that is Go / lyx, the faster, cheaper,
and more resumable it gets** — every step moved out of an LLM context is a step that costs
no tokens, cannot drift, and survives a restart.
