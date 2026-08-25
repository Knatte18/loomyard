# Module: Tenter + Hardener (DRAFT — concept not yet settled)

> **⚠️ DRAFT. This is an early concept sketch, not a settled design.** Unlike the other module docs (which are *Design — not built* but agreed), this one is still being figured out. The shape below captures what a weekend of hand-running the method taught us and the decisions reached so far in discussion; **expect fields, mechanisms, and even the boundary of the module to change.** Do not implement from this doc yet. When the concept firms up this banner comes off and the doc becomes a normal *Design — not built* entry; per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle) it is eventually deleted into the package header + `overview.md` on landing.
>
> **Hand-executed origin:** [`crucible`](../../crucible/README.md) is the method this module would automate — the hand-run version of the same idea, named separately to avoid colliding with this module's own name. This was developed over the last week out of a concrete need: to **run** actual `reed` code hard enough to surface defects a green `go test` never proves. Six hand-orchestrated rounds fixed what many rounds of text-based review could not — it was genuinely *hardening*.
>
> **Status: Someday, deprioritized.** Not required to get `loom` running — `loom` only ever uses text-review (its `Bouncer`+`Burler` review segments), never behavior-review. Kept separate from the Planned `Treadle`/`Shed` work for exactly that reason: nothing here blocks `loom`.

## Naming: two things, not one

Two distinct layers, split out once the shared-engine design (the `internal/treadleengine` package documentation, [shed.md](shed.md)) was pinned:

- **`Tenter`** — the review-loop alone: `Treadle` (the generic round-loop engine — judge, gate, round-spawn, cap, pause, lock) configured with a live-substrate-driving round-runner and a behavior-review profile, instead of a `burlerengine` round + text-review profile.
  The direct structural sibling of loom's own text-review segments.
  Not separately runnable in isolation the simple way a text-review segment is, because behavior-review needs a live sandbox/worktree lifecycle around it.
- **`Hardener`** — the full, on-demand, autonomous campaign: `Shed` (the generic outer phase-FSM — see [shed.md](shed.md)) wrapping `Tenter`, plus Hardener's own Preflight (sandbox provisioning, live-suite readiness).
  This is what gets a worktree spawned for it (via `fabric`) and safe-merges back into parent when done, the same lifecycle `loom` uses, just with `Hardener`'s own producer list carrying `Tenter` where `loom`'s list carries its Discussion, Plan and Webster producers.

`Tenter` is a **behavior-based reviewer**: where a text-review segment reads an artifact, `Tenter` **runs** a live-substrate module, reacts to what it observes, and builds bespoke adversarial scenarios to break it. `Hardener` is a separate, on-demand, **post-loom** campaign — not on the `shuttle → burler → shed → loom` spine — meant to harden a live-substrate module (the archetype: `reed` driving real tmux) before merge.

## Why `Tenter` is not a text-review segment

`Tenter` and a text-review segment share the `burler` *round discipline* (see the `internal/burlerengine` package documentation) — A-review → B-fix, no self-grading, commit-per-fix, fix-everything — but they are different reviewers along two axes:

| | text-review segment | `Tenter` |
|---|---|---|
| Mode | **text** — read the artifact | **behavior** — run the module, react, build scenarios |
| Substrate | in-worktree, fast | a **live sandbox repo**; slow, heavy git/go operations |
| Gate | LLM verdict (or a light `command` gate) | **deterministic**: run the smoke suite N× concurrent, zero stray state |
| Cost | cheap, minutes | token- **and** wall-clock-heavy; a single iteration ran 1–2 hours; a campaign, a weekend |
| When | between every phase (on the spine) | **on demand, after loom**, only when `Hardener` is invoked |

A text-review segment's `command` gate lets a code profile *touch* behavior lightly;
`Tenter` is the heavy tier — driving real substrate and hand-rolling crash/rebirth/concurrency scenarios is its whole job.

## `Tenter`'s round-loop — resolved: `Treadle` drives per-round respawn, no persistent thread

**The engine underneath `Tenter` is a general one, not Tenter-specific — see the `internal/treadleengine` package documentation.**
That doc covers the round-runner interface, the judge-maintained handoff (a general improvement, not just Tenter's),
and the process for getting there (`Treadle` is Planned;
`Tenter` itself stays Someday, built on `Treadle` only once `Treadle` exists).
What follows here is Tenter's own instance of that shared design.

In the hand-run version (see [crucible/README.md](../../crucible/README.md)), **one persistent orchestrator thread** stayed alive across the campaign: it spawned a fresh round agent per round, **independently verified** the round's work (re-ran the gates from cold state on the committed tree — never trusting the round's own "merge-ready" verdict), **accumulated** an understanding of where the module's bugs live, **targeted** each next round agent ("focus on X"), maintained a **handoff** that survived compaction, and asked the operator what to do next.
The targeting + accumulation is what made the reed campaign's rounds succeed where stateless text-review rounds did not — the round agent kept re-discovering the terrain cold otherwise.

