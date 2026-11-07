# Webster contract: the fork-based implementer loop's cross-module contracts

> **Status: Contract — pinned.** This is webster's own consumer-facing cross-module contract — what other modules may rely on, nothing more — a durable reference doc, kept, not deleted on landing. webster's own internals (the fork-based loop shape, the bracket-verb sequence, crash/resume, the verify gate and its fixers, and everything else webster keeps to itself) live in `internal/websterengine`'s package documentation, not here.

## What it is

Webster is Loomyard's implementer module.
One long-lived **Master** session reads a plan once.
It forks one implementer per execution batch in-session, sequentially, until the plan is built.
It is the `webster` implementer of tapestry's implementer row.
[implementer-spec.md](implementer-spec.md) pins that row's in, out and outcomes.
This file pins only the shapes another module is entitled to depend on.
How webster reaches those shapes is `internal/websterengine`'s own business:
the fork mechanism, the bracket verbs, the audit policy, crash/resume and the verify gate's mechanics.
Its package documentation holds that design.

## Plan input

Webster consumes the pinned flat card-list plan-format via `internal/planparser`, the sole parser of `_lym/plan/`.
The format itself is pinned in the plan-format blocks that `contracts/stencils/loom/loom-template-plan.md` includes;
there is no separate reference doc.
Webster groups a plan's cards into execution batches via a batcher configured through `batcher.yaml`.
The grouping is computed once per run and recorded in `state.json`, and every verb reads the recorded partition,
so a file the batches change never regroups the cards mid-run.

## `_lym/webster/` as an ownership boundary

Webster owns `_lym/webster/` and everything in it: `state.json`, the reports directory, `outcome.yaml` and `summary.md`.
`plan-baseline/` is there too, the content store behind the recorded plan hashes.
`internal/websterengine`'s `Dir` and `ReportsDir` helpers resolve these paths.
They are the sole declarers of that path segment.
Its never-tracked siblings are the rendered fork prompts and every `*.lock`.
They live at the mirrored subpath under `.lym/webster/`, outside the fabric-committed pathspec.
`internal/websterengine`'s `ScratchDir` and `PromptsDir` helpers resolve it.
No other module writes into either directory.

## `outcome.yaml`

Master's final-action file:

```yaml
outcome: done | stuck
stuck_reason: null | "<one line>"   # required non-empty when outcome: stuck
batches_done: <int>
```

`outcome` is one of `done`, `stuck`.
`stuck_reason` is required non-empty when `outcome: stuck` and empty otherwise.
`batches_done` is the count of batches that reached `status: done` this run.
The file is strictly decoded, so unknown fields are rejected.
It follows archive-never-refuse: a stale file is timestamp-renamed, never overwritten or refused.

## The summary artifact — `_lym/webster/summary.md`

Webster's `summary.md` is the run record.
Landing does not read it; landing reads the change description loom's `Describe` row writes instead.
[loom-final-summary-spec.md](loom-final-summary-spec.md) pins the format and validation shared across producers.
This section states only webster's own writer-side additions.
The summary is required and fail-loud only when `outcome: done`.
It follows the same archive-never-refuse discipline as every other stale artifact.
A long-lived Master session alone has full oversight of what shipped, which is why it writes the run record.

A `summary.md` may additionally carry two sections the verify gate produces.
When the gate's rerun passes a failing plan-level verify, the identities that failed once are flaky.
`internal/websterengine`'s `AppendIntegrationTriage` appends an `## Integration suite triage` section listing them.
The run keeps its outcome.
When the gate sends a failure back to Merriam, Merriam answers with one fixer, a fork or the cold fixer.
Merriam then rewrites `summary.md` with a `## Verify gate fixes` section.
The section names the findings, what that fixer says it changed and each `fix: <summary>` commit it made.
The `Describe` stencil carries either section into the change description as a check item for the PR reviewer.
Publish does not pass `summary.md` through.

## The verify gate

Merriam's strand carries the hub's `webster` gate, rendered from `gate.yaml` as one must-pass gate named `verify`.
The gate is the one call `lym gate test --wait --gate webster`.
It runs once per attempt at HEAD after Merriam's turn ends, never inside a batch.
A `gate.yaml` that cannot be loaded, or a `webster` key naming an undeclared tier, is the gate's error, never a pass.
The error names `lym config gate`.
Merriam's strand role is `webster`, so its agent name is `<shortname>:<slug>:webster`.
A failing run is rerun once;
a pass on the rerun passes the gate and records the failing identities as flaky.
A run that exceeds the verify timeout is not rerun, since a hang is not flakiness:
it fails the gate at once with the timeout, the log path and the log tail as findings.
A failure that survives the rerun, or a tree that is not clean, is returned to Merriam as findings.
The gate records it in `verify-gate.yaml` in the reports directory.
Merriam answers each failure with one fixer, whose commits are `fix: <summary>` and are checked at the next attempt.
The gate routes each failure to a fixer and records it as `fixer` in `verify-gate.yaml`.
The default fixer is a fork in Merriam's session, on Merriam's model.
The cold fixer takes the last attempt, and every attempt after a fixer that committed nothing.
`lym webster fix-verify` spawns the cold fixer as its own strand at the `recovery` role.
`verify_gate_attempts` in `webster.yaml` bounds the attempts;
an exhausted gate ends the run `stuck`, as does a fixer commit the gate rejects.
Nothing runs the suite after the session ends.

`lym webster verify` runs the same `webster` gate call through `internal/verifytree`, and boots no reed session.
A pass records the verified tree, so a landing step or a later gate that finds the tree unchanged skips the run.

## See also

- [loom-final-summary-spec.md](loom-final-summary-spec.md) — the read contract for `summary.md`'s format and validation.
- The plan-format blocks that `contracts/stencils/loom/loom-template-plan.md` includes:
  the flat-card format webster consumes via `internal/planparser`.
- [refusal-spec.md](refusal-spec.md) — the way forward every webster refusal names, and its classes.
- [llm-model-spec.md](llm-model-spec.md) — the model-spec notation webster's roles resolve against.
- [loom-status-spec.md](loom-status-spec.md) — the status file contract for loom's orchestration state.
- `internal/websterengine` package documentation — the as-built code this doc summarizes.
