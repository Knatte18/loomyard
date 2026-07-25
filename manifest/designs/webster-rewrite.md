# webster: rewrite for the flat card list

> **Status: Design — not built.** A redesign of the shipped `webster` module (fork-based sibling
> of `builder`, one long-lived Master session forking one implementer per unit) to consume
> [plan-format v3](../../docs/reference/plan-format-v3.md) instead of today's shipped,
> batch-based plan-format v2.
> **The core orchestration loop (Master + warm-fork + digest engine) is expected to be largely
> reusable — this is a rewrite of the plan-consumption layer, not from scratch.** Per the
> [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold
> into `internal/websterengine`'s package doc and `docs/reference/builder-contract.md` when this
> lands; this file is then deleted.

## `builder` becomes obsolete

`builder` — the older, separate, cold-start-per-batch reimplementation of Millhouse's Python
builder (no fork usage) — is **obsolete** and not an active consumer of the plan format going
forward. A/B-testing `builder` vs. `webster` on the shared batch-plan-format (the last window to
do so fairly, before webster leaves that format for good) was considered and **explicitly
declined** — months of hands-on Millhouse experience already gives enough confidence that
forking outperforms cold-start-per-batch; a formal test wasn't judged worth the time.

## Fork contract

Each card-implementer fork returns either `OK, SHA <x>` (build + unit tests passed) or a short
deviation note. **A file-list mismatch against declared `changes-files` is always informational,
never blocking on its own** — Millhouse's own production experience shows plan-predicted impact
area is frequently incomplete; treating deviation as failure would make the system impractically
brittle.

## Scheduling: no DAG, no SCC merging in v0

Cards run strictly in declared order. The whole DAG/cycle-detection/SCC-merging mechanism is
designed [below](#continuous-dag-update-as-cards-land-deferred-with-the-symbol-fields) but
depends on `codeintel`/symbol fields and is out of scope until those land (see the roadmap's
Someday list).

**Write the "which card is next" scheduler with a conditional branch from day one**, even though
only one branch is live in v0:

```go
if card.HasSymbolFields() {
    // Mechanism 1: DAG from plan-internal cross-matching (dead code until codeintel lands)
} else {
    // v0: declared order
}
```

This costs nothing now and turns the eventual codeintel rollout into "planner starts populating
fields," not a future webster code change.

### Mechanical DAG derivation (designed now, wired in later — dead code until codeintel lands)

Not active in v0 (no symbol fields exist yet to derive edges from), but designed in full now so
the eventual rollout is "planner starts populating fields," not a future webster code change.

#### Mechanism 1 — plan-internal symbol matching (works even for not-yet-existing symbols)

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

#### Mechanism 2 — codeintel as a verification layer, not a graph builder

Once a symbol is known-real (i.e. for `edits-symbols`, which claims to modify something that
already exists), codeintel can verify: does it actually exist with that exact name, and are
there references to it elsewhere in the codebase — outside the plan's own cards — that no card
accounts for (a real safety-net the plan couldn't have known about without reading the whole
codebase)?

### Symbol fields and the planner/webster codeintel-availability mismatch (resolved)

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

### Continuous DAG update as cards land (deferred with the symbol fields)

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

## Fork failure → stop the plan, escalate to a human

No automatic retry-with-a-stronger-model (Millhouse's "bumped model" pattern) in v0 — a real,
valuable idea, but adds open questions (retry limits, how much failure context to feed forward,
whether to reset to a clean state) not worth resolving before a first working run. Revisit once
failure frequency is actually observed on real plans.

## Integration test suite

Runs as its own dedicated fork, **once**, after all cards have landed — not per-card, not
periodic, not in a separate worktree in v0. Sequential (webster waits for it before proceeding
to loom's finishing step), no commit from this fork. On failure: bisect against the per-card SHAs
already available from the "OK, SHA X" notices (cheap — logarithmic number of re-runs, not
linear) to localize the offending card, then escalate.

Webster writes a summary document (built from the accumulated per-card OK/deviation notices)
that becomes the merge-commit message when loom merges the finished work back into the parent.

## Master's context management

- Master reads all background material once, then forks one implementer per **card** (not per
  batch — batch granularity is dropped along with the batch concept itself, see
  [plan-format-v3.md](../../docs/reference/plan-format-v3.md); the old "N cards, one verify at the end" behavior is
  not reproduced by making one giant card, since that would destroy the fine-grained
  collision/rollback/localization properties the small-card model depends on).
- After each card, Master appends a short status line to its own context before forking the next
  card — the fork inherits Master's *updated* context (deviation notes included) via
  prompt-cache continuity, at near-zero marginal token cost, rather than the fork having to
  re-discover anything itself.
- **Principle: "Master ingests deviation deltas, never success narratives."** A clean card gets
  one line. A card that deviated from its declared contract gets a short, explicit correction —
  framed as background info, not an instruction to act on (a later card that legitimately owns
  the same file works from disk directly regardless).
- **Context growth/compaction risk for long plans (e.g. 40+ cards):** Claude Code's built-in
  auto-compact is a lossy, generic LLM summarization pass that (a) isn't guaranteed to preserve
  exact symbol names/paths precisely, and (b) resets prompt-cache continuity for forks spawned
  right after it fires (new prefix, first use). Prefer a **self-controlled checkpoint
  mechanism**: webster periodically writes its own structured state file (card statuses, SHAs,
  accumulated deviations — not free text) at a token threshold *below* Claude Code's automatic
  trigger, and starts a fresh Master session from that file, rather than trusting the built-in
  summarizer to decide what survives.

## Fork overhead economics, per card

- Real cost: each fork spawn is a prompt-**cache read** against Master's accumulated context at
  that point — not a full re-processing (cache-*write*, much more expensive), but not free
  either. Because Master's context grows through the plan, later cards fork against a larger,
  more expensive prefix than early ones — the cost rises through the plan, not flat per card.
- **The right comparison isn't against zero cost — it's against not forking at all:** Master
  doing every card inline avoids repeated cache-reads but dumps all cards' exploration noise
  directly into Master's own context, hitting the compaction risk much sooner and harder.
  Fork-per-card trades a known, cheap, repeated cost for avoiding an unknown, more expensive
  context-pollution risk.
- **Possible future optimization** (a webster execution-policy decision, not a plan-schema
  concern, not built now): let webster decide a given card is trivial enough that Master handles
  it inline without forking, reserving fork overhead for cards where the isolation benefit
  actually outweighs the spawn cost.
- **Recommended:** measure actual `cache_read_input_tokens` growth through a real 40+-card plan
  rather than assume.

## What Master should read before forking Card 1

**Should read:**
1. The full card list (all cards, not just card 1) — needed for the orchestration job itself
   (DAG tracking, cycle detection, picking the next ready card).
2. Project conventions (build commands, unit/integration test split, style rules) — identical
   across all cards, read once and flow via cache to every fork.
3. Coarse orientation via raddle's `Overview.md` (and maybe top-level module docs) — the "which
   neighborhood" level, not "which house" detail.
4. Starting SHA/current branch state — the reference point for post-commit git-diff
   verification.
5. Confirmation that the relevant language's codeintel daemon(s) are up and healthy — a health
   check *before* the first fork, not something discovered broken mid-plan.

**Should NOT read:** deep, file-by-file reading of everything the plan touches — that's each
individual fork's own job, for its own narrow card. Pre-loading it for all cards builds an
expensive cache that's ~98% irrelevant to any given card.

**Rule of thumb:** identically relevant to all cards → read into Master. Specific to one or a
few cards → let the relevant fork fetch it itself, on demand.

**Caveat about raddle specifically:** raddle files are a **snapshot** of the codebase from
*before* the plan started — Master (and any fork inheriting its context) must treat raddle
content as "how things were before this plan," never outranking a fresh codeintel query or an
actual file read once cards have started landing. Worth an explicit sentence to that effect in
Master's startup prompt.

## Testing strategy

- Unit tests: fast, mocked, no LLM calls — run in full after **every** card
  (`go build ./...` + unit tests).
- Integration tests: the expensive, LLM-calling tests — run at a less frequent checkpoint (end
  of plan, or every N cards), not after every card.
- **Go caveat:** Go's test cache is **per test binary (roughly per package)**, not per test
  function — no built-in way to say "cache this cheap test but never this expensive one" within
  the same package. Physically separating unit and integration tests (different files/build
  tags, not just `testing.Short()` checks within the same file) matters — if a package mixes
  both in one test binary, a single change forces the whole binary (including the expensive
  tests) to re-run regardless of caching.
- codeintel can narrow the expensive gate further, once available: `References`/`Definition` on
  symbols touched since the last full-suite run can filter down to only the integration tests
  that actually cover the affected code, instead of running the whole expensive suite every
  checkpoint. This is a webster-level use of codeintel's output, not a new card-schema field.

## Operational check (do before trusting a long real run)

Verify Claude Code CLI version is ≥2.1.90 and confirm actual Agent-fork prompt-cache hit rate
(`cache_read_input_tokens` vs `cache_creation_input_tokens`) on a real multi-card run. A known,
version-dependent Claude Code regression (partially fixed in 2.1.90, some related reports still
open) can silently degrade fork economics; webster's whole cost model assumes cache reuse is
working.

## Adjacent pieces (not webster's own job, but webster hands off to them)

- **The planner instruction** (feeds webster its input) — converts a discussion-protocol thread
  into a flat card list per [plan-format-v3.md](../../docs/reference/plan-format-v3.md). Own doc:
  [loom-planner.md](loom-planner.md).
- **Loom's Finalize phase** — merge-in from parent, conflict resolution, optional PR creation.
  Own doc: [loom-finalize.md](loom-finalize.md). **Before writing it from scratch: check
  Millhouse's existing auto-merge machinery for direct reuse/porting** — it's already
  production-tested, including against the exact kind of plan-vs-actual-impact drift discussed
  above; likely the strongest, most battle-tested candidate for anything touched on here.

## Superseded: the more aggressive parallel-card-execution design

An earlier, more aggressive design (`manifest/designs/websterv2.md`, now retired — explored both
pre-vacation and again during the vacation discussion) proposed worktree-per-card parallel
execution with semantic `depends-cards` edges, file-conflict detection, and wave-based concurrent
forking (a 42-card case study estimated ~2–3× wall-clock speedup). **Both explorations rejected
it for now**, for concrete reasons: git's index/staging area is a single shared file per working
tree, so concurrent forks committing — even to disjoint files — race on the same lock; codeintel
would see other forks' uncommitted, potentially syntactically-broken in-flight edits; and a
declared-disjoint card pair that turns out to actually overlap is a live corruption risk without
worktree isolation, not just a bookkeeping problem to fix after the fact. See the roadmap's
Someday list and
[webster-parallel-execution.md](webster-parallel-execution.md) for the parked design and full
case-study data.

## Related

- [plan-format-v3.md](../../docs/reference/plan-format-v3.md) — the input contract this rewrite consumes.
- [fabric.md](fabric.md) — `ChangedFilesSince`/`SnapshotSHA` used for contract verification.
- [loom.md](loom.md) — the phase machine this module's output feeds into (Builder phase).
- [codeintel-redesign.md](codeintel-redesign.md) — what the (currently omitted) symbol fields
  depend on.