**Resolved: no persistent thread.**
Go itself is the orchestrator (`Treadle`);
each round is three fresh, one-shot spawns, not a living session accumulating state in a context window:

1. **`progress-judge`, pre-round.** `Treadle` spawns a fresh one-shot agent that reads the handoff file, decides what to target next, and writes the round's seed prompt (the file the reviewer will read).
   It then terminates — no lingering session.
2. **Reviewer.** `Treadle` spawns the round agent proper, pointed at the prompt the judge just wrote — the same A-review → B-fix, commit-per-fix, no-self-grading round crucible already runs by hand.
   This is the `burler`-shaped worker;
   the *campaign* loop wrapping it is new, the round itself is not.
3. **`progress-judge`, post-round.** `Treadle` spawns a fresh one-shot agent that reads the handoff plus the round's review/fixer-report artifacts, **independently validates the findings** (crucible's non-negotiable rule — three of the reed campaign's seven rounds self-reported "merge-ready" and were wrong each time), rewrites the handoff in place, and decides whether another round is needed or the campaign has converged.

**Naming, fixed:** the pre/post role above is **`progress-judge`**, reusing the term the text-review gate's own description already uses ("run `burler` rounds → `APPROVED`/`stuck` + `progress-judge` + cap") — not "handler."
"Handler" already names a different thing in loom's existing vocabulary (the A-review→B-fix round worker itself, i.e. the "Reviewer" above) — reusing it for the targeting/validating role would collide with that.

The handoff file is the sole accumulation vehicle;
there is no live memory anywhere in the loop.
The one cost this pays versus a text-review segment today: a `progress-judge` spawn on **both** sides of the reviewer, not just post-round — a text-review segment's judge doesn't do this pre-round targeting for Discussion/Plan/Webster today;
those rounds reuse a fixed rubric, not a dynamically retargeted prompt.

### Pre-round targeting

Superseded by the `internal/treadleengine` package documentation — pre-round targeting is designed there as a general capability `Treadle`'s judge can support, exercised by Tenter's profile and simply unused by a text-review profile. See that doc's "Pre-round targeting" section instead of resolving this here.

### The handoff — two-tier memory, and the one crux

- **Handoff** — a **distilled summary**, **edited in place each round** (not appended, so per-round context stays bounded regardless of round count).
  Always read.
  The orchestrator's compressed understanding + what to target next. *(The hand-run instructions already rewrite it in place per round — this discipline is proven, not new.)*
- **Raw reviews + fixer-reports** — complete, authoritative, **read on demand** when the handoff points at something needing detail.
  Reviews inform *targeting*;
  fixer-reports inform *verification*.

The read-set is *instructions + handoff (always) + selective raw files (on demand)* — **not** "all reviews every round," which would reintroduce the O(N) context growth the handoff exists to kill.

**The one crux for going stateless (per-round respawn):** in the hand-run version a *live* thread held some things implicitly — above all "this finding has recurred before."
A respawn has no live memory, so **anything the thread knew implicitly must become explicit in the handoff.**
The prime suspect is the **finding-identity / recurrence ledger** (which findings reappeared in which rounds): if an in-place edit "cleans up" a finding that looks resolved, the recurrence trail can be silently lost and stuck-detection fails quietly.
So: **distill the prose, but keep the key-ledger lossless.**
The migration persistent-thread → respawn is essentially this one audit — find what the live thread knew implicitly and make it explicit.

### Reuse: only `Bouncer` carries over

