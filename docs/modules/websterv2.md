# webster v2 — card-level parallel implementation, and structured impact lookup (DRAFT / concept)

> **Status: DRAFT / concept. Not built, not scheduled.** Two companion design notes for a
> possible evolution of the `webster` module (see
> [builder-contract.md](../reference/builder-contract.md#webster-the-fork-based-sibling)),
> merged into one file since they were always meant to be read together. Both are speculative
> in the same sense as [long-term-ideas.md](../long-term-ideas.md): nothing here is committed to
> until v1 (`internal/websterengine` + `internal/webstercli`) has run in anger and the
> parallelism payoff is measured, not assumed.
>
> - **Part A** is the card-level parallel-execution design. Treat every number in it as an
>   estimate from a single case study, not a guarantee.
> - **Part B** is a separable idea that surfaced while discussing Part A's case study: replace
>   LLM-driven ripple-search with a deterministic Go verb. It supersedes an earlier draft that
>   proposed sharing ripple-search results via Master's in-context "impact index" — this revision
>   describes a strictly better mechanism (§B.2). Its underlying tooling was tested independently
>   of webster's own timeline as wiki task `codeintel-spike` (done; see §B.6), with a follow-up
>   wiki task `codeintel-multilang` now open to generalize the LSP arm beyond Go.

---

# Part A — Card-level parallel implementation

## A.1. Where v1 stands, and what v2 changes

**webster v1 (built):** one long-lived **Master** session reads the codebase and the whole
plan once, then forks **one implementer per batch, sequentially, in-session** (Claude Code's
Agent tool). Cross-batch context is carried by a distilled **digest** (`Distill`/`Digest` +
`RecordBatch`), never a raw file re-read. There is **no concurrency** between forks and **no
worktree isolation** — because forks never edit files at the same time. v1 captures the
*robust* win (warm shared context: "read once, fork many") without paying any parallelism tax.

**webster v2 (this doc):** keep the Master + warm-fork + digest engine unchanged, and add a
**scheduling layer on top** that runs *multiple forks concurrently within a batch*, each in an
**isolated git worktree**, gated by an explicit **card-level dependency graph**. v2 does not
replace the v1 engine — it *consumes* it. Everything v1 built (the shuttle fork-audit seam,
`Distill`/`Digest`, `CheckFork`/`CheckParent`, `State`/`BatchState`, `Begin`/`Record`/`Recover`,
templates, weft commits, `summary.md`, run-level) is model-invariant substrate that v2 needs
regardless.

The v2 bet rests on one empirical claim, examined in §A.6: **at card granularity, real
implementation plans are wider than their batch structure suggests** — so a card-level
scheduler can extract parallelism the batch-sequential model leaves on the table.

## A.2. The two sources of speedup — keep them separate

They behave differently and must not be conflated:

1. **Warm context (fork inherits Master).** A cold implementer re-*derives* orientation through
   tool calls (Read/Grep/Glob to learn where things live, how the module fits, what
   `CONSTRAINTS.md` demands) — tens of thousands of tokens and *serial latency before the first
   edit*. A fork inherits the *result* of Master's exploration. This is the moat, and it is a
   **one-time, per-unit** saving that exists **regardless of parallelism**. v1 already captures it.
2. **Parallelism (many forks at once).** Bounded hard by the **critical path** (Amdahl). Only
   materializes when the work graph fans out. This is v2's added ambition, and it is where the
   worktree cost and coordination tax are paid.

Prompt caching already covers the *static* prompt text; it does **not** cover the re-derived
exploration. That exploration is the expensive, identical-across-cold-implementers work fork
inheritance eliminates — the single reason "read once, fork many" pays.

## A.3. Redefining the Card

The `card` concept was designed for mill-plan / mill-go: an **ordered chunk sized to fill one
implementer thread**, with **advisory** file lists, whose ordering is encoded *implicitly by
position in a batch* executed by a single sequential session. Almost every property of that
card is wrong for parallel fork-per-card execution. v2 tightens the contract on four axes:

| Axis | mill-go / webster v1 card | webster v2 card |
|---|---|---|
| **Granularity** | Chunk sized to a thread; multi-step | **Extremely atomic** — the smallest independently-committable unit; oversized cards must be split (cf. `RoleMasterOversized`) |
| **File lists** | Advisory (`Context`/`Creates`/`Edits`), scope-policing only | **Hard, verified contract.** `depends-files` (read) and `changes-files` (write) are exhaustive and machine-checked; a fork touching anything outside `changes-files` fails closed |
| **Dependencies** | Implicit — position within the batch | **Explicit `depends-cards` edges** at card granularity (card B names the cards whose *symbols* it needs) |
| **Self-containment** | Leans on prior cards in the same warm thread | **Fully self-contained brief** — a v2 fork does **not** see sibling forks, so everything it needs lives in the card + Master substrate |

The name "card" can survive; the **contract** must harden. Concretely a v2 card carries:

```yaml
card: 20
name: begin-batch
commit: "webster: BeginBatch/BeginDeps/BeginResult and per-batch model assertion"
changes-files:            # EXHAUSTIVE write set — fork fails closed if it must touch more
  - internal/websterengine/beginbatch.go
  - internal/websterengine/beginbatch_test.go
depends-files:            # EXHAUSTIVE read set the fork is guaranteed
  - internal/websterengine/state.go
  - internal/websterengine/config.go
  - internal/websterengine/roles.go
depends-cards: [8, 9, 10, 3]   # semantic edges: needs Config, roles, State/BatchState, ModelSwitch
```

**Why exhaustive-and-verified, not advisory:** the whole parallel schedule is computed from
these lists. If they are wrong, forks race on undeclared files or run before a symbol exists.
In v1 an implementer that discovered an out-of-scope file just ran the "STOP → extend plan →
commit plan-edit first" protocol serially. Under v2 that discovery is a **scheduling fault** —
see §A.7.

## A.4. Redefining the Batch (= a wave)

A v2 **batch is a wave**: the maximal set of cards with **no unmet dependencies**, run **in
parallel**, then **integrated and tested as a unit**. This is exactly a topological level of
the card DAG. The batch keeps three roles it also had in v1, and gains one:

- **Transaction / integration boundary** — the point where the wave's worktrees merge into the
  task branch and the *full* verify runs (per-fork verify is local-only; see §A.5).
- **Digest boundary** — where each fork's distilled outcome is recorded (`RecordBatch`) and the
  Master's shared substrate advances.
- **Deviation-synchronization point** — this is the subtle one. Forks in the *same* wave run
  simultaneously, so a fork **cannot see a sibling's deviation mid-flight**. Deviations surface
  only at the batch boundary, in the digest, and propagate to the **next** wave's forks. This is
  precisely why a wave's cards must be file-disjoint, and why "all later forks learn about a
  deviation" happens *at the boundary*, never intra-wave.
- **(new) Fan-out unit** — instead of one sequential implementer, the batch dispatches N forks,
  one per card, concurrently.

So v2 did **not** escape the batch — it kept it as the topological/synchronization unit, and
moved parallelism from *between* batches (where v1/mill-go found it rarely paid) to *within* the
batch (where the forks share Master's warm context). The batch-level DAG is dropped; a
**card-level** dependency graph replaces it.

## A.5. Execution model

Per wave:

1. **Compute the wave.** From the card DAG (semantic `depends-cards` edges ∪ file-conflict edges
   derived from `changes-files` overlap), take all cards whose dependencies are satisfied.
2. **Verify intra-wave disjointness.** No two cards in the wave may share a `changes-files`
   entry. If two do (e.g. both edit `cli.go`), serialize them into consecutive waves — never run
   them concurrently.
3. **Burst-spawn forks while the cache is warm.** Fork one implementer per card, each in an
   **isolated git worktree** (`isolation: worktree`), branched from the current task-branch tip.
   Spawn the whole wave *at once*: fork-spawn cost is dominated by prompt-cache state (5-min TTL),
   so a warm burst hits cache on the shared Master prefix; dribbling forks out re-pays uncached
   context per fork.
4. **Fork does its card + LOCAL verify only.** The fork edits only its `changes-files`, compiles
   *its own package* (not `go test ./...`), and writes a **distilled report to file**.
5. **Integrate at the boundary.** Merge the wave's worktrees back onto the task branch, then run
   the **full** verify once. This is the only place the expensive suite runs, and the only place
   an integration failure (N forks each green vs base, union red) can surface — so it is where
   localization is preserved.
6. **Record digests, advance Master, next wave.**

## A.6. Does the parallelism actually materialize? (the case study)

A card-level dependency analysis of *this very plan* (the 42-card master-builder plan that built
webster v1) is the first data point, and it **overturned the naive "linear chain" assumption**:

| Metric | Value |
|---|---|
| Cards | 42 |
| Batches (mill-go, sequential) | 9 |
| **True card-DAG depth (critical path)** | **7** |
| Peak wave width | **10** |
| Cards off the critical path | **83 %** (35 of 42) |
| Wave widths (1→7) | 10, 9, 7, 7, 6, 2, 1 |

Findings worth carrying into the design:

- **The batch DAG over-constrains.** Batch 2's `depends-on: [1]` is largely spurious at card
  granularity — its foundation cards need nothing from batch 1 and belong in wave 1. ~26 cards
  (waves 1–3) could have run as three parallel waves instead of spread across four sequential
  batches.
- **File-conflicts barely bind.** Because the plan is **create-then-extend**, nearly every
  file-conflict pair is *already* dependency-ordered into different waves. Only **one** true
  intra-wave conflict existed (`webstercli/cli.go`, two cards). Good card structure keeps write
  conflicts out of parallel waves almost for free.
- **The tail is the real ceiling, not dependencies.** `cmd/lyx` registration (needs *all* verbs)
  → sandbox (needs the registered module) is a hard 2-wave funnel; wave widths crash 6→2→1
  regardless of fork budget. The speedup is **front-loaded**.

**Honest speedup estimate: ~2–3× wall-clock**, discounted from the analysis's 3–5× ideal
because a wave's wall-clock is its **slowest** card, and the heavy implementation cards
(`BeginBatch`, `RecordBatch`, `Run`) sit *on* the critical path — so width doesn't reduce below
the heavy-card chain. Add fork-carry overhead and edit-time-discovered edges (§A.7) and 2–3× is
the honest band. Real, worth it — but not 6×, and not free.

**Two caveats that both push the real number down:**

- The semantic edges were *inferred* from card descriptions, so wave widths are **optimistic**.
  Real edit-time dependencies will only add edges and shrink waves.
- **This task is atypically large.** 42 cards in one task is far above normal — most tasks carry
  a handful of cards. A big task is a *favorable* case for parallelism (more cards → wider
  waves), so the 2–3× here is closer to an **upper bound** than a typical figure. A routine
  5–10-card task has little fan-out headroom and would show a speedup near 1×, dominated by
  warm-context (source 1), not parallelism. This is the central reason v2 must be gated on
  measurement across *real* plans (§A.9), not justified by this one flattering case.

## A.7. Hazards and open problems

1. **Undeclared file touches (the scheduling fault).** v1 handled "I must touch a file outside
   my card" by stopping and extending the plan serially. Under v2 a fork discovering this is a
   **race**: it either blocks (serializes) or collides with a sibling. v2 must make the
   `changes-files` set **fail-closed** — a fork that needs more must abort its worktree and
   escalate to the Master, which re-plans that card into a later wave. This is the single biggest
   risk to the whole model, and the reason cards must be genuinely atomic with airtight file
   lists.
2. **Semantic coupling ≠ file overlap.** Two cards with disjoint files can still be dependent
   (B calls A's new symbol). The scheduler needs **both** edge types; file-disjointness alone
   would run B before A's symbol exists.
3. **Integration hazard.** N forks each individually green against the base can produce a union
   that is red — they branched from the same Master SHA, not from each other, so the merged tree
   is one none of them compiled against. Mitigated (not eliminated) by the wave-boundary full
   verify; when it goes red, re-merge incrementally to localize.
4. **Uneven card sizes.** Wave wall-clock = slowest card. Width-based speedup overstates when a
   wave mixes one heavy impl card with trivial doc/test leaves.
5. **Worktree cost.** ~200–500 ms setup + disk per fork. Only pays off on genuinely wide waves —
   another reason to gate v2 on measured width, not hope.
6. **Master hygiene is load-bearing.** A fork inherits *everything* in Master, so Master must be
   lean **by construction** — holding only the current wave's substrate, deviations folded in as
   deltas, never accumulated success narratives. On the happy path Master reads a fork's outcome
   as **boolean + SHA only**; the distilled report exists on disk for the *failure* path and human
   audit, and Master opens it only when a fork failed. Ingesting N full reports is N×(report size)
   straight back into Master — which destroys the "read once, fork many" gain on the back end.
   **However:** a fork *must* still report **deviations** (what departed from the plan), because
   those change the substrate later waves inherit. The rule is precise: **Master ingests
   deviation deltas, never success narratives.** On a clean card the delta is empty and Master
   reads only the SHA.

## A.8. Two separable wins — you can take the cheap one first

The case study implies v2's value splits into two independently-shippable pieces:

- **(a) A planner that emits true card dependencies** instead of mill-go's over-constrained
  batch line. This alone recovers most of the *width* — and it needs **no worktrees and no
  concurrent execution**. mill-plan changes its output; the scheduler derives waves. Cheap,
  low-risk, worth taking regardless of whether (b) ever ships.
- **(b) A parallel executor with worktree isolation** that actually *runs* the width. This is the
  big change §A.5 describes, and the one worth gating on evidence.

Ship (a), measure real wave widths across several plans, and build (b) only if the widths hold
up under real (not inferred) dependency graphs.

## A.9. Decision gate — when is v2 worth building?

Run the card-DAG width analysis (§A.6) across several *real* completed plans — and weight for
**typical** task size, not this outlier. Then:

- **Wide (fat waves, short critical path, low file-conflict):** v2's executor pays — build (b).
- **Narrow (long critical path, most cards chained, or simply few cards):** parallelism won't
  materialize; v1 sequential-warm was not just the MVP, it was the *complete* correct design.
  Don't build (b); take win (a) if the planner change is cheap.

The overhead calculus per card (cold-start orientation is a one-time expensive serial cost;
fork-carry is a cheap per-turn cost when cached) means fork-per-card is a clear win for **small
atomic cards** and can *lose* for large multi-turn cards — which is the deepest reason §A.3
demands extreme atomicity. v2 lives or dies on how atomic and how airtight-in-file-declaration its
cards are.

---

# Part B — Structured impact lookup via go/packages / gopls

## B.1. The gap this responds to

Discussing Part A's case study, the `mill-go` thread made a sharper point than §A.1's own
framing: the expensive part of an implementer's turn is usually not *re-deriving general
orientation* (where things live, what `CONSTRAINTS.md` demands) — it's **finding every place
affected by the one specific edit a card makes**, and confirming every follow-on fix lands. That
ripple/impact search is inherently **per-card**: Master's one-time initial exploration cannot
have pre-computed "who calls the function card 7 is about to change," because the edit hasn't
happened yet. A warm fork inherits Master's *general* orientation, but still has to pay the full
ripple-search cost itself — today, via LLM-driven Grep/Read — exactly like a cold implementer
would.

Consequence: webster v1's warm-start benefit is real but narrower than it first appears — it
eliminates redundant *generic* exploration, not the dominant *card-specific* impact-search cost.
v2's parallel executor doesn't fix this either; it just lets several forks pay that same
per-card cost **concurrently** instead of serially (a real but Amdahl-bounded win, and one that
depends on v2's risky airtight-dependency-graph machinery actually working — see §A.6–A.7).

## B.2. The idea

Go has a direct analog to what Roslyn gives C#/.NET: `go/packages` + `go/types` (whole-module,
type-checked semantic analysis, not textual matching), and `gopls` — the official language
server — is built on exactly that stack. It already answers, precisely and deterministically:

- "find all references" to a symbol (`textDocument/references`)
- call hierarchy, incoming/outgoing (`callHierarchy/incomingCalls` / `outgoingCalls`)
- interface implementations (`textDocument/implementation`)

For transitive impact (not just direct callers), `golang.org/x/tools/go/callgraph` builds a
whole-program call graph (CHA/RTA/VTA — increasing precision and cost).

So instead of the earlier draft's mechanism — Master doing an LLM-driven search once and
sharing the result via context inheritance, which only pays off when cards' impact areas happen
to overlap — expose this as a **Go verb**: a fork (or Master, or any LLM session) shells out to
`gopls` (or calls `go/packages`+`go/types` directly, in-process) and gets back a precise,
structured list of call sites for a given symbol, on demand, per card, at near-zero cost. No
context-sharing trick, no overlap requirement, no prefetch-timing guess.

This is a strict upgrade over the earlier draft:

1. **Precise** — no false positives/negatives from textual grep.
2. **Cheap** — zero LLM tokens spent on the search itself; it's a native computation, not a
   sequence of exploratory tool calls.
3. **Needs no cross-card overlap to pay off** — any fork calls it fresh, for its own card,
   whenever it needs it. This removes the central limitation of the earlier draft entirely (see
   §B.4).

## B.3. The real remaining cost: package-graph warm-up, not search results

`go/packages`/`gopls` must load and type-check the module before it can answer anything —
that's a real, possibly non-trivial cost on a large repo (seconds, not milliseconds). That load
is the one thing actually worth amortizing across a run: keep the loaded package graph warm
(e.g. a long-lived `gopls` process, or a cached `go/packages` load) across every query in a
plan's execution, rather than re-loading it per card. This is a much cheaper and more reliable
thing to keep warm than "an LLM's prior grep conclusions" (the earlier draft's mechanism) — it's
a deterministic cache, not something that depends on which cards happen to share symbols.

Open questions a spike needs to answer, not assumed here:

- Load/warm-up cost on this repo's actual size, and whether it's cheap enough to pay once per
  `lyx webster run` (or once per `lyx builder run`) without materially affecting wall-clock.
- `gopls`-as-subprocess (LSP over stdio) vs. `go/packages`+`go/types` called directly from a Go
  verb's own process — **these are not just an efficiency tradeoff, they differ in whether the
  capability generalizes beyond Go at all.** `go/packages`/`go/types` is a Go-only library — it
  cannot exist for a Python, TypeScript, or Rust target repo. LSP is a protocol, not a Go thing:
  every mainstream language has a server implementing the same `textDocument/references` /
  `callHierarchy/*` methods (pyright/pylsp, typescript-language-server, rust-analyzer, clangd,
  …). A Go verb built as a generic stdio LSP client generalizes across target-repo languages for
  free — the per-repo cost is picking which server binary to shell out to, not rewriting the
  verb — while the direct-library route only ever pays off for Go targets, since `lyx` itself is
  written in Go, and would need a wholly separate implementation per language to match. Given lyx
  is used against non-Go target repos too (see §B.4), this makes the LSP-subprocess route the
  strategic default; direct `go/packages`/`go/types` use is at best a Go-target-only fallback or
  optimization, not the general answer — this is worth weighing alongside the raw
  overhead-vs.-reimplementation cost the spike measures.
- Precision limits worth knowing up front: interface satisfaction, reflection, and generics can
  make "who calls this" incomplete or over-broad depending on which call-graph algorithm
  (CHA/RTA/VTA) is used — a spike should characterize this on real code, not assume CHA's
  cheap-but-imprecise answer is good enough. Note the callgraph algorithms (CHA/RTA/VTA) are
  themselves Go-only (`golang.org/x/tools/go/callgraph`) regardless of which option above is
  picked — an LSP server's own `callHierarchy` methods are the only route to comparable
  transitive-impact answers for non-Go languages.

## B.4. Relationship to v1 and v2 — this is no longer regime-specific (or language-specific)

`lyx` is routinely used against target repos that aren't Go — webster/builder implementers work
in whatever language the target project is written in, not necessarily Go. That makes the
language-generality point in §B.3 more than a hypothetical: a Go-only implementation of this
capability (the direct `go/packages`/`go/types` route) would only ever help implementers on Go
target repos, leaving the common non-Go case with no structured lookup at all. The
LSP-subprocess route is what makes this capability actually general-purpose across `lyx`'s real
usage profile, not just across webster's own plan shapes.

The earlier draft of this note only helped "narrow/coupled" plans (§A.9's two regimes) because
its mechanism depended on cards' impact areas overlapping. A Go-verb-based lookup has no such
dependency — it's a flat, on-demand, per-query capability any fork can use regardless of plan
shape:

- **Sequential (webster v1):** each batch's fork calls it for its own card instead of paying the
  ripple-search cost via LLM-driven Grep/Read.
- **Parallel (webster v2, if ever built):** every concurrent fork in a wave calls it independently
  — cheap and stateless, no coordination needed between forks. This also directly attacks
  §A.6's finding that wave wall-clock is dominated by the *heaviest* card on the
  critical path (`BeginBatch`, `RecordBatch`, `Run`): if impact-search is what makes those cards
  heavy, a fast precise lookup shrinks the ceiling v2's own case study identified, independent of
  whether v2's scheduling machinery ever gets built.

In other words: this is not a webster-specific idea at all. It is a general capability that
would reduce tool-call/token burden for **any** LLM-driven module that currently does ripple
analysis via Grep/Read — `builder`'s implementers today, `webster`'s forks, and `burler`'s
reviewers, not only a hypothetical future v2.

## B.5. Cost and risk profile

- No new execution shape, no worktrees, no scheduling faults — this is additive tooling any
  session can call, not a change to webster's fork/batch model.
- New dependency: either a `gopls` binary available in the environment, or a vendored
  `go/packages`/`go/types`-based implementation — either way, a new external-tool or library
  dependency to vet and pin.
- Precision is bounded by static analysis limits (see §B.3) — this augments, not replaces, an
  implementer's judgment; a card's fork should still be free to Grep/Read further if the
  structured answer looks incomplete.
- Fits the existing digest discipline used elsewhere in webster/builder: a Go verb computes and
  returns a structured, bounded answer; the LLM reads it rather than performing the raw search
  itself.

## B.6. This doesn't need to wait for webster or loom

Unlike Part A's parallel-executor idea, this capability's value is not contingent
on webster v1's hardening, on loom, or on any card-DAG planning problem — it is useful today,
for `builder`'s implementers as they run in production right now. That is why it was spiked as
its own standalone wiki task, `codeintel-spike` (no dependency on `webster`/`loom`, **now done**)
rather than folded into webster's roadmap milestone. The spike's harness measured both the
Go-only in-process arm and the `gopls`-as-subprocess (LSP) arm side by side; its findings on the
LSP arm's cost/precision are the starting point for the open follow-up wiki task,
`codeintel-multilang`, which generalizes the LSP-client plumbing to non-Go target-repo languages
(§B.3/§B.4 above).

## Related

- [builder-contract.md](../reference/builder-contract.md#webster-the-fork-based-sibling) — the v1
  cross-module contract webster shares with `builder`.
- [long-term-ideas.md](../long-term-ideas.md) — the original speculative parallel-batches-via-DAG
  note Part A makes concrete, and the "impact index" note Part B's earlier draft descended from.
- `internal/websterengine` package docs — the v1 engine Part A builds on.
- wiki task `codeintel-spike` — the standalone feasibility spike for the `go/packages`/`gopls`
  tooling Part B relies on (done).
- wiki task `codeintel-multilang` — the open follow-up generalizing Part B's LSP arm beyond Go.
- `docs/reference/model-spec.md` — the registry pattern `codeintel-multilang`'s per-language
  server mapping is modeled on.
