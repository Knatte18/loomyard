# Final-summary spec — the producer-agnostic final-summary artifact

> **Status: Contract — pinned.** This doc pins the final-summary artifact's read contract: what any last-content-producing step writes, and what any consumer may rely on.
> Durable reference doc — kept, not deleted on landing — the read-side counterpart to [webster-spec.md](webster-spec.md), whose own writer-side additions for its own producer stay there rather than here.

## What it is

The final-summary artifact is the prose final-summary a run's last content-producing step writes: a first non-blank line `# <title>`, then free-form prose narrating what was actually built, including deviations from the original task.
Today webster's own `Master` session is that step, but the contract names no producer — a future last-content-producing step (e.g. Tenter) can satisfy it too.

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
It takes a told path — a caller resolves the containing directory and hands it in — and declares no location of its own.

The contract names no location: the artifact's directory belongs to whichever producer writes it, and the consumer is handed the path, never a directory it derives itself.

## Consumers

The artifact has two consumers today, both in `internal/landingshed`:

- **`Publish`** uses the parsed title and body as the pull request's own title and body fields.
- **`Finalize`** uses `CommitMessage` — the title, a blank line, and the body with its leading whitespace trimmed — as the landing merge commit's message.

`Finalize`'s read is unconditional: a missing or malformed artifact there is a hard error.
`Publish`'s read is reached only when the parent branch requires a pull request and no pull request already exists.

## Producer-appended sections

A producer may append its own sections to the body after writing it, and any such section rides into both consumers unchanged — this contract makes no distinction between the heading-adjacent prose and anything appended after it.

## See also

- [webster-spec.md](webster-spec.md) — webster's own writer-side additions: when the artifact is required, its archive-never-refuse discipline, and the integration-failure section webster's own bisect appends.
