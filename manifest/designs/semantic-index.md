# semantic-index — semantic search over docstrings and descriptive text

> **Status: Speculative, not scoped.** Inspired by
> [Enzyme](https://www.enzyme.garden/blog/an-lsp-for-your-notes), a semantic search system for
> personal note vaults. This is the "deferred idea" [codeintel-redesign.md](codeintel-redesign.md)
> already refers to ("a semantic/conceptual index... a separate, further-out idea, not part of
> this proposal") and the relationship-table row from the original codeintel proposal ("have we
> written something conceptually similar, without shared vocabulary? — embeddings + temporal-decay
> weighting; not part of this proposal") — now named, not yet designed in depth. Per the
> [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever
> picked up the durable parts fold into the owning package's doc when it lands; if abandoned,
> this file is simply deleted.

## The problem this responds to

Grep/text-search finds literal keyword matches. It cannot answer "find code that does X" when
the code implementing X uses none of the words a caller would naturally search for — e.g. "show
me the error-handling patterns in this codebase" when error handling is spelled out in prose
inside docstrings and comments but never literally contains the word "error" everywhere it
matters. `codeintel` (see [codeintel-redesign.md](codeintel-redesign.md)) doesn't solve this
either — it answers "what exactly references/defines this symbol," a precise, compiler-derived
question, not "what have we conceptually written that's similar to this."

## Core mechanism, adapted from Enzyme

Enzyme indexes a personal notes vault; the same shape maps onto a codebase's descriptive text
(docstrings, comments, package `doc.go` headers):

1. **Catalyst generation.** Enzyme derives "thematic questions" from actual content per tag/link/
   folder in a notes vault. For code, the analogous unit is likely per-package or per-module:
   derive thematic questions from that package's docstrings, comments, and `doc.go` header.
2. **Vector embedding.** Catalysts are embedded as vectors, with cosine similarity precomputed
   against every text chunk (docstring, comment block, doc header).
3. **Temporal decay weighting.** Recently written/modified text influences the index more
   heavily than old, rarely-touched text — the index shifts with the codebase's current shape,
   not a static snapshot taken once. For code specifically, "recency" is naturally available
   from git history (`git log` per file/function) — plausibly the same kind of SHA-diffing
   `internal/gitrepo` already does for other consumers, rather than a new mechanism.
4. **Semantic retrieval.** An agent searches catalyst vectors instead of guessing grep terms,
   surfacing conceptually related code across files/packages that share no literal vocabulary.

## Relationship to `codeintel` and `raddle` — complementary, not overlapping

Same three-way split the original codeintel proposal already drew, now with a name for the third
row:

| Module | Answers | Mechanism |
|---|---|---|
| `codeintel` | "What exactly references/defines this symbol, right now?" | Deterministic, compiler-derived (LSP) |
| `raddle` | "Where does this conceptually belong, and why?" | LLM-authored/maintained narrative docs |
| `semantic-index` (this) | "Have we written something conceptually similar, without shared vocabulary?" | Embeddings + temporal-decay weighting |

None of these three replace either of the others — different question, different mechanism.

## Open questions (genuinely unscoped)

- **Indexing granularity.** Per-function docstring, per-file, or per-package `doc.go` — Enzyme's
  own granularity (tag/link/folder) doesn't map onto code 1:1; needs its own design pass.
- **Embedding provider.** Self-hosted vs. API-based — cost, latency, and offline/air-gapped
  operation all matter differently here than for a personal notes tool.
- **Temporal decay source.** Whether it reuses `gitrepo`'s `ChangedFilesSince`/SHA machinery
  directly, or needs its own recency signal.
- **Standalone vs. baked into loomyard.** Same question already asked of `codeintel` and
  `raddle` — lean build-inside-first, extract only once a second concrete consumer exists.
- **Consumer.** Presumably the planner (finding existing similar implementations before writing
  a card) and webster forks (finding a pattern to follow) — not yet concretely designed.

## Related

- [codeintel-redesign.md](codeintel-redesign.md) — the precise, compiler-derived sibling; already
  named this as an explicitly out-of-scope, deferred idea.
- [raddle.md](raddle.md) — the curated-narrative sibling.
- [`internal/gitrepo`](../../internal/gitrepo/doc.go) — plausible source of the temporal-decay
  recency signal.
