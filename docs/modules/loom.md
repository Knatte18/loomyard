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
generic **`perch`** module (`lyx perch run|pause` — see the `internal/perchengine` package
documentation) — the iterative review loop, independent of loom but used by it between every
phase. `perch` composes `burler` (see the `internal/burlerengine`
package documentation), the review+fix round worker. The `/ly-*` skill layer shrinks to thin
human-facing wrappers over these. The everyday call has a convenience alias:
**`lyx run` → `lyx loom run`**. (Naming: `lyx` is the binary,
`loom`/`perch`/`burler` are modules, `ly-*` are the skills — see [overview.md](../overview.md).)

## Why — the inversion

Today the LLM **is** the orchestrator: the `mill-start` / `mill-go` skills encode the
entire machine in prose (every round, gate, branch, handoff, error path) because the
model *executes* it by reading the text. That is why those skills are so long — the
length is structural, not poor writing, as long as control flow lives in a prompt.

Move the machine into Go and orchestration leaves *every* prompt. Each agent collapses
to one job over a file contract:

- Plan producer: "read `discussion.md`, write the `plan/` directory
  ([plan format](plan-format.md))." Nothing else.
- Review handler: "read the plan (against `discussion.md`), write review + fixer-report."

No agent knows about rounds, gates, N-caps, finalize, or the others. Each phase becomes
a **pure function over files** — input file in, output file out — runnable and testable
in isolation with a fixture. That independence is the real prize: it is also what makes
resume, swapping, and parallelizing easy, because there is no hidden state in a context
window to reconstruct.

**The one discipline that delivers the independence:** the file contract must be the
*only* channel between phases. The moment a phase needs "something from the conversation"
that is not in its input file, the independence is gone and the prompts grow back. So
the design effort moves from writing long skills to pinning the contracts
(`discussion.md` → `plan/` → diff/report).

## The phase machine

```
Setup
Discussion → [Discussion-review] ─ approved ↓   stuck ─┐
Plan       → [Plan-review]       ─ approved ↓   stuck ─┤
Builder    → [Builder-review]    ─ approved ↓   stuck ─┤
Raddle     (git-diff-targeted docs)                    │
Finalize                                               │
                                       (stuck handler)─┘
```

