# webster: rewrite for the flat card list

> **Status: Design — not built.** A redesign of the shipped `webster` module (fork-based sibling
> of `builder`, one long-lived Master session forking one implementer per unit) to consume
> [plan-format v3](plan-format-v3.md) instead of today's shipped, batch-based plan-format v2.
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
designed in [plan-format-v3.md](plan-format-v3.md#continuous-dag-update-as-cards-land-designed-deferred-with-the-symbol-fields)
but depends on `codeintel`/symbol fields and is out of scope until those land (see the roadmap's
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
  [plan-format-v3.md](plan-format-v3.md); the old "N cards, one verify at the end" behavior is
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
  into a flat card list per [plan-format-v3.md](plan-format-v3.md). No manifest/glossary/
  CONTEXT.md artifact to design around: a "living vision" document (Matt Pocock–style
  CONTEXT.md/ADR split) was explored at length and **deliberately rejected** for this project —
  raddle (code-derived, snapshot-tracked) already owns "what IS," and CONSTRAINTS.md-equivalent
  files already own "what must remain true"; no separate "what SHALL become" artifact was judged
  necessary. See [loom.md](loom.md) for where this fits in the phase machine.
- **Loom's finishing step** — merge-in from parent (in case parent has moved), conflict
  resolution (spawn an LLM fork if a real conflict occurs), optional PR creation if configured.
  Expected to be mostly deterministic Go. **Before writing this from scratch: check Millhouse's
  existing auto-merge machinery for direct reuse/porting** — it's already production-tested,
  including against the exact kind of plan-vs-actual-impact drift discussed above; likely the
  strongest, most battle-tested candidate for anything touched on here.

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

- [plan-format-v3.md](plan-format-v3.md) — the input contract this rewrite consumes.
- [fabric.md](fabric.md) — `ChangedFilesSince`/`SnapshotSHA` used for contract verification.
- [loom.md](loom.md) — the phase machine this module's output feeds into (Builder phase).
- [codeintel-redesign.md](codeintel-redesign.md) — what the (currently omitted) symbol fields
  depend on.
