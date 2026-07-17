# webster — structured impact lookup via go/packages / gopls (DRAFT / concept)

> **Status: DRAFT / concept. Not built, not scheduled.** Written in the `loomyard` worktree
> while `websterv2.md` (the card-level parallel-execution design) was being discussed in the
> `master-builder` worktree, so both land side by side once that task merges. Supersedes an
> earlier draft of this note that proposed sharing ripple-search results via Master's in-context
> "impact index" — a follow-up discussion found a strictly better mechanism (§2), which this
> revision describes. A separate spike task (wiki: `codeintel-spike`) tests the underlying
> tooling independently of webster's own timeline — see §6.

## 1. The gap this responds to

Discussing `websterv2.md`'s case study, the `mill-go` thread made a sharper point than the doc's
own §2 framing: the expensive part of an implementer's turn is usually not *re-deriving general
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
depends on v2's risky airtight-dependency-graph machinery actually working — see `websterv2.md`
§6–7).

## 2. The idea

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
   §4).

## 3. The real remaining cost: package-graph warm-up, not search results

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
  is used against non-Go target repos too (see §4), this makes the LSP-subprocess route the
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

## 4. Relationship to v1 and v2 — this is no longer regime-specific (or language-specific)

`lyx` is routinely used against target repos that aren't Go — webster/builder implementers work
in whatever language the target project is written in, not necessarily Go. That makes the
language-generality point in §3 more than a hypothetical: a Go-only implementation of this
capability (the direct `go/packages`/`go/types` route) would only ever help implementers on Go
target repos, leaving the common non-Go case with no structured lookup at all. The
LSP-subprocess route is what makes this capability actually general-purpose across `lyx`'s real
usage profile, not just across webster's own plan shapes.


The earlier draft of this note only helped "narrow/coupled" plans (§9 of `websterv2.md`'s two
regimes) because its mechanism depended on cards' impact areas overlapping. A Go-verb-based
lookup has no such dependency — it's a flat, on-demand, per-query capability any fork can use
regardless of plan shape:

- **Sequential (webster v1):** each batch's fork calls it for its own card instead of paying the
  ripple-search cost via LLM-driven Grep/Read.
- **Parallel (webster v2, if ever built):** every concurrent fork in a wave calls it independently
  — cheap and stateless, no coordination needed between forks. This also directly attacks
  `websterv2.md` §6's finding that wave wall-clock is dominated by the *heaviest* card on the
  critical path (`BeginBatch`, `RecordBatch`, `Run`): if impact-search is what makes those cards
  heavy, a fast precise lookup shrinks the ceiling v2's own case study identified, independent of
  whether v2's scheduling machinery ever gets built.

In other words: this is not a webster-specific idea at all. It is a general capability that
would reduce tool-call/token burden for **any** LLM-driven module that currently does ripple
analysis via Grep/Read — `builder`'s implementers today, `webster`'s forks, and `burler`'s
reviewers, not only a hypothetical future v2.

## 5. Cost and risk profile

- No new execution shape, no worktrees, no scheduling faults — this is additive tooling any
  session can call, not a change to webster's fork/batch model.
- New dependency: either a `gopls` binary available in the environment, or a vendored
  `go/packages`/`go/types`-based implementation — either way, a new external-tool or library
  dependency to vet and pin.
- Precision is bounded by static analysis limits (see §3) — this augments, not replaces, an
  implementer's judgment; a card's fork should still be free to Grep/Read further if the
  structured answer looks incomplete.
- Fits the existing digest discipline used elsewhere in webster/builder: a Go verb computes and
  returns a structured, bounded answer; the LLM reads it rather than performing the raw search
  itself.

## 6. This doesn't need to wait for webster or loom

Unlike the parallel-executor idea in `websterv2.md`, this capability's value is not contingent
on webster v1's hardening, on loom, or on any card-DAG planning problem — it is useful today,
for `builder`'s implementers as they run in production right now. That is why this is being
spiked as its own standalone wiki task (`codeintel-spike`, no dependency on `webster`/`loom`)
rather than folded into webster's roadmap milestone: if the spike shows a good cost/precision
tradeoff, adopting it early (as a Go verb `builder` implementers can already call) is plausible,
independent of whether webster v2 or its card-DAG planning problem is ever solved.

## Related

- `docs/modules/websterv2.md` (lands with the `master-builder` merge) — the card-level parallel
  execution design this note originally responded to.
- [long-term-ideas.md](../long-term-ideas.md) — the original speculative parallel-batches note
  both v2 and this extension descend from.
- `docs/reference/builder-contract.md` — the v1 cross-module contract webster shares with
  `builder`.
- wiki task `codeintel-spike` — the standalone feasibility spike for the `go/packages`/`gopls`
  tooling this note relies on.
