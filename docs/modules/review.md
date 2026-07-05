# Module: review (design)

> **Status: Design — not built.** Per the [documentation lifecycle](../overview.md#documentation-lifecycle),
> this file is deleted when the module lands and its durable parts fold into the package header
> and `overview.md`. It was split out of [loom.md](loom.md): `loom` owns the phase machine;
> `review` owns the gate that guards every phase.
>
> **Hand-executed prototype:** [`docs/reviews/`](../reviews/README.md) documents the manual,
> human-in-the-loop form of this exact loop (A-review → B-fix, fresh Handler per round, no
> self-grading, independent confirmation) — the method used to harden `mux` over seven rounds
> before merge. This module automates that loop by moving the orchestrator role into Go.

`review` (`lyx review`) is the **gate engine** — a generic, profile-driven reviewer that takes
an artifact and an "against" and returns one of two verdicts: **`APPROVED`** or **`stuck`**. It
is what [`loom`](loom.md) puts between every phase, and it also runs standalone
(`lyx review <profile>`). One engine serves **all** review — discussion-review, plan-review,
builder-review, and ad-hoc "review anything" are just different call-sites with different
profiles. It spawns its handlers, cluster reviewers, and progress-judge through
`shuttle` (see the `internal/shuttleengine` package documentation).

## The X-review block — the gate

From the caller's view a review is a **black box** with two exits — `APPROVED` or `stuck`.
What happens inside is not the caller's concern; the block is not finished until the artifact is
approved or it is definitively stuck. Inside, **Go drives a round-loop** (no standing
orchestrator agent — that was only an LLM in mill because orchestration was LLM-driven):

1. Go spawns a fresh, **tool-based Handler** for the round (one `shuttle` run — see the
   `internal/shuttleengine` package documentation).
2. **A — Review.** The Handler reviews the artifact like a normal reviewer, not yet knowing it
   will also fix. It writes a review to file with a verdict: `BLOCKING` or `APPROVED`. In step A
   it **may** spawn N extra **cluster reviewers**, wait for them (or time out), and write a
   cross-checked review — this is how cluster-review support falls out for free.
3. **B — Fix.** The Handler then fixes what it found, itself, from its own review plus its own
   reasoning — **even if the verdict was `APPROVED`** (non-blocking polish). It writes a
   fixer-report.
4. Control returns to Go, which reads the round status. If not `APPROVED`, it spawns a **new**
   Handler for the next round (2–3 again).

The Handler combines review and fix in one agent on purpose: the review context (explore +
findings) is already loaded, so the fix is cheap — no re-explore, no cold re-read. A fresh
Handler per round, hydrated from the prior round's review/fixer-report files, avoids both of
mill's suboptimal alternatives: (1) the original producer fixing (token-heavy at long resume)
and (2) a separate fixer thread (loses the *why*).

**No self-grading.** A is pure review and precedes B, so A is a legitimate gate identical to a
normal reviewer. The fix from round N is judged by a fresh Handler's A in round N+1. Termination
on `APPROVED` is therefore always a clean review round — every fix gets an independent
confirmation before the block closes.

## Stuck detection

`stuck` is the other exit, and it is the hard part. Two mechanisms:

- **Round cap (N).** Go's deterministic backstop — the loop always terminates.
- **Progress / circularity.** It is not just the *count* of blocking findings that matters but
  the *type*: are we going in circles? Oscillation can hold the count flat (fix A, break B; next
  round fix B, break A → count stays at 1, a perfect loop). So the judge must track finding
  **identity** across the whole history, not magnitude.

The progress check is the one part that does not become pure Go, for two reasons: it is
**semantic** (is finding A in round 3 the same underlying issue as finding B in round 1? a naive
set-diff is fooled by rewording), and it must be **independent of the Handler** (else
self-grading — a Handler is motivated to claim progress to avoid being declared stuck). It does
**not** need a standing orchestrator: it is a thin, ephemeral **progress-judge** (a Haiku is
enough — bounded compare-and-classify over short, already-articulated findings) that Go spawns
via `shuttle` (see the `internal/shuttleengine` package documentation) on demand.

- It spawns **conditionally**: only after a `BLOCKING` round *and* when there is a prior round to
  compare against (not round 1; not after `APPROVED`).
- Its input is **self-contained** — Go hands it the relevant rounds' reviews (or the
  canonical-key history); it carries no memory between calls.