`Tenter`'s own review-loop is expected to land as a `Shed` segment: a round producer (this module's own equivalent of the shipped `shedadapters: Burler-round producer`, wrapping whatever `Tenter`'s round mechanism turns out to be — not `burlerengine`) paired with an instance of the shipped `Bouncer: the generic review-gate producer`, the same hand-wired shape `loom`'s own review producers use (folk name "perch," see `CLAUDE.md`'s terminology note — this segment is "a perch" in that same loose sense, same shape, different round producer inside it).
Only `Bouncer` is literally reusable code here: `burlerengine`'s own round mechanism is inherently text/diff-specific (Target/Fasit/Rubric over a `shuttle` session editing text), so `Tenter`'s round producer needs its own from-scratch implementation of the same always-`Stuck`-until-approved contract, not a port of `burlerengine`.
This is the second data point (after `loom`'s own review producers) for whether `internal/treadleengine` is ever fully retired — see the shipped `Retire perch` item's own note that its retirement is a separate call, made once `Tenter` lands.

## `Hardener` — the campaign

The operator's role in the hand-run campaign was mostly **gating** — approve, ask for another round — not irreplaceable judgment.
That is front-loadable into the seed instructions ("run until the gates are green or K rounds;
do not ask").
So `Hardener` can run **autonomously, overnight**, with reed + Go handling **auto-compaction** (which, per the insight above, *is* per-round respawn).
Model rotation across rounds (Opus / Fable / Sonnet) stays as a cheap diversity lens — convergence across *different* models is stronger evidence than N passes from one.

`Hardener`'s own worktree-spawn (via `fabric`) and safe-merge-back-to-parent lifecycle is `Shed`'s job (see [shed.md](shed.md)) — the same `loom`-shared lifecycle described above, with Hardener's own Preflight (below) instead of loom's.

### The sandbox dependency

`Hardener` cannot run against the module's own repo alone — it needs a **live sandbox repo** to do destructive, stateful things (create worktrees, junctions, spawn tmux, tear down) without corrupting the real repo,
and a maintained **live-driving suite** (`tools/sandbox/SANDBOX-<MODULE>-SUITE.md`) as the substrate-exercising vehicle.
Consequences carried from the hand-run method:

- **Deploy-first.**
  The suite runs the **deployed** binary, not the working tree — re-deploy after every source change or you validate a stale binary.
- **The decisive gate is N× concurrent smoke, not a quiet serial pass** — concurrency + CPU saturation is the amplifier that surfaces teardown races and leaked substrate state.
- **Zero stray substrate state at teardown** is itself an invariant under test.
- **Grow the suite with the module** — a bug found live leaves behind both a `//go:build smoke` regression test and, where visual, a suite scenario.

## The likely `lyx` shape (open)

`Tenter`'s defining trait — an accumulating, targeting review-loop — is precisely what lyx's thesis otherwise *replaces* with Go.
So this probably is **not** "Go takes over the orchestrator."
More likely: **lyx provides the deterministic scaffolding** and the **orchestrator brain stays an LLM** —

- **Go / lyx owns** (split across `Shed` and `Treadle`): provision/reset the sandbox, deploy the binary, run the slow gates (smoke suite, zero-stray-state), collect + structure results, maintain handoff files (via `internal/state`), the per-round respawn loop, teardown.
- **LLM orch-brain owns** (Tenter's judge, within `Treadle`): read results, accumulate understanding, decide what to target next, write the next round's focused prompt.
- **`burler`-shaped round agent** (see the `internal/burlerengine` package documentation): the A→B worker, spawned per round (drives the sandbox;
  `fix-scope: source`;
  commit-per-fix).

Whether the round agent literally imports the `burler` package or only follows the same `review-prompt-template.md` discipline is an implementation choice for when this is built.

## Dependencies (tentative)

- `internal/treadleengine` package documentation — the generic round-loop engine `Tenter` would configure (as-built;
  module doc deleted per the documentation lifecycle), independent of whether `Tenter`/`Hardener` ever get built.
- [`shed.md`](shed.md) — the generic outer phase-FSM `Hardener` configures;
  Planned, same independence.
- `shuttle` — spawns the round agents and judges `Treadle`/`Tenter` drive.
- [`internal/stencil`](../../docs/shared-libs/stencil.md) — fills the round-agent / orchestrator prompt templates (shared with `burler` and the review gate).
- `internal/state` — handoff + round artifacts on disk (the memory that makes respawn work).
- a **sandbox repo + live suite** — a provisioned environment and a maintained asset, not just code.
- `reed` transitively, via shuttle;
  possibly directly for the overnight/autonomous session + auto-compaction.

## Status / open questions

- ~~Persistent thread vs. per-round respawn~~ — resolved: per-round respawn via `Treadle`'s three-step loop (pre-round `progress-judge` → reviewer → post-round `progress-judge`);
  see above.
- ~~Naming: one module or two?~~ — resolved: `Tenter` (review-loop) + `Hardener` (`Shed` + `Tenter`, the campaign);
  see "Naming" above.
- The shared engine designs (round-runner interface, handoff/ledger format, pre-round-targeting mechanics for `Treadle`;
  the outer FSM for `Shed`) live in their own docs — not decided here, and both are Planned independently of whether `Tenter`/`Hardener` themselves ever get scheduled.
- Exactly what the handoff must carry losslessly (key-ledger confirmed;
  what else?).
- The Go-scaffolding / LLM-brain boundary above.
- Whether it reuses the `burler` package or just the prompt template.
- Sandbox provisioning: how much lyx automates vs. a pre-existing sandbox repo.

**This module is post-loom and on-demand;
nothing here blocks the `burler → shed → loom` spine, nor the Planned `Treadle`/`Shed` work, which proceeds independently of whether `Tenter`/`Hardener` are ever scheduled.**
