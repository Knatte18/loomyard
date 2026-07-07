# Module: burler (design)

> **Status: Design — not built.** Per the [documentation lifecycle](../overview.md#documentation-lifecycle),
> this file is deleted when the module lands and its durable parts fold into the package header
> and `overview.md`. It was split out of the former `review.md` together with [perch.md](perch.md):
> **`burler` owns one review+fix round; [`perch`](perch.md) owns the loop that runs burler rounds
> until an artifact is approved or stuck.**
>
> **Hand-executed prototype:** [`docs/reviews/`](../reviews/README.md) documents the manual,
> human-in-the-loop form of this exact round agent (A-review → B-fix, fresh agent per round,
> no self-grading) — the method used to harden `mux` over seven rounds before merge.

`burler` is the **round worker** — the agent that performs **one** review+fix pass over an artifact
and returns a verdict. It is named for *burling and mending*: the cloth-finishing step where a worker
inspects woven fabric for defects **and** repairs them in one pass. That is exactly what a burler
does — **A: review** (find the defects), then **B: fix** (repair them) — in a single agent.

A burler runs **one round and exits.** It knows nothing about round loops, caps, convergence, or
progress across rounds — those belong to [`perch`](perch.md), which composes burler. This split is
deliberate: it puts *all* the LLM-heavy work in burler (testable on its own, standalone) and leaves
perch a thin, deterministic Go loop (testable with a fake burler). See
[Why a separate module](#why-a-separate-module-from-perch).

## The A/B round

A burler does two jobs, in order, in one agent:

1. **A — Review.** The burler reviews the artifact like a normal reviewer, **not yet knowing it will
   also fix.** It reads the target (and drives the real substrate if the profile calls for it), forms
   its own findings, and writes a review to file with a verdict: `BLOCKING` or `APPROVED`. In step A
   it **may** spawn N extra [cluster reviewers](#cluster-support), wait for them (or time out), and
   fold their findings into a cross-checked review.
2. **B — Fix.** The burler then fixes what it found, itself, from its own review plus its own
   reasoning — **even if the verdict was `APPROVED`** (non-blocking polish). It writes a
   fixer-report.

**A before B is a hard gate, not advisory.** Job A must be fully written to the review file on disk
*before* the burler touches any target file. Fixing findings as it spots them turns the "review"
into a post-hoc rationalization of edits already made, which destroys the independent judgment that
is the whole point. Every burler prompt states this explicitly (the "Sequencing rule" in
[`review-prompt-template.md`](../reviews/review-prompt-template.md)).

**Why review and fix in one agent.** The review context (explore + findings) is already loaded, so
the fix is cheap — no re-explore, no cold re-read. A **fresh burler per round**, hydrated from the
prior round's review/fixer-report files, avoids both of mill's suboptimal alternatives: (1) the
original producer fixing (token-heavy at long resume) and (2) a separate fixer agent (loses the
*why*).

**No self-grading — the discipline lives in [`perch`](perch.md).** A single burler never grades its
own fix. A is pure review and precedes B, so A is a legitimate gate identical to a normal reviewer.
The fix from round N is judged by a **fresh** burler's A in round N+1. That cross-round independence
is enforced by perch (it spawns a new burler each round); burler itself only guarantees A-before-B
within its own round.

### Two round disciplines carried from the hand-run method

- **Commit per fix, not one commit per round.** A burler commits each individual fix as it lands
  (message identifying the finding it closes), green build/vet/test first — but **never pushes**. If
  the session dies mid-round, `git log` shows exactly which findings are done and anything without a
  commit is unambiguously not done — no reverse-engineering a raw diff. (This bit the shuttle
  campaign for real; see [`docs/reviews/`](../reviews/README.md).) Applies to `fix-scope: source`
  profiles; markdown-scope profiles commit once.
- **Fix every finding, including NITs.** Severity affects how a finding is *reported*, not whether
  it gets *fixed*. Leaving NIT/LOW findings unfixed made round count *go up* in practice — unfixed
  nits re-surface or silently vanish across rounds instead of ever closing. A burler fixes
  everything it records in the same round.

## Cluster support

Cluster reviewers are **first-class but default off** (`cluster-N: 0`). `cluster-N` controls only the
*extra* cross-checkers spawned inside step A:

- **`cluster-N = 0`** → only the burler's own A-review. You still get one full review. This is the
  default and the common case.
- **`cluster-N > 0`** → the burler spawns N cluster reviewers, waits (or times out), aggregates their
  findings into one cross-checked review, then fixes.

Cluster belongs to the *round* (hence to burler), not to the loop. The N reviewers ask for
`display:{anchor:own-window}` (see the `internal/muxengine` package documentation) so they land in a
separate, switchable psmux window rather than crowding the worktree column.

**`cluster-N > 0` is gated on mux window support, which does not exist yet.** Own-window anchoring —
spawning a strand into its own switchable psmux *window* rather than a pane in the worktree column —
is an unbuilt mux capability (a deferred mux enhancement; see
[roadmap milestone](../roadmap.md#deferred-mux-enhancements)). Until it lands, `cluster-N = 0` (the
default) is the only supported mode: the `display:{anchor:own-window}` field above is a forward
reference, and burler must fail loudly rather than silently crowd the worktree column if asked for
`N > 0` before mux can place the windows. This gates only the cluster feature — **burler itself, and
the whole `shuttle → burler → perch → loom` spine, ship on `N = 0` without waiting on mux windows.**

## The profile burler consumes

A burler is driven entirely by a **profile** handed to it at spawn — it stores no phase knowledge of
its own. (Who supplies the profile: [`loom`](loom.md) per phase, or the caller for ad-hoc runs — see
[perch's config section](perch.md#configuration--where-profiles-live).)

| Field | Meaning |
|-------|---------|
| **target** | What to read — one file (`plan.md`), or for a code round a git-diff against base + the working tree. Not always one file. |
| **against** (fasit) | The source of truth to check the target *against*. **The easily-missed, most important field** — without it a review degenerates to a pure internal-consistency check, not fidelity to intent. The contract is `{fasit, target} → verdict`, not `target → verdict`. |
| **rubric** | What counts as `BLOCKING` for this target type. Data (a markdown asset), not code. |
| **fix-scope** | What the burler may write — a markdown file vs the source tree (also gates commit-per-fix). |
| **tool-use** | Whether the burler (and its cluster reviewers) drive the real substrate or read only. A text round reads; a code round runs. |
| **cluster-N** | How many cross-checkers to spawn in step A. Default 0. |

## What a burler returns

An invariant contract, regardless of what was reviewed:

- a **verdict** — `APPROVED | BLOCKING`,
- a **review file** — structured findings (with canonical-ish identity, so [`perch`](perch.md) can
  track recurrence across rounds),
- a **fixer-report** — what B changed.

That invariance is what lets `perch` drive round after round identically, and what lets `shuttle`'s
engine swap the provider without touching burler. Vary the payload (profile), never the control
surface (verdict contract).

## Why a separate module (from perch)

The review design is genuinely two things, and separating them is what makes it testable:

- **burler is LLM-heavy** — one round is a `shuttle` run (or several, with cluster). Its tests are a
  handful of **opt-in smoke tests against a real engine** (build-tagged, not in CI), plus unit tests
  with a **fake shuttle** returning scripted output files. It stands alone: `{profile, prior-round
  files} → {verdict, review, fixer-report}` is a self-contained contract.
- **[`perch`](perch.md) is deterministic Go** — the loop, the cap, cycle detection. It tests against
  a **fake burler** returning scripted verdicts, with no LLM at all.

Keeping them one module would blend the two test regimes. The dependency runs one way —
`perch → burler → shuttle` — a strict chain, same invariant as the rest of the stack ("each layer
knows only the one below").

## Standalone vs. composed

- **Composed (the product surface):** [`perch`](perch.md) spawns a fresh burler each round.
  LoomYard never calls a burler directly — it always goes through perch, so the no-self-grading
  cross-round discipline is always in force.
- **Standalone (debug only):** a single `burler` run is useful in isolation for developing the round
  agent, but is not a product verb. A debug CLI is optional and deferred; there is no `loom`
  call-site for a bare burler.

## Shared with `hardener`

The burler *discipline* — A-before-B as a hard gate, no-fix-before-review, commit-per-fix,
fix-everything — is the same round-agent shape the [`hardener`](hardener.md) module reuses for its
behavior-based hardening campaigns. Whether hardener imports the burler package or only follows the
same prompt template (`review-prompt-template.md`) is an implementation choice for when hardener is
built; the *contract* (A→B, no self-grade) is shared either way.

## Dependencies

- `shuttle` (see the `internal/shuttleengine` package documentation) — spawns the burler agent and
  the N cluster reviewers (each is one shuttle run through an engine). Tool-freedom (bulk vs
  tool-use) is a **generic tools-restriction on the shuttle spec** that burler sets and shuttle
  enforces at launch — shuttle never learns the word "bulk".
- [`internal/stencil`](../shared-libs/stencil.md) — fills the handler / cluster-reviewer prompt
  templates (markdown + marker fields → prompt); the bulk blob is Go-assembled in burler and passed
  as a value. Fails loudly on an unfilled marker (e.g. an empty `fasit`).
- `mux` transitively, via shuttle (the cluster window; the worktree column).
