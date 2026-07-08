# Module: hardener (DRAFT — concept not yet settled)

> **⚠️ DRAFT. This is an early concept sketch, not a settled design.** Unlike the other module docs
> (which are *Design — not built* but agreed), `hardener` is still being figured out. The shape below
> captures what a weekend of hand-running the method taught us and the decisions reached so far in
> discussion; **expect fields, mechanisms, and even the boundary of the module to change.** Do not
> implement from this doc yet. When the concept firms up this banner comes off and the doc becomes a
> normal *Design — not built* entry; per the [documentation lifecycle](../overview.md#documentation-lifecycle)
> it is eventually deleted into the package header + `overview.md` on landing.
>
> **Hand-executed origin:** [`docs/reviews/`](../reviews/README.md) is the method this module would
> automate. `hardener` was developed over the last week out of a concrete need: to **run** actual
> `mux` code hard enough to surface defects a green `go test` never proves. Six hand-orchestrated
> rounds fixed what many rounds of text-based review could not — it was genuinely *hardening*.

`hardener` is a **behavior-based reviewer**: where [`perch`](perch.md) reads an artifact, hardener
**runs** a live-substrate module, reacts to what it observes, and builds bespoke adversarial
scenarios to break it. It is a separate, on-demand, **post-loom** module — not on the
`shuttle → burler → perch → loom` spine — meant to harden a live-substrate module (the archetype:
`mux` driving real psmux) before merge.

## Why it is not `perch`

`perch` and `hardener` share the `burler` *round discipline* (see the `internal/burlerengine`
package documentation) — A-review → B-fix, no self-grading, commit-per-fix, fix-everything — but
they are different reviewers along two axes:

| | [`perch`](perch.md) | `hardener` |
|---|---|---|
| Mode | **text** — read the artifact | **behavior** — run the module, react, build scenarios |
| Substrate | in-worktree, fast | a **live sandbox repo**; slow, heavy git/go operations |
| Gate | LLM verdict (or a light `command` gate) | **deterministic**: run the smoke suite N× concurrent, zero stray state |
| Orchestrator | stateless Go loop | an **accumulating** orchestrator (see below) |
| Cost | cheap, minutes | token- **and** wall-clock-heavy; a single iteration ran 1–2 hours; a campaign, a weekend |
| When | between every phase (on the spine) | **on demand, after loom**, only when needed |

perch's `command` gate lets a code profile *touch* behavior lightly; hardener is the heavy tier —
driving real substrate and hand-rolling crash/rebirth/concurrency scenarios is its whole job.

## The orchestrator — persistent thread, or per-round respawn + handoff?

In the hand-run version, **one persistent orchestrator thread** stayed alive across the campaign: it
spawned a fresh round agent per round, **independently verified** the round's work (re-ran the gates
from cold state on the committed tree — never trusting the round's own "merge-ready" verdict),
**accumulated** an understanding of where the module's bugs live, **targeted** each next round agent
("focus on X"), maintained a **handoff** that survived compaction, and asked the operator what to do
next. The targeting + accumulation is what made 6 rounds succeed where stateless text-review rounds
did not — the round agent kept re-discovering the terrain cold otherwise.

**Key insight (still being validated):** if the orchestrator **compacts to its handoff after each
round**, that is functionally the same as **respawning a fresh orchestrator per round that reads
{instructions + handoff}**. The handoff *is* the accumulated state, externalized. So the "persistent
thread" may be an artifact of how it was hand-run, not an essential requirement — hardener's
orchestrator could be a per-round respawn, which is cheaper (no unbounded context growth),
crash-safe (handoff on disk), and aligned with lyx's Go-drives-fresh-agents model. This is the same
shape as perch/burler's "fresh agent per round, hydrated from prior files" — the difference is the
heavier round and the accumulating (not just summarizing) handoff.

### The handoff — two-tier memory, and the one crux

- **Handoff** — a **distilled summary**, **edited in place each round** (not appended, so per-round
  context stays bounded regardless of round count). Always read. The orchestrator's compressed
  understanding + what to target next. *(The hand-run instructions already rewrite it in place per
  round — this discipline is proven, not new.)*
- **Raw reviews + fixer-reports** — complete, authoritative, **read on demand** when the handoff
  points at something needing detail. Reviews inform *targeting*; fixer-reports inform *verification*.

The read-set is *instructions + handoff (always) + selective raw files (on demand)* — **not** "all
reviews every round," which would reintroduce the O(N) context growth the handoff exists to kill.

**The one crux for going stateless (per-round respawn):** in the hand-run version a *live* thread
held some things implicitly — above all "this finding has recurred before." A respawn has no live
memory, so **anything the thread knew implicitly must become explicit in the handoff.** The prime
suspect is the **finding-identity / recurrence ledger** (which findings reappeared in which rounds):
if an in-place edit "cleans up" a finding that looks resolved, the recurrence trail can be silently
lost and stuck-detection fails quietly. So: **distill the prose, but keep the key-ledger lossless.**
The migration persistent-thread → respawn is essentially this one audit — find what the live thread
knew implicitly and make it explicit.

## Autonomy

The operator's role in the hand-run campaign was mostly **gating** — approve, ask for another round —
not irreplaceable judgment. That is front-loadable into the seed instructions ("run until the gates
are green or K rounds; do not ask"). So hardener can run **autonomously, overnight**, with mux + Go
handling **auto-compaction** (which, per the insight above, *is* per-round respawn). Model rotation
across rounds (Opus / Fable / Sonnet) stays as a cheap diversity lens — convergence across *different*
models is stronger evidence than N passes from one.

## The sandbox dependency

Hardener cannot run against the module's own repo alone — it needs a **live sandbox repo** to do
destructive, stateful things (create worktrees, junctions, spawn psmux, tear down) without corrupting
the real repo, and a maintained **live-driving suite** (`tools/sandbox/SANDBOX-<MODULE>-SUITE.md`) as
the substrate-exercising vehicle. Consequences carried from the hand-run method:

- **Deploy-first.** The suite runs the **deployed** binary, not the working tree — re-deploy after
  every source change or you validate a stale binary.
- **The decisive gate is N× concurrent smoke, not a quiet serial pass** — concurrency + CPU
  saturation is the amplifier that surfaces teardown races and leaked substrate state.
- **Zero stray substrate state at teardown** is itself an invariant under test.
- **Grow the suite with the module** — a bug found live leaves behind both a `//go:build smoke`
  regression test and, where visual, a suite scenario.

## The likely `lyx` shape (open)

Hardener's defining trait — an accumulating, targeting orchestrator — is precisely what lyx's thesis
otherwise *replaces* with Go. So "hardener as a module" probably is **not** "Go takes over the
orchestrator." More likely: **lyx provides the deterministic scaffolding** and the **orchestrator
brain stays an LLM** —

- **Go / lyx owns:** provision/reset the sandbox, deploy the binary, run the slow gates (smoke suite,
  zero-stray-state), collect + structure results, maintain handoff files (via `internal/state`), the
  per-round respawn loop, teardown.
- **LLM orch-brain owns:** read results, accumulate understanding, decide what to target next, write
  the next round's focused prompt.
- **`burler`-shaped round agent** (see the `internal/burlerengine` package documentation): the A→B
  worker, spawned per round (drives the sandbox; `fix-scope: source`; commit-per-fix).

Whether the round agent literally imports the `burler` package or only follows the same
`review-prompt-template.md` discipline is an implementation choice for when this is built.

## Dependencies (tentative)

- `shuttle` — spawns the orchestrator strand, the round agents, and any judges.
- [`internal/stencil`](../shared-libs/stencil.md) — fills the round-agent / orchestrator prompt
  templates (shared with `burler`/`perch`).
- `internal/state` — handoff + round artifacts on disk (the memory that makes respawn work).
- a **sandbox repo + live suite** — a provisioned environment and a maintained asset, not just code.
- `mux` transitively, via shuttle; possibly directly for the overnight/autonomous session + auto-compaction.

## Status / open questions

- Persistent thread vs. per-round respawn — leaning respawn, pending the recurrence-ledger audit.
- Exactly what the handoff must carry losslessly (key-ledger confirmed; what else?).
- The Go-scaffolding / LLM-brain boundary above.
- Whether it reuses the `burler` package or just the prompt template.
- Sandbox provisioning: how much lyx automates vs. a pre-existing sandbox repo.

**This module is post-loom and on-demand; nothing here blocks the `burler → perch → loom` spine.**
