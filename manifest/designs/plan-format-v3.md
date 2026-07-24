# plan-format v3 — flat card list

> **Status: Design — not built.** Supersedes two things at once: (1) [plan-format.md
> v2](../../docs/reference/plan-format.md) — today's shipped, pinned, **batch**-based contract
> `builder`/`webster` consume; this is a deliberate breaking change to an already-shipped
> contract, not filling an empty gap. (2) the open "decide the plan's card schema" question that
> used to live in `manifest/designs/websterv2.md` — that question is now answered by this doc.
> Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), when this
> lands the durable parts fold into `docs/reference/plan-format.md` (replacing v2) and this file
> is deleted.

## What changes: batch is gone as a plan-schema concept

**Batching is a webster-internal execution-policy optimization, not a plan-schema concept.**
The plan's unit is always the individual **card** — the smallest, most precise, independently
verifiable unit. Webster may later choose to group cards that share context (e.g. same
file/module, per their declared `changes-files`) into one fork purely as a read-cost
optimization — that is a later, measured, entirely internal decision, not something the plan
format needs to express or the planner needs to decide.

## What a card is

The smallest change that:

1. **Compiles/builds on its own** — `go build ./...` succeeds immediately after the card's
   commit. No broken syntax, no reference to a symbol that doesn't yet exist.
2. **Is independently committable** — a meaningful, revertible git commit on its own.
3. **Bundles its own test, when it introduces new behavior** (implementation + `_test.go` in the
   same card). Pure refactors/renames may rely on existing tests instead.

**Key insight, and the reason the schema is shaped this way:** criterion 1 is not just a
post-hoc check — it's *why the dependency graph exists at all*. A card that references a symbol
which doesn't exist yet cannot compile, so it structurally cannot be valid until whatever card
creates that symbol has already landed. The DAG is a **consequence** of the compile-validity
requirement, not a separate constraint bolted on top.

## v0 fields — decided

```
card:              id/number
name:              short human-readable name (used in commit message)
description:       what to build and why — the actual instruction; everything
                    else is contract/metadata around this
changes-files:      all files the card writes to (incl. its own test file)
depends-on:        list of card ids this card depends on
```

