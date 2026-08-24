# Discussion format redesign — bounded exploration scope and the relocation-finding principle

> **Status: designed, not implemented.** Supersedes `contracts/stencils/loom/loom-template-discussion.md`'s Step 2 ("Explore before asking", as bounded below) and Step 3's `Architecture` interview category, and nothing else in that stencil.
> It does not supersede Step 3's other five categories (Scope, Constraints, Edge cases, Security, Testing), does not supersede Step 5's section shapes, and does not supersede the stencil as a whole.

## What is not changing

Loom's existing `_lyx/discussion/` two-file split is correct as-is and this doc does not touch it.
`decision-record.md`'s seven required H2 sections (`Goal`, `Scope`, `Decisions`, `Constraints`, `Auto-mode assumptions`, `Open risks`, `Acceptance criteria`) stay unchanged, and so do `support-log.md`'s four required H2 sections (`Interview`, `Rejected alternatives`, `Review rounds`, `Question ledger`).
`internal/discussionparser/validate.go`'s mechanical checks are unaffected, because they check heading presence and file existence, never content quality — nothing this doc describes is checkable by that validator.

## Fix 1 — exploration scope

This is a standing rule for `Discussion-Write`: at a coarse level, it MAY ask whether the design conflicts with an existing pattern and which module boundary the work falls under.
It MUST NOT enumerate exact signatures, file:line citations, interface shapes, or dependency lists, and it MUST NOT do exhaustive existing-pattern research — that class of fact is Quarry's (or manual grep's) job, computed fresh at Plan time.

The bound is stated once, at the exploration-behavior level, rather than once per file the content would land in, because it applies regardless of which of `decision-record.md` or `support-log.md` the content would land in — the bound is about what `Discussion-Write` gathers in the first place, not about which file absorbs the result.

Step 2's own disposition follows from the same bound, stated so a later stencil rewriter has no unresolved conflict to reconcile: Step 2's instruction to read the codebase before asking the operator anything stays, because it bounds redundant *questions* rather than interview *content*.
But it takes the same bound as Step 3 does — pre-interview exploration must not become exhaustive architecture/interface/dependency gathering either.
Step 2 and Step 3 therefore state the identical bound from their own angles: Step 2 bounds what to explore, Step 3 bounds what to ask.

The bound is a positive/negative pair rather than a bare prohibition on purpose.
A purely negative instruction under-specifies how much is too much, and risks either continued over-gathering at the margin (a bare "don't gather deep detail" leaves "how deep is deep" to guesswork) or an overcorrection that refuses to note a genuinely decision-relevant fact (an agent erring maximally safe stops noting anything, including facts the interview actually needs).
Stating both the coarse-level MAY and the exact-detail MUST NOT gives the agent a concrete line to hold, in both directions.

## Fix 2 — review-loop principle

Three parts, stated here as the writer-side rationale; the durable, authoritative copy of the rubric text itself lives in [loom.md](loom.md#discussion-review-rubric--what-to-also-flag-relocation-and-exclusion), under the `Discussion-Review rubric — what to also flag (relocation and exclusion)` subsection, and the eventual `loom: Discussion-Review producer` task reads that subsection, not this doc — because this doc is deleted at Wave 2, before that task starts.

1. "This belongs in `support-log.md` instead" and "this doesn't belong in Discussion at all" are legitimate `Discussion-Review` findings, on equal footing with gap-filling findings — a review loop that can only resolve a finding by adding content is the concrete mechanism behind discussion files that only grow across rounds.
2. **The completeness-before-leanness test.**
   Before relocating anything, check whether it carries a requirement or constraint Planner needs.
   If it does, extract that into `decision-record.md`'s own Decisions or Constraints first, and only then move the surrounding deliberation narrative — because `Plan-Write` never reads `support-log.md` at all, so moving something out is a genuine loss rather than a lower-visibility relocation.
3. **The writer/reviewer symmetry note.**
   Whatever the writer's stencil says not to gather, the reviewer's rubric must say not to flag as missing, or the same purely-additive, non-convergent bias reappears with the writer-side fix in place.

Cross-reference: [review-finding-classification.md](review-finding-classification.md) documents the same symmetry principle independently, as its own item 5, without adopting its finding-class vocabulary — the two docs arrived at the same conclusion from different evidence, and neither depends on the other.

## Lifecycle

This doc is a module-design draft per [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), and it stays a draft until the `loom: Discussion-Write producer` task (Wave 2) lands, at which point Fix 1's content folds into the stencil itself and this doc is deleted.

Every inbound markdown link under `manifest/` pointing at this file breaks identically at that deletion; retargeting or removing them is the deleting task's job, not this doc's.
They currently sit at `manifest/designs/loom.md` (its producer-table row and its relocation-rubric subsection), `manifest/designs/plan-card-format.md`'s status blockquote, and `manifest/roadmap.md` (the card-format group intro and this item's `## Done` entry).
The natural retarget for each is the stencil, where Fix 1's content lives after the rewrite; `manifest/roadmap.md`'s `## Done` entry may instead keep a historical reference and drop out of the Markdown Link Integrity scope — that task's call.

Fix 2's *content* survives this doc's deletion, because its durable copy is in `manifest/designs/loom.md`, but the *links* pointing at this file from Fix-2-related spots break exactly like the Fix-1 ones and need the same treatment.

## Open, not decided here

- The stencil rewrite itself — the `loom: Discussion-Write producer` roadmap item.
- The actual `Discussion-Review` rubric text — the `loom: Discussion-Review producer` roadmap item.
- `review-finding-classification.md`'s finding-class vocabulary, which stays its own Someday proposal.
