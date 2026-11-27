# Final-summary spec — the producer-agnostic final-summary artifact

> **Status: Contract — pinned.** This doc pins the final-summary artifact's read contract: what any last-content-producing step writes, and what any consumer may rely on.
> Durable reference doc — kept, not deleted on landing — the read-side counterpart to [webster-spec.md](webster-spec.md), whose own writer-side additions for its own producer stay there rather than here.

## What it is

The final-summary artifact is the prose final-summary a run's last content-producing step writes: a first non-blank line `# <title>`, then free-form prose describing the change.
That step is the `Describe` row of the tapestry, darn and sampler recipes, or the darn writer when the darn review is off.
It writes `_lym/landing/summary.md`.
Its body is a change description, what the change does and why for a PR reviewer, rather than a run narrative.
The contract names no producer, so any last-content-producing step can satisfy it.

## Format and validation

The format is fail-loud and minimally validated.
Each of the following is checked, and each violation is its own distinct error:

- the file must be readable;
- its content must not be whitespace-only;
- its first non-blank line must be a `# ` heading;
- the title after that `# ` prefix must be non-empty.

Everything after the heading line is the body, carried verbatim.

## Sole declarer, sole parser

`internal/summaryparser` is the sole declarer of the artifact's filename and the sole parser of its format.
It takes a told path, which a caller resolves from the containing directory and hands in, or content the caller read elsewhere, such as from an archived revision.
It declares no location of its own.

The contract names no location:
the artifact's directory belongs to whichever producer writes it, and the consumer is handed the path, never a directory it derives itself.

## Consumers

The artifact's consumers are both in `internal/landingshed`:

- **`Publish`** uses the parsed title and body as the pull request's own title and body fields.
- **`Finalize`** uses `LandingMessage` as the landing merge commit's message.
  `LandingMessage` is the title, a blank line, the body with its leading whitespace trimmed, then, after a blank line, the credit block.
  Go composes the credit block from `landing.yaml`'s `credit_*` keys and the engines that ran a session in the task.
  The pull request's body carries the same body and credit block.

Both consumers append the task's regression check, when the task has one, after the body and before the credit block.
It is a `## Regression check` section that `internal/driftengine` renders from the task's Drift records.
The artifact never holds that section itself,
so it reaches the pull request and the landing commit whichever producer wrote the artifact last.
`Finalize` also appends, after the regression check, a `## Flaky test failures` section to its landing commit and its GitHub merge.
It lists each failure of its landing gate that passed when run once more on the same head, and it is absent when there was none.

`Finalize`'s read is unconditional: a missing or malformed artifact there is a hard error.
`Publish`'s read is reached only when the parent branch requires a pull request:
it opens one from the artifact, or refreshes an open one's title and body,
so a re-run Publish carries a rewritten description onto the pull request.
While its review runs, PR-Review reads the artifact at each status poll of an open pull request.
It sets the title and body again when the artifact yields other text, so the pr-reviewer changes them by rewriting the artifact.
A read that fails there is logged and retried at the next poll, never an error.

## Producer-appended sections

A producer may append its own sections to the body after writing it, and any such section rides into both consumers unchanged.
The contract makes no distinction between the heading-adjacent prose and anything appended after it.

## See also

- [webster-spec.md](webster-spec.md) — webster's own writer-side additions: when the artifact is required, its archive-never-refuse discipline, the flaky-failure section the verify gate appends for identities that passed on rerun, and the "Verify gate fixes" section Merriam writes after a verify-gate fixer.
