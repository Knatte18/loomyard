# Review: webster: DAG-derived card sequencing

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:decision] Digest-predecessor lookup fix left as an open fork with no lean
**Section:** Technical context → Gotchas (`beginbatch.go:178` / `recoverbatch.go:216`), also raised in Testing and the Q&A log
**Issue:** The discussion correctly identifies that `State.Batches[batchNumber-1]` is arithmetic on the card number, not the execution predecessor, and that reordering makes this existing lookup wrong — but it explicitly leaves the fix as an open fork ("whether the fix is a true execution-predecessor lookup or a documented accepted deviation is a plan-level call") with no stated lean, unlike every other decision in this document, which names a choice even when deferring exact mechanics.
**Suggested fix:** State a recommended default (e.g. "fix to a true execution-predecessor lookup unless the plan finds a reason not to") so the plan writer starts from a lean rather than a blank two-way fork on a correctness-relevant point.

## Verdict

APPROVE
Scope, constraint coverage, failure modes (cycles, determinism, length-preservation), and every other decision are concretely specified and grounded in cited code locations and the pinned design docs; the one open item is well-surfaced and appropriately scoped to the plan phase.
