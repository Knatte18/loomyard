# Module: perch (design)

> **Status: Design — not built.** Per the [documentation lifecycle](../overview.md#documentation-lifecycle),
> this file is deleted when the module lands and its durable parts fold into the package header
> and `overview.md`. It replaces the former `review.md`, split into two modules:
> **`perch` owns the iterative gate loop; `burler` (see the `internal/burlerengine` package
> documentation) owns one review+fix round.**
>
> **Hand-executed prototype:** [`docs/reviews/`](../reviews/README.md) documents the manual,
> human-in-the-loop form of this loop (orchestrator drives rounds, spawns a fresh round agent,
> independently verifies). `perch` is that orchestrator role moved from a human+Claude pair into Go.

`perch` (`lyx perch`) is the **gate engine** — a generic, profile-driven reviewer that takes an
artifact and a *fasit* and returns one of two verdicts: **`APPROVED`** or **`stuck`**. It is named
for *perching*: the cloth-finishing station where woven fabric is draped over a frame under light and
**judged** pass or fail. That is perch's role — it does not do the mending itself (that is
`burler` — see the `internal/burlerengine` package documentation); it runs burler rounds and
**decides when the cloth passes.**

perch is what [`loom`](loom.md) puts between every phase, and it also runs standalone
(`lyx perch <profile>`). One engine serves **all** text-based review — discussion-review,
plan-review, builder-review, and ad-hoc "review this file / this PR" are just different call-sites
with different profiles. (The heavier, behavior-based hardening of live-substrate modules is a
**separate** module, [`hardener`](hardener.md) — see [Not hardener](#not-hardener-perch-is-text-review).)

## The gate — a black box with two exits

From loom's view a review is a **black box** with two exits — `APPROVED` or `stuck`. What happens
inside is not the caller's concern; the block is not finished until the artifact is approved or it is
definitively stuck. Inside, **Go drives a round-loop** — no standing orchestrator agent (that was
only an LLM in mill because orchestration was LLM-driven):

1. Go spawns a fresh `burler` (see the `internal/burlerengine` package documentation) for the
   round (one round: A-review, optional cluster, then B-fix).
2. Control returns to Go, which reads the round's verdict and review file.
3. If not `APPROVED`, Go spawns a **new** burler for the next round, hydrated from the prior round's
   review/fixer-report files.

**Convergence is loop-until-dry, not resolve-one-round.** The block terminates on `APPROVED` only
when a review round finds **zero** blocking findings — a **clean round on top of the previous round's
fixes**. Because every round's fix is judged by the *next* round's fresh burler A (no self-grading),
termination on `APPROVED` always carries an independent confirmation. Resolving the findings of a
single round is **not** convergence; a fresh round must come back empty.

## Stuck detection

`stuck` is the other exit, and it is the hard part. Three mechanisms, layered:

### Round cap (K) — the deterministic backstop

Go's hard floor: a per-profile cap `K` (e.g. 5). The loop cannot run forever.

### Cap escalation — the cap is soft, but bounded

If the artifact is still `BLOCKING` at round K, Go does **not** immediately declare `stuck`. It runs
an extra evaluation — *should the cap be raised?* — because a genuinely-progressing artifact that
merely needs a few more rounds should not be failed on an arbitrary K. **But the escalation is itself
bounded:** a maximum number of raises (or an absolute ceiling `K_max`) so Go **still guarantees
termination.** Without that bound the termination guarantee is gone. (This mechanism — call it
*cap-review* — is a v-later refinement; v1 may ship the plain hard cap.)

### Progress / circularity — the semantic check

It is not just the *count* of blocking findings that matters but the *type*: are we going in circles?
Oscillation can hold the count flat (fix A, break B; next round fix B, break A → count stays at 1, a
perfect loop). So the judge must track finding **identity** across the whole history, not magnitude.

The progress check is the one part that does not become pure Go, for two reasons: it is **semantic**
(is finding A in round 3 the same underlying issue as finding B in round 1? a naive set-diff is
fooled by rewording) and it must be **independent of the burler** (else self-grading — a burler is
motivated to claim progress to avoid being declared stuck). It does **not** need a standing
orchestrator: it is a thin, ephemeral **progress-judge** (a Haiku is enough — bounded
compare-and-classify over short, already-articulated findings) that Go spawns via `shuttle` on
demand.

- It spawns **conditionally**: only after a `BLOCKING` round *and* when there is a prior round to
  compare against (not round 1; not after `APPROVED`).
- Its input is **self-contained** — Go hands it the relevant rounds' reviews (or the canonical-key
  history); it carries no memory between calls.
- It is **fail-safe**: uncertain → default "progressing," and let the K-cap be the hard floor. A
  false "progress" costs a few bounded rounds; a false "stuck" is the costly error, so it must
  require clear evidence of circularity.

**The sharp split:** the progress-judge **canonicalizes** each round's findings into stable keys
(normalize "same issue" → same key); **Go** does the cycle detection deterministically over the key
history ("key X reappeared in rounds 1, 3, 5 → circling"). Judgment (are these the same issue) in the
judge; cycle logic (does the key recur) in Go.

## The pluggable gate — verdict vs. command

A profile's *convergence gate* — what "clean round" means — is **pluggable**:

- **`llm-verdict`** — the round is clean when a fresh burler's A returns `APPROVED`. The default for
  **text** review (discussion, plan): the artifact is prose, and an independent read is the gate.
- **`command`** — the round is clean when a **deterministic command Go runs itself** passes (e.g.
  `go test`, a lint gate, a zero-stray-state check). Go runs it on the committed tree, independent of
  the burler, and trusts the *observed* result over any LLM verdict. This is how a **code** profile
  can gate convergence on real tests rather than an LLM opinion.
- **`both`** — the round is clean only when the burler's A *and* the deterministic command agree.

The command gate lives in **perch** (run independently of the burler), not inside the burler's A —
because the whole point of independent verification is that the decider does not trust the worker.
This is the one place text-review touches behavior; the heavy, substrate-driving, scenario-building
form is [`hardener`](hardener.md), a separate tier.

## Pause at the round boundary

The round-loop is a Go loop, so it honours the shared `pause_requested` flag
([loom](loom.md#graceful-pause)) at the **round boundary** — the atom is the round (after the burler's
A/B and any cluster reviewers aggregate), never mid-aggregation. A long gate (5–6 rounds) therefore
pauses at the *next* round, not only when the block finally exits; resume continues at the current
round (round-level resume already exists via the block's on-disk files). Whichever loop is
innermost-active honours the flag first, so a pause requested during a review lands here, at the round
boundary.

## Configuration — where profiles live

The per-phase profile (discussion / plan / builder) is **not perch's config** — it is
[`loom`](loom.md)'s, because loom owns the phases. This keeps perch a truly generic engine that
never learns the words "discussion" or "plan":

- **`perch.yaml` (engine-general, a fixed key set):** the progress-judge model, the fail-safe policy,
  the default round-cap `K`, the cap-escalation ceiling `K_max`, the default `cluster-N`. Profile-
  independent rails and defaults. (These are so few that v1 may ship them as constants and add the
  config module only when someone wants to tune them.)
- **loom's config (per phase):** which rubric, target/fasit derivation, per-phase round-cap
  override, per-phase `cluster-N`, tool-use grade, gate mode (`llm-verdict` / `command` / `both`).
  Everything phase-specific.
- **Standalone / ad-hoc:** `lyx perch <profile>` supplies the profile **inline** (CLI args or a
  profile file), not from `perch.yaml`.

**This is a recurring design rule, worth stating once:** *knowledge of phases and lifecycle collects
in `loom`; the engines beneath it (`perch`, `burler`) are phase-agnostic.* The same rule places the
composite lifecycle verbs (abandon, spawn, merge) on loom rather than on the leaf modules — see
[loom.md](loom.md). Because the profile-values move up to loom, `perch.yaml` has a fixed key set, so
the config template validation (which assumes a fixed schema) works with no changes.

## Three disciplines that keep this ONE engine

1. **The per-phase difference is the rubric, not the code.** Feed the rubric (and fasit) in; keep one
   engine. Forking per phase loses the point.
2. **The verdict contract is invariant** — `APPROVED | stuck` out of perch, `APPROVED | BLOCKING` +
   findings + fixer-report out of each burler round, regardless of what was reviewed. That invariance
   is exactly what lets `loom` drive all phases identically, and what lets `shuttle`'s engine swap the
   provider without touching perch. Vary the payload, never the control surface.
3. **Rubric and fasit are data.** Tighten "what is a blocking plan flaw" by editing a rubric file and
   every plan-review picks it up — without touching the engine.

## Standalone vs. inside loom

- **Inside loom:** loom calls `perch.Run(profile, worktree)` between phases; it only sees the
  `APPROVED | stuck` exit and advances or escalates ([loom.md](loom.md#the-gate)).
- **Standalone:** `lyx perch <profile>` runs the same block on demand — "review this PR / this file"
  — with no phase machine around it.

## Not hardener — perch is text review

`perch` is a **text-based** reviewer: it (and its burlers) primarily **read** the artifact. Its
`command` gate lets a code profile *touch* behavior by running tests, but that is a light touch.

[`hardener`](hardener.md) is a **behavior-based** reviewer and a **separate module**: it drives a
live sandbox repo, runs slow real substrate operations, builds bespoke adversarial scenarios, and is
orchestrated by an accumulating (per-round-respawn + handoff) LLM orchestrator, not a stateless Go
loop. It shares only the `burler` *round discipline* (see the `internal/burlerengine` package
documentation). perch is on the `shuttle → burler → perch → loom` spine and runs between every
phase; hardener is an on-demand,
token- and wall-clock-heavy campaign run **after** loom, not on the spine. Do not conflate them.

## Dependencies

- `burler` (see the `internal/burlerengine` package documentation) — perch spawns a fresh burler
  each round; burler is the LLM-heavy worker, perch the deterministic loop around it.
- `shuttle` (see the `internal/shuttleengine` package documentation) — spawns the progress-judge (one
  shuttle run, on demand); burler reaches shuttle itself.
- [`internal/stencil`](../shared-libs/stencil.md) — fills the progress-judge prompt template.
- `internal/state` — the block's round state (reviews, fixer-reports, verdict/key history) on disk,
  so a review can resume mid-block at the current round.
- `mux` (see [overview.md#modules](../overview.md#modules)) transitively, via shuttle.