Setup validates geometry and preconditions (cwd/Hub/Prime via `internal/hubgeometry`, clean
worktree, weft pairing present **and in sync** — host branch == weft branch, via
[`warp`](warp.md#drift-detection--when) — no half-finished prior run). Each producing phase emits
a draft artifact and is followed by a review gate. `approved` advances to the next
phase; `stuck` routes to the stuck handler (bounce back to an earlier phase, or escalate
to a human) — never "keep fixing symptoms."

**Raddle** is a dedicated step after Builder — deliberately *not* the implementer's job.
Experience (millhouse) is that implementers, busy with code, forget the docs; a dedicated
always-run step removes the dependency on anyone remembering, and a fresh-context agent
reading only the diff often writes better docs than the implementer who is "done in their
head." Mechanism: loom stamps a **start-SHA** (host `HEAD`) into the status file when Builder
begins; the Builder agent **commits its own work** (required anyway — for backtracking, and
so there is a diff to read). The Raddle step then generates docs over
`git diff <start-SHA>..HEAD` on the host (excluding `_lyx`/`_raddle`) for a targeted
update — **building heavily on millhouse's `codeguide-update`** — and commits the docs into the weft via `lyx weft sync` (never raw git — see the
[warp responsibility boundary](warp.md#responsibility-boundary--warp-vs-weft-vs-host)). The
`_raddle` merge-back at Finalize is exactly what `warp cleanup` gates on. (Whether the
Raddle step is itself review-gated is an open choice; shown ungated above.)

## The gate

Each producing phase is guarded by a **review gate**, and from loom's view that gate is a
**black box with two exits — `APPROVED` or `stuck`.** loom calls it, and on `APPROVED`
advances to the next phase; on `stuck` it routes to the stuck handler (bounce back to an
earlier phase, or escalate to a human) — never "keep fixing symptoms." loom does not see the
rounds, the handler/fixer, the cluster reviewers, or the progress-judge inside.

That black box is its **own module — `perch`** (`lyx perch run|pause`), a generic profile-driven
gate engine reused for every phase (discussion / plan / builder) and standalone. The whole point of
the black-box boundary is that loom drives all phases **identically** because the verdict contract is
invariant; only the review *profile* (rubric + fasit) differs per phase. See the `internal/perchengine`
package documentation for the round-loop and stuck detection, and the `internal/burlerengine` package
documentation for the combined handler/fixer round and the profile schema.

## Builder — an LLM orchestrator over Go verbs (advance), the sibling of perch (converge)

Unlike the discussion and plan producers (each one `shuttle.Run` → one artifact), **Builder is a
batch loop held by a long-lived LLM orchestrator session** (model config-chosen; Sonnet default)
that drives fat **`lyx builder` verbs** — `spawn-batch`, `poll`, `status` — provided by Go
(`internal/builderengine`). Go supplies **only the verbs plus the distillation behind them**: it
does not hold the loop, iterate batches, or make orchestration decisions. This is the one
deliberate exception to "Go owns the machine": a pure Go-driven batch loop was explicitly
rejected — mill-go's context bloat came from the LLM orchestrator swallowing verbose sub-agent
output, not from the loop being LLM-held.

- **Advance per batch, end at batches-built.** The orchestrator drives the plan's batches
  strictly in order (ordered list, **no DAG** — the plan contract is pinned in
  [plan-format.md](plan-format.md)), spawning one implementer worker per batch (config-chosen
  model; Sonnet default — see [model-spec](../reference/model-spec.md)). `builder run` itself
  ends the moment the last batch is green (or the run reports stuck/paused) — it runs **no**
  review of its own. The terminal holistic review is the separate **Builder-review gate**: a
  full `perch` converge-loop over the whole diff, driven by `loom` (or the operator running
  `lyx perch run` directly) *after* `builder run` returns `done`; no per-batch design review in
  v1. See [modules/builder.md](builder.md) for the as-built verb surface and digest contract.
- **Digest-only consumption.** The `poll` verb reads the implementer's on-disk batch-report,
  distills it in Go, and returns a terse digest; the orchestrator never ingests raw session
  prose. That is what keeps a persistent LLM orchestrator lean.
- **Escalation by fresh spawn.** A stuck worker is escalated by the orchestrator spawning a
  **fresh higher-capability model** that reads the durable reports — not a `/model` switch
  inside the stuck session (which would inherit the polluted context; see the
  `internal/shuttleengine` package documentation for the escalation rationale).

**Same substrate, different loop semantics:** Builder **advances** (batch → batch → done);
perch **converges** (iterate review+fix on one artifact until `APPROVED`/`stuck`) — the
Builder-review gate is perch running *after* builder, never a phase inside builder's own loop.
Pause stays uniform across them (see [pause](#graceful-pause)): builder's verbs check the pause
flag at the batch boundary even though its loop is LLM-held.

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
agents are interactive tmux sessions, but no human sits in them ([Agent execution](#agent-execution)).
When it reaches an inherently interactive boundary — Discussion input, or a `stuck`
escalation — it stops cleanly, writes the next action to the status file, and exits. The
human does the interactive part (which advances the status), and the next `lyx run`
resumes unattended. So `lyx run` is autonomous for everything it can advance and yields
only at the human gates.

**Auto mode.** A run can be told to *never* yield — `lyx run --auto`. The phase machine is
unchanged; the only difference is that at a would-be human gate the agent is instructed to **make
its own best guess and proceed** instead of asking (and the `AskUserQuestion` guardrail — see the
`internal/shuttleengine/claudeengine` package documentation — already forbids it from blocking on a dialog). Auto mode
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
  the top, per the `internal/muxengine` package documentation on the strand contract) so the operator sees what
  the Go driver is *doing*, not only what the agents are saying. The driver writes the file; the
  status strand reads and prints it — mux never parses it, it just hosts the pane.
- **Round-level resume.** Handler/fixer artifacts are already on disk, so resuming inside
  a review block continues at the current round rather than restarting the phase.
- **Separation of state.** `lyx perch` owns its block's round state in the block's files;
  `lyx run`'s status only needs phase + the block's outcome. When `lyx perch` returns
  `APPROVED | stuck`, `lyx run` advances.

### Crash recovery — resume on output files, not live processes

After a crash, a restarted `lyx run` cold-starts from the `_lyx/` status file and must reconcile
its logical state with whatever agents may or may not still be alive. The discipline that makes
this tractable: **loom resumes on output FILES, not on live processes.** The file contract means
"was the work done" is decoupled from "is the process alive." For the step it was on:

1. **Is there a complete output file?** → the step finished; read it and advance. (The agent's
   process may be long dead — its result survived. This is the common case.)
2. **Else, is the agent's session still alive?** (via `mux`'s — see
   [overview.md#modules](../overview.md#modules) — `.lyx/mux.json` → session
   id → `claude agents --json`) → *working*: re-attach, just wait on its `Stop` hook (do **not**
   respawn — that would duplicate). *blocked*: it is a human gate / stuck — surface it.
3. **Else (dead, no output):** respawn a **fresh** agent for the step, hydrated from the prior
   round's on-disk artifacts. The round is idempotent, so a fresh handler is deterministic.

loom therefore **never depends on `claude --resume` for correctness** — an unfinished step is
respawned, not resumed (mux's `--resume` is finicky for programmatically-driven sessions, and a
never-conversed session has nothing to resume). mux's pane-`--resume` is a *separate, non-critical*
layer that restores the **visible** sessions for the operator (see the `internal/muxengine` package
documentation on resume); loom's correctness rests on
files. A dead claude with a finished output file is, to loom, a **done step** — not a problem.

## Graceful pause

`lyx loom pause` requests a pause; the running orchestration honours it at the next **step
boundary**, never mid-operation — `mill-pause`'s natural-stopping-point property, made systematic.

- **A property of the loop pattern, not loom alone.** Every loop — loom (phases),
  `perch` (rounds), [Builder](#builder--an-llm-orchestrator-over-go-verbs-advance-the-sibling-of-perch-converge)
  (batches; its loop is LLM-held, but the `spawn-batch` verb checks the flag in Go before
  spawning) — checks a `pause_requested` flag in the [status file](#state--contracts) at its step
  boundary and stops before spawning the next unit. The **innermost active loop** honours it first,
  so pause lands at the finest active boundary (next batch / round / phase). The Go code is almost
  always *between* steps (it spawns and waits), so catching it there is trivial.
- **The leaf agent finishes its unit; nothing is killed.** Boundary pause lets the in-flight worker
  complete its small unit (one batch / round — its output file written), then the driver stops.
  Resume (`lyx loom run`) spawns the next step from the status file — the same resume-on-files
  discipline as [crash recovery](#crash-recovery--resume-on-output-files-not-live-processes), minus
  the crash.
- **In-agent interrupt is optional.** To pause *faster* than the current unit finishes,
  `shuttle` (see the `internal/shuttleengine` package documentation) can ESC-and-hold the live
  agent (session kept warm in the mux server — see [overview.md#modules](../overview.md#modules),
  not killed; resume continues it in place). With Builder
  decomposed into batches/cards the boundary wait is short, so this is a latency nicety, not a
  correctness requirement.
- **Distinct from crash recovery.** Crash (involuntary death) respawns a fresh agent from the
  on-disk output files (loom never relies on `claude --resume` for correctness — see above). Pause
  deliberately stops at a boundary, so there is nothing to respawn — the cheaper path. Both rest on
  the file contract; pause just avoids the death.

## Module decomposition

| Piece | Form | Notes |
|-------|------|-------|
| `loom` (`lyx loom run`) | new Go module | the phase machine / autonomous driver |
| `perch` (`lyx perch`) | new Go module | the gate loop: run `burler` rounds → `APPROVED`/`stuck` + progress-judge + cap |
| `burler` | new Go module | one review+fix round: A-review (+ optional cluster) → B-fix; composed by `perch` |
| builder | LLM orchestrator + Go verbs (`internal/builderengine`) | long-lived orchestrator session holds the batch loop over the six as-built verbs (`validate`/`run`/`spawn-batch`/`poll`/`status`/`pause`); Go = verbs + distillation; fresh-spawn escalation; ends at batches-built — the holistic review is perch's separate Builder-review gate, not builder's own job — **not** a single producer spawn; input contract: [plan-format.md](plan-format.md); as-built doc: [builder.md](builder.md) |
| producers (discussion / plan) | prompt/profile files | **not** modules — just a prompt + profile fed to `shuttle.Run` |
| `lyx loom status` | a loom subcommand | the 1-line status view; runs as a strand (see `internal/muxengine`; `below-parent` + `ShrinkWhenWaitingOnChild`), not a separate module |
| execution stack | existing/new infra | [`proc`](README.md) → mux → shuttle — see [overview.md#execution-stack](../overview.md#execution-stack-orchestration-layers) — built once, used by both modules above |
| Setup | uses existing modules | `warp` (topology owner), `weft`, `board` |
| `/ly-*` skills | thin wrappers | over `lyx loom run` |

The new Go specific to loom is the **three modules** (`loom`, `perch`, `burler`) plus the
**builder module** (`internal/builderengine` — the fat verbs + distillation the Builder
orchestrator drives) and the `lyx loom status` subcommand; beneath them is the shared [execution stack](README.md) (`proc`, `mux`, `shuttle`); and
everything else is prompt files, profiles, and the existing lyx modules. The display is **not** a
module — it is `lyx loom status` running in a strand that `mux` (see
[overview.md#modules](../overview.md#modules)) hosts and arranges.

## Entry point — the session bootstrap

Today: launch `claude` in a terminal, then `/mill-start` — an interactive LLM session drives
everything. Loom inverts this: `lyx loom run` (alias `lyx run`) is the **session bootstrap** —
more than the driver alone. Run in a worktree's pane, it:

```
lyx loom run:
  1. ensure the worktree's tmux session is up           (mux)
  2. add the status strand                                (mux.AddStrand "lyx loom status --watch",
                                                           display: below-parent, shrinkWhenWaitingOnChild:true —
                                                           full height while it has no live child, collapsing to
                                                           collapsed_strip_rows once a forked child exists. A
                                                           childless status strand rendering full-height is
                                                           intended, not a bug to re-file (discussion Decision
                                                           childless-full-height-is-acceptable).)
  3. spawn the loom driver DETACHED                       (internal/proc — it needs no TTY;
                                                           it reads/writes files, drives strands via mux)
  4. attach the current terminal to the tmux session     (mux takes the foreground)
```

So **loom goes to the background and the tmux session takes the window.** loom needs no terminal —
it coordinates through files and drives strands via mux — so the screen is free for the mux view
(the status line on top, agents below as they spawn). loom and the view are independent: loom writes
the `_lyx/` status file; the status strand reads and prints it; neither blocks the other.

**The run-launcher.** A double-click shortcut makes this one click: `lyx warp add` drops a
small `.lyx/lyxrun.cmd` (machine-local, untracked — it embeds an absolute path) in the worktree
that just does `cd <worktree>` then `lyx loom run`. Because everything is
[cwd-authoritative](../overview.md#principles), the launcher needs no arguments — geometry resolves
from cwd, so you cannot run it from the wrong place. It reuses the
[launcher geometry](../overview.md#hub-geometry-invariants) already in `internal/hubgeometry`.

**One terminal per worktree.** Scope for now is exactly that — each worktree its own terminal /
tmux session. The cross-worktree multi-column view (all worktrees in one window) is a deferred mux
feature (see the `internal/muxengine` package documentation) — cheap
when it comes (a `worktree` strand field + a grouping rule), but not now.

## Agent execution

Every agent loom spawns — producers, the review handler, cluster reviewers, the
progress-judge — runs through the `internal/shuttleengine` layer as an **interactive
tmux session, never headless `claude -p`** (an economic constraint; see the
`internal/shuttleengine` package documentation). **I/O still rides
the file contract** — the agent writes its output files and Go reads them — so the
file-contract design above is unchanged; only the *spawn + completion-detection* mechanism
differs from a headless model.

The consequence for loom: it sits on top of the [`proc → mux → shuttle`](README.md) stack, so that
stack is on loom's critical path. loom (via `perch` — see the `internal/perchengine` package
documentation — → `burler`, see the `internal/burlerengine` package documentation) calls
`shuttle.Run` per spawn and stays ignorant of strands, layout, and engines — those belong to `mux` (see
[overview.md#modules](../overview.md#modules); the strand
bookkeeping + render: which pane is which, layout, focus, the cluster window where N reviewers go)
and `shuttle` (see the `internal/shuttleengine` package documentation; the swappable provider engine). What loom owns is everything in this
document: the phase machine, the gate wiring, and the status contract.

## Principle alignment

- **One-shot, daemonless, file-coordinated** ([Principle 3](../overview.md#principles)) — `lyx run`
  and `lyx perch` are processes that read state, act, and exit; they cooperate through
  files and the status file, not a server.
- **cwd-authoritative** ([Principle 4](../overview.md#principles)) — `lyx run` operates on the
  current worktree's task.
- **Correctness by tool-design** ([Principle 6](../overview.md#principles)) — moving control flow
  into Go makes the correct sequence the only sequence: the machine cannot forget a phase,
  skip a gate, or miscount rounds the way a prose-driven LLM orchestrator can.

The through-line: **the more of the orchestration that is Go / lyx, the faster, cheaper,
and more resumable it gets** — every step moved out of an LLM context is a step that costs
no tokens, cannot drift, and survives a restart.
