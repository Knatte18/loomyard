# Treadle — a shared Go round-loop engine for `perch` and `tenter`

> **Status: Design sketch, Planned.** Captures where the `perch`/`hardener` unification
> conversation landed, now with settled names. Naming history: this engine was called `gorch`
> during discussion; renamed to **Treadle** (a loom's pedal mechanism, driving each repeated
> pass/pick — apt for a judge/gate/round-spawn loop that drives repeated review rounds). Planned as
> **one combined task with the `perch` rewrite** (see Process below) — see `manifest/roadmap.md`.

## What it is

`Treadle` generalizes `perch`'s existing Go orchestration loop — judge, gate, round-spawn, the
round-caps milestone ladder, pause, run-dir lock, state.json — into a shared engine with one new
extension point: **which round-runner gets spawned is supplied from outside**, not hardwired.

Two named products come from configuring the same engine differently:

- **`perch`** = `Treadle` + `internal/burlerengine` as round-runner + a text-review judge profile
  (rubric, fasit) — perch's existing shape and behavior, unchanged from the outside.
- **`Tenter`** (see [hardener.md](hardener.md)) = `Treadle` + a live-substrate-driving round-runner
  (drives real tmux/processes, builds adversarial scenarios) + a behavior-review judge profile —
  Perch's direct sibling, structurally identical, configured for behavior-review instead of
  text-review. `Tenter` is the review-loop only; the full on-demand campaign that wraps it with a
  worktree-spawn/merge-back lifecycle is **`Hardener`** (`Shed` + `Tenter` — see `shed.md`), which
  stays Someday, deprioritized. `Tenter` itself is Someday too — it exists solely to serve
  `Hardener`, so it isn't needed to get `loom` running and doesn't belong in Planned.

## What's genuinely new vs. what perch already has

Perch's existing, shipped package doc (`internal/perchengine/doc.go`) already treats profile data
(rubric, fasit, gate, caps) as external to the engine — *"Profile ... is data, never code."*
Judge, gating (`llm-verdict`/`command`/`both`), and the round-caps milestone ladder are already
exactly what both `perch` and `hardener` need — no change required there. What's actually new:

1. **Pluggable round-runner.** Perch's `Run` today literally *"spawns a fresh `burlerengine`
   round"* — hardwired, not an interface. `Treadle` needs a round-runner interface (spawn given some
   hydration, return a verdict + artifact path(s)), with `burlerengine` as the default/only current
   implementation and Tenter's behavior-round-agent as a second one.
2. **Judge-maintained handoff** (below) — a real capability addition, not just a Tenter-specific
   bolt-on.
3. **Optional pre-round targeting** (below) — a general capability the judge can support; perch's
   own profile simply won't exercise it.

## The handoff optimization — benefits `perch` too, not just `Tenter`

Perch's judge already runs on every blocking round and today reads **every prior round's** review
file to decide `PROGRESSING`/`CIRCLING`/`UNCERTAIN` — unbounded growth as rounds accumulate. Since
the judge already runs per round regardless, it can also **maintain a handoff**, edited in place
each round (not appended): future judge calls then read {handoff + latest round} instead of {every
prior review + latest}. This is a genuine efficiency improvement to `perch`'s existing, shipped
behavior — independent of whether `Tenter`/`Hardener` ever gets built.

**Hard constraint, carried over from `hardener.md`'s own analysis:** the handoff cannot be a single
free-form prose summary. Perch's circling verdict depends on knowing whether a specific finding has
recurred across rounds — a distilled prose summary that quietly drops a recurring finding breaks
circling-detection silently, which is worse than the O(N) cost it replaces. The handoff needs two
parts: a **structured, lossless finding-identity / rounds-seen ledger** (never summarized away) plus
a **distilled prose narrative** for everything else — *"distill the prose, but keep the key-ledger
lossless."*

## Pre-round targeting — `Tenter` needs it; perch's profile simply won't use it

`Treadle`'s judge should support a **pre-round** call — read the handoff, decide what to target
next, write the next round's seed prompt — as a general capability, not a Tenter-only special case
nor a structural fork from perch. Perch's own profile just never exercises it (its rounds keep
re-using a fixed rubric); Tenter's profile does. Whether this is the same judge invocation as the
post-round call (with a mode flag) or a separate call is an open question for the discussion round
below.

## Process — do NOT fold this into Hardener's task

Perch is shipped and tested, with real documented invariants (milestone-ladder exactness, the three
`GateMode`s, the judge's fail-safe posture, the pause seam, run-dir locking, weft-blindness, the
`profile > perch.yaml > built-in` config resolution order). Extracting `Treadle` out from under it
means preserving all of that while adding the round-runner interface and the handoff/ledger
capability — real refactor risk on working code, not net-new design. Do not bundle this into
`hardener`'s own (already large, still-DRAFT) proposal.

**One combined task, not two: build `Treadle` and rewrite `perch` onto it together.** Most of
`Treadle` is already `perch`'s existing, shipped code (see "What's genuinely new" above — the
round-runner interface, handoff/ledger, and pre-round targeting are the only actual additions).
Extracting a generic engine without immediately proving the one real consumer (`perch`) still
works on it leaves an untested abstraction — the same reason `fabric` was built alongside
`warp`/`weft` as a reference fixture before its own cutover, rather than as a separate,
unvalidated step. Rewriting `perch` onto `Treadle` in the same task is what lets `perch`'s own
existing behavior serve as the differential test: if `perch` still passes with identical
behavior/CLI, `Treadle` has everything `perch` needs.

The agent taking this task must read this doc, `hardener.md`, `shed.md`, and
`internal/perchengine/doc.go` in full before writing anything — this is a real refactor of shipped
code requiring genuine design judgment, not a mechanical rename like `mux-to-reed` was. Normal
discussion → plan → go flow applies.

Sequencing (Planned — see `manifest/roadmap.md`):

1. **`Treadle` + `perch` rewrite, combined** — pin and build the generalized engine (round-runner
   interface, handoff/ledger format, whether pre-round targeting is a judge-mode flag or a separate
   call), informed by both perch's existing call sites and Tenter's needs as the two concrete data
   points (even though Tenter itself is not being built yet), then rewrite `perch` onto it in the
   same task, behavior/CLI unchanged from the outside.
2. **`Shed`** (see `shed.md`) — the separate, outer FSM generalization; a different engine, its own
   independent task, not part of this doc and not blocked on step 1.

Building `Tenter`/`Hardener` on top of `Treadle`/`Shed` is its own, later, Someday task — not
scheduled now, and not required for either step above.

## Related

- [hardener.md](hardener.md) — where `Tenter` (Treadle configured for behavior-review) and
  `Hardener` (Shed + Tenter, the full campaign) are described; Someday, deprioritized.
- `shed.md` — the separate, outer FSM engine `Hardener` also needs (alongside this doc's `Treadle`)
  once it's eventually built.
- `internal/perchengine/doc.go` — perch's current, shipped package doc; the source of every
  "already exists, don't rebuild" fact above.
- [crucible/README.md](../../crucible/README.md) — the hand-run method both perch and Tenter
  automate; the handoff/ledger discipline traces back to its own "two-tier memory" description.