- It is **fail-safe**: uncertain → default "progressing," and let the N-cap be the hard floor. A
  false "progress" costs a few bounded rounds; a false "stuck" is the costly error, so it must
  require clear evidence of circularity.

A sharper split: let the progress-judge **canonicalize** each round's findings into stable keys
(normalize "same issue" → same key), and let **Go** do the cycle detection deterministically over
the key history ("key X reappeared in rounds 1, 3, 5 → circling"). Judgment (are these the same
issue) in the judge; cycle logic (does the key recur) in Go.

## Pause at the round boundary

The round-loop is a Go loop, so it honours the shared `pause_requested` flag
([loom](loom.md#graceful-pause)) at the **round boundary** — the atom is the round (after the
Handler's A/B and any cluster reviewers aggregate), never mid-aggregation. A long gate (5–6 rounds)
therefore pauses at the *next* round, not only when the block finally exits; resume continues at the
current round (round-level resume already exists). Whichever loop is innermost-active honours the
flag first, so a pause requested during a review lands here, at the round boundary, rather than
waiting for the whole phase.

## The review profile

The module **must be configurable on what it reviews**. The per-target configuration is a
**review profile** (discussion / plan / builder are three profiles; ad-hoc review is a fourth):

| Field | Meaning |
|-------|---------|
| **target** | What to read — one file (`plan.md`), or for builder a git-diff against base + the working tree. Not always one file. |
| **against** (fasit) | The source of truth to check the target *against*. The plan is checked against `discussion.md`; the code against `plan.md`. **The easily-missed, most important field** — without it a review degenerates to a pure internal-consistency check, not fidelity to intent. The contract is `{fasit, target} → verdict`, not `target → verdict`. |
| **rubric** | What counts as `BLOCKING` for this target type. Plan rubric ("is the DAG sound, are batches independent, does it cover the discussion") ≠ code rubric ("correctness, tests green, no regression"). Data, not code. |
| **fix-scope** | What the fixer may write — a markdown file (`plan.md`) vs the source tree. |
| **tool-use** | Handler always (reviewing anything real means looking at the world, not just the artifact text). Cluster reviewers graded: builder wants tool-use; discussion can run bulk. |
| **cluster-N, round-cap** | Optional per profile. |

## Three disciplines that keep this ONE module

1. **The per-phase difference is the rubric, not the code.** Feed the rubric in; keep one engine.
   Forking the Handler per phase loses the point.
2. **The verdict contract is invariant** — `APPROVED | BLOCKING` + structured findings +
   fixer-report, regardless of what was reviewed. That invariance is exactly what lets `loom`
   drive all phases identically, and what lets `shuttle`'s engine (see the
   `internal/shuttleengine` package documentation) swap the provider
   without touching review. Vary the payload, never the control surface.
3. **Rubric and fasit are data.** Tighten "what is a blocking plan flaw" by editing a rubric file
   and every plan-review picks it up — without touching the engine. The bar is versionable and
   tunable, separate from the machinery.

## Standalone vs. inside loom

- **Inside loom:** loom calls `review.Run(profile, worktree)` between phases; it only sees the
  `APPROVED | stuck` exit and advances or escalates ([loom.md](loom.md#the-phase-machine)).
- **Standalone:** `lyx review <profile>` runs the same block on demand — "review this PR / this
  file" — with no phase machine around it.

## Dependencies

- `shuttle` (see the `internal/shuttleengine` package documentation) — spawns the Handler, the N
  cluster reviewers, and the progress-judge (each is one shuttle run through an engine). Cluster
  reviewers ask for `display:{anchor:own-window}` so the N of them land in a separate, switchable
  psmux window rather than crowding the column.
- `internal/state` — the block's round state (reviews, fixer-reports, verdict history) on disk,
  so a review can resume mid-block at the current round
- `mux` (see [overview.md#modules](../overview.md#modules)) transitively, via shuttle
