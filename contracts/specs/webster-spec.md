# Webster contract: the fork-based implementer loop's cross-module contracts

> **Status: Contract — pinned.** This is webster's own consumer-facing cross-module contract — what other modules may rely on, nothing more — a durable reference doc, kept, not deleted on landing. webster's own internals (the fork-based loop shape, the bracket-verb sequence, crash/resume, the integration fork + bisect, and everything else webster keeps to itself) live in `internal/websterengine`'s package documentation, not here.

## What it is

Webster is Loomyard's implementer module: one long-lived **Master** session reads a plan once and forks one implementer per execution batch in-session, sequentially, until the plan is built.
This file pins only the shapes another module is entitled to depend on.
Everything about *how* webster reaches those shapes — the fork mechanism, the bracket verbs, the audit policy, the model assertion, crash/resume, the integration-suite fork and its bisect — is `internal/websterengine`'s own business; see its package documentation for that design.

## Plan input

Webster consumes the pinned flat card-list plan-format via `internal/planparser`, the sole parser of `_lyx/plan/` — the format itself is pinned in `contracts/stencils/loom/loom-template-plan.md`, the Plan producer's own stencil, not a separate reference doc.
webster groups a plan's cards into execution batches via a batcher configured through `batcher.yaml`.

## `_lyx/webster/` as an ownership boundary

Webster owns `_lyx/webster/` and everything in it — `state.json`, the reports directory, `outcome.yaml`, `summary.md` — resolved via `internal/websterengine`'s own `Dir`/`ReportsDir` helpers, which are the sole declarers of that path segment.
Its never-tracked siblings — the pause flag, the rendered fork prompts, every `*.lock` — live at the mirrored subpath under `.lyx/webster/` via `internal/websterengine`'s `ScratchDir`/`PromptsDir` helpers, and are deliberately outside the fabric-committed pathspec.
No other module writes into either directory.

## `outcome.yaml`

Master's final-action file:

```yaml
outcome: done | stuck | paused
stuck_reason: null | "<one line>"   # required non-empty when outcome: stuck
batches_done: <int>
```

`outcome` is one of `done`, `paused`, `stuck`.
`stuck_reason` is required non-empty when `outcome: stuck` and empty otherwise.
`batches_done` is the count of batches that reached `status: done` this run.
The file is strictly decoded — unknown fields are rejected — and follows archive-never-refuse: a stale file is timestamp-renamed, never overwritten or refused.

## The summary artifact — `_lyx/webster/summary.md`

The artifact's format, validation, and consumers are pinned producer-agnostically in [final-summary-spec.md](final-summary-spec.md); this section states only webster's own writer-side additions.
It is required and fail-loud only when `outcome: done`, and follows the same archive-never-refuse discipline as every other stale artifact.
See [final-summary-spec.md](final-summary-spec.md) for both of its consumers, because a long-lived Master session is the only party with full oversight of what actually shipped.

A `summary.md` may additionally carry an appended `## Integration suite failed` section naming the bisect-localized offending card and its commit SHA — `internal/websterengine`'s `AppendIntegrationFailure` writes it as the document half of an integration-failure escalation, and it reaches the consumer because Publish passes the parsed body, appended section included, as the pull request's own body field.
The bisect mechanism that produces it stays webster-internal and is not described here.

## See also

- [final-summary-spec.md](final-summary-spec.md) — the producer-agnostic read contract for `summary.md`'s format, validation, and consumers.
- `contracts/stencils/loom/loom-template-plan.md` — the flat-card format webster consumes via `internal/planparser`, pinned in the Plan producer's own stencil rather than a separate doc.
- [llm-model-spec.md](llm-model-spec.md) — the model-spec notation webster's roles resolve against.
- [loom-status-spec.md](loom-status-spec.md) — loom's own status file, the analogous contract for loom's orchestration state.
- `internal/websterengine` package documentation — the as-built code this doc summarizes.