**`creates-symbols`/`edits-symbols`/`reads-symbols` are not included at all for v0 — not just
left optional.** They depend on a working, planner-side-verified `codeintel`, which is
deprioritized (see the roadmap's Someday list). Adding them now as unused optional fields would
create confusion later; better to add them explicitly once `codeintel` is actually ready.

**Why `depends-on` is safe to include now, unlike the symbol fields — it carries no
hallucination risk:** it only references other cards within the same plan, written by the same
planner in the same session — never a claim about external code that could turn out to be
wrong. Three reasons to include it in v0:

1. Human-readable context at escalation time (if card 5 fails, is card 6 known to depend on it?).
2. Forward-compatible input for the future DAG mechanism (a cross-check layer once
   codeintel-derived edges exist, analogous to how `SHAExists` cross-checks a stored git
   reference — see [fabric.md](fabric.md)).
3. **A cheap, mechanical, pre-review order-validation gate:** a plan reviewer (`perch`/`burler`)
   — or an even earlier, dedicated Go check before any LLM-based review runs at all — can flag a
   card whose `depends-on` points to a *later* card in the declared order, catching a real
   planner mistake before webster ever forks anything, at zero LLM cost.

## Plan vs. schedule

The flat card list is the **plan** (a DAG of intent: what depends on what). It is not itself an
execution order. Whoever executes the plan (webster today, or a hypothetical future parallel
executor — see the roadmap's Someday list) decides *how* to turn the DAG into an actual run —
sequential-in-declared-order today, potentially wave-based parallel execution for some future
version. **The plan format should not need to change if that execution-policy decision changes
later.**

## Mechanical DAG derivation (designed now, wired in later — dead code until codeintel lands)

Not active in v0 (no symbol fields exist yet to derive edges from), but designed in full now so
the eventual rollout is "planner starts populating fields," not a future webster code change.

### Mechanism 1 — plan-internal symbol matching (works even for not-yet-existing symbols)

Instead of asking the planner to reason globally about cross-card dependencies, each card would
only declare **its own** `creates-symbols`/`edits-symbols`/`reads-symbols` — a narrower, more
checkable judgment ("what do I touch"). Pure Go, no LLM, no LSP:

1. Build a symbol table: `symbol name → which card "owns" it` (from the union of all
   `creates-symbols`/`edits-symbols` across the plan).
2. For each card, for each name in its `reads-symbols`: look it up in the table. If found, add an
   edge from the owning card to this card.

This works identically for symbols that already exist in the codebase *and* symbols a later card
will create — because it never queries the actual codebase, only the plan's own declared data
against itself.

### Mechanism 2 — codeintel as a verification layer, not a graph builder

Once a symbol is known-real (i.e. for `edits-symbols`, which claims to modify something that
already exists), codeintel can verify: does it actually exist with that exact name, and are
there references to it elsewhere in the codebase — outside the plan's own cards — that no card
accounts for (a real safety-net the plan couldn't have known about without reading the whole
codebase)?

## Symbol fields and the planner/webster codeintel-availability mismatch (resolved)

What if the planner runs on a machine with codeintel available, but the implementation later
runs on a machine without it (or vice versa)? Resolved by splitting what the two mechanisms need:

- **Mechanism 1** (plan-internal cross-matching) requires **no live codeintel on webster's side
  at all** — only that the fields exist in the plan (i.e. that the planner had codeintel).
- **Mechanism 2** (verification against the real, live codebase) is the only piece that actually
  requires webster itself to have a live codeintel connection.

**Resulting rule:**

- Planner **has** codeintel → writes the symbol fields, verified at write time.
- Planner **lacks** codeintel → omits the symbol fields **entirely** — never guess a symbol name
  from text understanding alone. An unverified/hallucinated name is worse than no name: it would
  produce a silently-lost dependency edge that nothing detects.
- Webster's behavior is driven by **whether the fields are present in the plan**, not by whether
  webster itself has codeintel: fields present → Mechanism 1 works regardless of webster's own
  codeintel access; Mechanism 2 only activates if webster *also* has codeintel. Fields absent →
  pure v0 behavior.

Net effect: a plan written on a codeintel-equipped machine remains fully valid and still
delivers the DAG benefit even if executed later on a machine without codeintel — it just runs
without the extra verification safety net.

## Continuous DAG update as cards land (designed, deferred with the symbol fields)

Because new symbols only become lookup-able *after* the card that creates them has actually run,
the DAG would need updating incrementally, not just once at planning time:

1. After each card commits, verify what it *actually* touched
   (`fabric.Warp.ChangedFilesSince(...)`, see [fabric.md](fabric.md)) against its declared
   `changes-files`/symbols. Mismatch = a mechanically detected deviation. **This is always
   informational, never blocking on its own** — Millhouse's own production experience shows
   plan-predicted impact area is frequently incomplete; treating deviation as failure would make
   the system impractically brittle.
2. Update graph edges involving only the symbols this card just touched/created (narrow,
   incremental — not a full graph rebuild).
3. Before forking the next card: check if it's still ready to run (all its dependencies now
   satisfied). If not, pick the earliest remaining card that *is* ready instead (greedy
   topological selection, Kahn's-algorithm-style).
4. If **no** remaining card is ready → a genuine cycle has been discovered (only possible now,
   because new-symbol edges weren't knowable until cards landed). Resolve by finding the full
   strongly-connected component (Tarjan's algorithm generalizes "merge these two cards" to "merge
   the whole cyclic group," however large) and merging all cards in it into a single commit/unit.
   Log this as a deviation for planner feedback.

**Deliberately not built even in this future design** (avoid over-engineering ahead of
evidence): elaborate deviation-categorization (mechanical vs. semantic re-planning triggers);
double-diff/stale-deviation-notice cleanup logic for Master's context; file-splitting-by-
function-area collision refinement. Solve if/when actually observed, not preemptively.

## Forward-compat note for webster's scheduler

See [webster-rewrite.md](webster-rewrite.md) — the "which card is next" scheduler should be
written with a conditional branch from day one (`if card.HasSymbolFields() { /* mechanism 1 */ }
else { /* v0: declared order */ }`), dead code until codeintel lands, costing nothing now.

## Explicitly out of scope (superseded by this decision)

An earlier, more aggressive design (`manifest/designs/websterv2.md`, now retired) explored
worktree-per-card parallel execution with semantic `depends-cards` edges and file-conflict
detection. That design is superseded here for v0's scheduling model — see the roadmap's Someday
list and [webster-parallel-execution.md](webster-parallel-execution.md) for the parked
parallel-execution idea and its case-study data.

## Related

- [webster-rewrite.md](webster-rewrite.md) — the module that consumes this format.
- [fabric.md](fabric.md) — `ChangedFilesSince`/`SHAExists` used for contract verification.
- [codeintel-redesign.md](codeintel-redesign.md) — the module the symbol fields depend on.
