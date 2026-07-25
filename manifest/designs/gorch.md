# gorch — a shared Go orchestrator engine for `perch` and `hardener`

> **Status: Design sketch — not built, not yet discussed in full.** Captures where the
> `perch`/`hardener` unification conversation landed. A dedicated discussion round is still needed
> before this becomes an implementation-ready design — see Process below. Do not implement from
> this doc yet.

## What it is

`gorch` generalizes `perch`'s existing Go orchestration loop — judge, gate, round-spawn, the
round-caps milestone ladder, pause, run-dir lock, state.json — into a shared engine with one new
extension point: **which round-runner gets spawned is supplied from outside**, not hardwired.

Two named products come from configuring the same engine differently:

- **`perch`** = `gorch` + `internal/burlerengine` as round-runner + a text-review judge profile
  (rubric, fasit) — perch's existing shape and behavior, unchanged from the outside.
- **`hardener`** (see [hardener.md](hardener.md)) = `gorch` + a live-substrate-driving round-runner
  (drives real tmux/processes, builds adversarial scenarios) + a behavior-review judge profile.

Naming (`gorch`/`perch`/`hardener`) is provisional — the shape (one engine, two configured
instances) is the part worth pinning now.

## What's genuinely new vs. what perch already has

Perch's existing, shipped package doc (`internal/perchengine/doc.go`) already treats profile data
(rubric, fasit, gate, caps) as external to the engine — *"Profile ... is data, never code."*
Judge, gating (`llm-verdict`/`command`/`both`), and the round-caps milestone ladder are already
exactly what both `perch` and `hardener` need — no change required there. What's actually new:

1. **Pluggable round-runner.** Perch's `Run` today literally *"spawns a fresh `burlerengine`
   round"* — hardwired, not an interface. `gorch` needs a round-runner interface (spawn given some
   hydration, return a verdict + artifact path(s)), with `burlerengine` as the default/only current
   implementation and hardener's behavior-round-agent as a second one.
2. **Judge-maintained handoff** (below) — a real capability addition, not just a hardener-specific
   bolt-on.
3. **Optional pre-round targeting** (below) — a general capability the judge can support; perch's
   own profile simply won't exercise it.

## The handoff optimization — benefits `perch` too, not just `hardener`

Perch's judge already runs on every blocking round and today reads **every prior round's** review
file to decide `PROGRESSING`/`CIRCLING`/`UNCERTAIN` — unbounded growth as rounds accumulate. Since
the judge already runs per round regardless, it can also **maintain a handoff**, edited in place
each round (not appended): future judge calls then read {handoff + latest round} instead of {every
prior review + latest}. This is a genuine efficiency improvement to `perch`'s existing, shipped
behavior — independent of whether `hardener` ever gets built.

**Hard constraint, carried over from `hardener.md`'s own analysis:** the handoff cannot be a single
free-form prose summary. Perch's circling verdict depends on knowing whether a specific finding has
recurred across rounds — a distilled prose summary that quietly drops a recurring finding breaks
circling-detection silently, which is worse than the O(N) cost it replaces. The handoff needs two
parts: a **structured, lossless finding-identity / rounds-seen ledger** (never summarized away) plus
a **distilled prose narrative** for everything else — *"distill the prose, but keep the key-ledger
lossless."*

## Pre-round targeting — hardener needs it; perch's profile simply won't use it

`gorch`'s judge should support a **pre-round** call — read the handoff, decide what to target next,
write the next round's seed prompt — as a general capability, not a hardener-only special case nor
a structural fork from perch. Perch's own profile just never exercises it (its rounds keep re-using
a fixed rubric); hardener's profile does. Whether this is the same judge invocation as the
post-round call (with a mode flag) or a separate call is an open question for the discussion round
below.

## Process — do NOT fold this into hardener's task

Perch is shipped and tested, with real documented invariants (milestone-ladder exactness, the three
`GateMode`s, the judge's fail-safe posture, the pause seam, run-dir locking, weft-blindness, the
`profile > perch.yaml > built-in` config resolution order). Extracting `gorch` out from under it
means preserving all of that while adding the round-runner interface and the handoff/ledger
capability — real refactor risk on working code, not net-new design. Do not bundle this into
`hardener`'s own (already large, still-DRAFT) proposal.

Sequencing:

1. **A dedicated discussion round first** — its one deliverable is a pinned design for `gorch`'s
   interface (round-runner contract, handoff/ledger format, whether pre-round targeting is a
   judge-mode flag or a separate call), informed by both perch's existing call sites and hardener's
   needs as the two concrete data points.
2. **Then, as separate downstream tasks:** rewrite `perch` onto `gorch` (behavior/CLI unchanged
   from the outside), and build `hardener` onto `gorch` (its own, later task).

## Related

- [hardener.md](hardener.md) — the DRAFT module whose design surfaced this generalization; its own
  orchestrator section now points here for the shared-engine design.
- `internal/perchengine/doc.go` — perch's current, shipped package doc; the source of every
  "already exists, don't rebuild" fact above.
- [crucible/README.md](../../crucible/README.md) — the hand-run method both perch and hardener
  automate; the handoff/ledger discipline traces back to its own "two-tier memory" description.
