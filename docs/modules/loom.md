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

The orchestrator is the **`loom`** module (`lyx loom run`); the gate engine is the generic
**`review`** module (`lyx review`). The `/ly-*` skill layer shrinks to thin human-facing
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
worktree, weft pairing present, no half-finished prior run). Each producing phase emits
a draft artifact and is followed by a review gate. `approved` advances to the next
phase; `stuck` routes to the stuck handler (bounce back to an earlier phase, or escalate
to a human) — never "keep fixing symptoms."

## The X-review block (the gate)

From the orchestrator's view a review is a **black box** with two exits: `APPROVED` or
`stuck`. What happens inside is not the orchestrator's concern; it is not finished until
the artifact is approved or the block is definitively stuck.

Inside, Go drives a round-loop (there is no standing per-block orchestrator agent — that
was only an LLM today because orchestration was LLM-driven):

1. Go spawns a fresh, **tool-based Handler** for the round.
2. **A — Review.** The Handler reviews the artifact like a normal reviewer, not yet
   knowing it will also fix. It writes a review to file with a verdict: `BLOCKING` or
   `APPROVED`. In step A it **may** spawn N extra **cluster reviewers**, wait for them
   (or time out), and write a cross-checked review — this is how cluster-review support
   falls out for free.
3. **B — Fix.** The Handler then fixes what it found, itself, based on its own review
   plus its own reasoning — **even if the verdict was `APPROVED`** (non-blocking polish).
   It writes a fixer-report.
4. Control returns to Go, which reads the round status. If not `APPROVED`, it spawns a
   **new** Handler for the next round (2–3 again).

The Handler combines review and fix in one agent on purpose: the review context
(explore + findings) is already loaded, so the fix is cheap — no re-explore, no cold
re-read. A fresh Handler per round, hydrated from the prior round's review/fixer-report
files, avoids both alternatives that today are suboptimal: (1) the original producer
fixing (token-heavy at long resume) and (2) a separate fixer thread (loses the why).

**No self-grading.** A is pure review and precedes B, so A is a legitimate gate
identical to today's reviewer. The fix from round N is judged by a fresh Handler's A in
round N+1. Termination on `APPROVED` is therefore always a clean review round — every
fix gets an independent confirmation before the block closes.

### Stuck detection

`stuck` is the other exit, and it is the hard part. Two mechanisms, both already present
in today's review setup:

- **Round cap (N).** Go's deterministic backstop — the loop always terminates.
- **Progress / circularity.** It is not just the *count* of blocking findings that
  matters but the *type*: are we going in circles? Oscillation can hold the count flat
  (fix A, break B; next round fix B, break A → count stays at 1, a perfect loop). So the
  judge must track finding **identity** across the whole history, not magnitude.

The progress check is the one part that does not become pure Go, for two reasons: it is
**semantic** (is finding A in round 3 the same underlying issue as finding B in round 1?
a naive set-diff is fooled by rewording), and it must be **independent of the Handler**
(else self-grading — a Handler is motivated to claim progress to avoid being declared
stuck). It does **not** need a standing orchestrator: it is a thin, ephemeral
**progress-judge** (a Haiku is enough — bounded compare-and-classify over short,
already-articulated findings) that Go spawns on demand.

- It spawns **conditionally**: only after a `BLOCKING` round *and* when there is a prior
  round to compare against (not round 1; not after `APPROVED`).
- Its input is **self-contained** — Go hands it the relevant rounds' reviews (or the
  canonical-key history); it carries no memory between calls.
- It is **fail-safe**: uncertain → default "progressing," and let the N-cap be the hard
  floor. A false "progress" costs a few bounded rounds; a false "stuck" is the costly
  error, so it must require clear evidence of circularity.

A sharper split: let the progress-judge **canonicalize** each round's findings into
stable keys (normalize "same issue" → same key), and let **Go** do the cycle detection
deterministically over the key history ("key X reappeared in rounds 1, 3, 5 → circling").
Judgment (are these the same issue) in the judge; cycle logic (does the key recur) in Go.

## `lyx review` — the generic reviewer module

One engine serves **all** review: discussion-review, plan-review, builder-review, and
`review anything` standalone are just different call-sites of the same module. This is
the lyx way — one one-shot module, reused, not duplicated.

The module **must be configurable on what it reviews**. The per-target configuration is
a **review profile** (discussion / plan / builder are three profiles; ad-hoc review is a
fourth). A profile carries:

| Field | Meaning |
|-------|---------|
| **target** | What to read — one file (`plan.md`), or for builder a git-diff against base + the working tree. Not always one file. |
| **against** (fasit) | The source of truth to check the target *against*. The plan is checked against `discussion.md`; the code against `plan.md`. **The easily-missed, most important field** — without it a review degenerates to a pure internal-consistency check, not fidelity to intent. The contract is `{fasit, target} → verdict`, not `target → verdict`. |
| **rubric** | What counts as `BLOCKING` for this target type. Plan rubric ("is the DAG sound, are batches independent, does it cover the discussion") ≠ code rubric ("correctness, tests green, no regression"). Data, not code. |
| **fix-scope** | What the fixer may write — a markdown file (`plan.md`) vs the source tree. |
| **tool-use** | Handler always (reviewing anything real means looking at the world, not just the artifact text). Cluster reviewers graded: builder wants tool-use; discussion can run bulk. |
| **cluster-N, round-cap** | Optional per profile. |

Three disciplines keep this **one** module and not three forks:

1. **The per-phase difference is the rubric, not the code.** Feed the rubric in; keep one
   engine. Forking the Handler per phase loses the point.
2. **The verdict contract is invariant** — `APPROVED | BLOCKING` + structured findings +
   fixer-report, regardless of what was reviewed. That invariance is exactly what lets Go
   drive all three phases identically. Vary the payload, never the control surface.
3. **Rubric and fasit are data.** You can tighten "what is a blocking plan flaw" by
   editing a rubric file and every plan-review picks it up — without touching the engine.
   The bar becomes versionable and tunable, separate from the machinery.

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

### State & contracts

- **The status file in `_lyx/` is the single source of truth** for orchestration state:
  current phase, current review block + round, and the verdict history the progress-judge
  needs. Nothing orchestration-relevant lives anywhere else.
- **Round-level resume.** Handler/fixer artifacts are already on disk, so resuming inside
  a review block continues at the current round rather than restarting the phase.
- **Separation of state.** `lyx review` owns its block's round state in the block's files;
  `lyx run`'s status only needs phase + the block's outcome. When `lyx review` returns
  `APPROVED | stuck`, `lyx run` advances.

## Module decomposition

| Piece | Form | Notes |
|-------|------|-------|
| `loom` (`lyx loom run`) | new Go module | the phase machine / autonomous driver |
| `review` (`lyx review`) | new Go module | the gate engine: Handler+fixer + optional cluster + progress-judge loop |
| producers (discussion / plan / builder) | prompt/profile files | **not** modules — just a prompt + profile fed to `agent.Run` |
| execution stack | existing/new infra | [`proc`](../overview.md#execution-stack-orchestration-layers) → [`mux`](mux.md) → [`shed`](shed.md) → [`agent`](agent.md) — built once, used by both modules above |
| Setup | uses existing modules | `worktree`, `weft`, `board` |
| `/ly-*` skills | thin wrappers | over `lyx loom run` |

The new Go specific to loom is the **two modules** (`loom`, `review`); everything beneath them
is the shared [execution stack](../overview.md#execution-stack-orchestration-layers) (`proc`,
`mux`, `shed`, `agent`), and everything else is prompt files, profiles, and the existing lyx
modules.

## Entry point

Today: launch `claude` in a terminal, then `/mill-start` — an interactive LLM session
drives everything. Loom inverts this: you launch a **Go process** (`lyx run`) that drives,
spawning each agent as an **interactive psmux session** ([Agent execution](#agent-execution))
and steering it unattended. A double-click wrapper script is convenience on top — it just
calls `lyx run`. Every agent runs interactively (subscription constraint); the only
difference is *who* is in the session — a human in Discussion, `lyx run` everywhere else.

## Agent execution

Every agent loom spawns — producers, the review handler, cluster reviewers, the
progress-judge — runs through the [`internal/agent`](agent.md) layer as an **interactive
psmux session, never headless `claude -p`** (an economic constraint; see
[agent.md](agent.md#interactive-never-headless--the-economic-constraint)). **I/O still rides
the file contract** — the agent writes its output files and Go reads them — so the
file-contract design above is unchanged; only the *spawn + completion-detection* mechanism
differs from a headless model.

The consequence for loom: it sits on top of the
[`proc → mux → shed → agent`](../overview.md#execution-stack-orchestration-layers) stack,
so that stack is on loom's critical path. loom calls `agent.Run` per spawn and stays
ignorant of panes, layout, and engines — those belong to [`shed`](shed.md) (placement,
focus, the **cluster window** where N reviewers go) and [`agent`](agent.md) (the swappable
provider engine). What loom owns is everything in this document: the phase machine, the
gate, stuck detection, and the status contract.

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
