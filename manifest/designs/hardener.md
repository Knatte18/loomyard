# Module: hardener (DRAFT — concept not yet settled)

> **⚠️ DRAFT. This is an early concept sketch, not a settled design.** Unlike the other module docs
> (which are *Design — not built* but agreed), `hardener` is still being figured out. The shape below
> captures what a weekend of hand-running the method taught us and the decisions reached so far in
> discussion; **expect fields, mechanisms, and even the boundary of the module to change.** Do not
> implement from this doc yet. When the concept firms up this banner comes off and the doc becomes a
> normal *Design — not built* entry; per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle)
> it is eventually deleted into the package header + `overview.md` on landing.
>
> **Hand-executed origin:** [`crucible`](../../crucible/README.md) is the method this module would
> automate — the hand-run version of the same idea, named separately to avoid colliding with this
> module's own name. `hardener` was developed over the last week out of a concrete need: to **run**
> actual `reed` code hard enough to surface defects a green `go test` never proves. Six
> hand-orchestrated rounds fixed what many rounds of text-based review could not — it was
> genuinely *hardening*.

`hardener` is a **behavior-based reviewer**: where `perch` (see the `internal/perchengine` package
documentation) reads an artifact, hardener
**runs** a live-substrate module, reacts to what it observes, and builds bespoke adversarial
scenarios to break it. It is a separate, on-demand, **post-loom** module — not on the
`shuttle → burler → perch → loom` spine — meant to harden a live-substrate module (the archetype:
`reed` driving real tmux) before merge.

## Why it is not `perch`

`perch` and `hardener` share the `burler` *round discipline* (see the `internal/burlerengine`
package documentation) — A-review → B-fix, no self-grading, commit-per-fix, fix-everything — but
they are different reviewers along two axes:

| | `perch` | `hardener` |
|---|---|---|
| Mode | **text** — read the artifact | **behavior** — run the module, react, build scenarios |
| Substrate | in-worktree, fast | a **live sandbox repo**; slow, heavy git/go operations |
| Gate | LLM verdict (or a light `command` gate) | **deterministic**: run the smoke suite N× concurrent, zero stray state |
| Orchestrator | stateless Go loop | an **accumulating** orchestrator (see below) |
| Cost | cheap, minutes | token- **and** wall-clock-heavy; a single iteration ran 1–2 hours; a campaign, a weekend |
| When | between every phase (on the spine) | **on demand, after loom**, only when needed |

perch's `command` gate lets a code profile *touch* behavior lightly; hardener is the heavy tier —
driving real substrate and hand-rolling crash/rebirth/concurrency scenarios is its whole job.

## The orchestrator — resolved: Go drives per-round respawn, no persistent thread

In the hand-run version (see [crucible/README.md](../../crucible/README.md)), **one persistent
orchestrator thread** stayed alive across the campaign: it spawned a fresh round agent per round,
**independently verified** the round's work (re-ran the gates from cold state on the committed
tree — never trusting the round's own "merge-ready" verdict), **accumulated** an understanding of
where the module's bugs live, **targeted** each next round agent ("focus on X"), maintained a
**handoff** that survived compaction, and asked the operator what to do next. The targeting +
accumulation is what made the reed campaign's rounds succeed where stateless text-review rounds did
not — the round agent kept re-discovering the terrain cold otherwise.

**Resolved: no persistent thread.** Go itself is the orchestrator (`Gorch`); each round is three
fresh, one-shot spawns, not a living session accumulating state in a context window:

1. **`progress-judge`, pre-round.** Gorch spawns a fresh one-shot agent that reads the handoff file,
   decides what to target next, and writes the round's seed prompt (the file the reviewer will
   read). It then terminates — no lingering session.
2. **Reviewer.** Gorch spawns the round agent proper, pointed at the prompt the judge just wrote —
   the same A-review → B-fix, commit-per-fix, no-self-grading round crucible already runs by hand.
   This is the `burler`-shaped worker; the *campaign* loop wrapping it is new, the round itself is
   not.
3. **`progress-judge`, post-round.** Gorch spawns a fresh one-shot agent that reads the handoff plus
   the round's review/fixer-report artifacts, **independently validates the findings** (crucible's
   non-negotiable rule — three of the reed campaign's seven rounds self-reported "merge-ready" and
   were wrong each time), rewrites the handoff in place, and decides whether another round is needed
   or the campaign has converged.

**Naming, fixed:** the pre/post role above is **`progress-judge`**, reusing the term perch's own
module description already uses ("run `burler` rounds → `APPROVED`/`stuck` + `progress-judge` +
cap") — not "handler." "Handler" already names a different thing in loom's existing vocabulary (the
A-review→B-fix round worker itself, i.e. the "Reviewer" above) — reusing it for the
targeting/validating role would collide with that.

The handoff file is the sole accumulation vehicle; there is no live memory anywhere in the loop. The
one cost this pays versus perch today: a `progress-judge` spawn on **both** sides of the reviewer,
not just post-round — the pre-round targeting step (read handoff, decide focus, write the next
round's seed prompt) is not something perch's `progress-judge` does for Discussion/Plan/Builder
today; those rounds reuse a fixed rubric, not a dynamically retargeted prompt.

### Open question: does pre-round targeting belong in `perch` itself?

Not yet decided — two shapes:

- **Generalize `perch`** so its `progress-judge` gains a pre-round targeting job (read state, decide
  focus, write the next round's seed) for every caller, not just hardener — Discussion/Plan/Builder
  would then also get per-round retargeting instead of a fixed rubric replayed unchanged each round.
- **Keep it hardener-specific** — a wrapper around perch's existing (post-round-only)
  `progress-judge` that bolts a new pre-round half on top, without changing perch's contract for its
  other callers.

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
are green or K rounds; do not ask"). So hardener can run **autonomously, overnight**, with reed + Go
handling **auto-compaction** (which, per the insight above, *is* per-round respawn). Model rotation
across rounds (Opus / Fable / Sonnet) stays as a cheap diversity lens — convergence across *different*
models is stronger evidence than N passes from one.

## The sandbox dependency

Hardener cannot run against the module's own repo alone — it needs a **live sandbox repo** to do
destructive, stateful things (create worktrees, junctions, spawn tmux, tear down) without corrupting
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
- [`internal/stencil`](../../docs/shared-libs/stencil.md) — fills the round-agent / orchestrator prompt
  templates (shared with `burler`/`perch`).
- `internal/state` — handoff + round artifacts on disk (the memory that makes respawn work).
- a **sandbox repo + live suite** — a provisioned environment and a maintained asset, not just code.
- `reed` transitively, via shuttle; possibly directly for the overnight/autonomous session + auto-compaction.

## Status / open questions

- ~~Persistent thread vs. per-round respawn~~ — resolved: per-round respawn via `Gorch`'s
  three-step loop (pre-round `progress-judge` → reviewer → post-round `progress-judge`); see above.
- Whether pre-round targeting generalizes into `perch` itself or stays hardener-specific (see
  above) — not yet decided.
- Exactly what the handoff must carry losslessly (key-ledger confirmed; what else?).
- The Go-scaffolding / LLM-brain boundary above.
- Whether it reuses the `burler` package or just the prompt template.
- Sandbox provisioning: how much lyx automates vs. a pre-existing sandbox repo.

**This module is post-loom and on-demand; nothing here blocks the `burler → perch → loom` spine.**
