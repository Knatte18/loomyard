# Discussion spec — the decision record and the support log

> **Status: Contract — pinned.** This doc pins the format of the two files a design step writes: the decision record and the support log.
> Durable reference doc — kept, not deleted on landing.
> `internal/discussionparser` checks the format.

## The decision record

The decision record is the plan writer's sole input;
it never reads anything else out of the discussion directory.
It carries these H2 sections, in this exact order, and no others besides the optional eighth:

1. `## Goal`
2. `## Scope`
3. `## Decisions`
4. `## Constraints`
5. `## Auto-mode assumptions`
6. `## Open risks`
7. `## Acceptance criteria`
8. `## Notes for the plan writer` (optional — a non-exhaustive head start for the plan writer, never a completeness requirement;
   the plan writer explores the codebase itself)

No frontmatter: no `format:` field, no `approved:` field.

### Compaction rules

- **`## Decisions` carries Decision + Rationale only.**
  Never list a rejected alternative here — those go to the support log's `## Rejected alternatives` section instead.
  A decision record that re-litigates what was *not* chosen is not distilled.
- **Must-cover test scenarios go under `## Acceptance criteria`.**
  There is no separate `## Testing` section.
  A scenario names a behavior to cover, not a test to write, and an existing test or a new table row may meet it.
- **A Decision that adds an edge or weakens a guard states its bound.**
  When a Decision introduces a new verb, flag, escape hatch or routing edge, or removes, downgrades or makes bypassable a guard, state in that Decision what it can now skip or let through and what bounds it.
  The discussion review flags one left unbounded.
- **No italic prose-coaching.**
  Write terse, structured prose the plan writer can act on directly — not a template with meta-commentary about how to fill it in.

### Parts no writer authors

- `### Added after Discussion (<by>, <date>): <title>` entries under `## Decisions` are appended only by `lyx loom decision add`.
  A writer never edits or removes one, and a record that drops an entry its history held fails the format check.
- `<!-- lyx:carry-over … -->` blocks under `## Open risks` are written only by Go.
  A writer never authors, edits or removes one.

## The support log

The support log is read only by the discussion review, never by the plan writer.
It carries these H2 sections, in this exact order:

1. `## Interview` — turn-by-turn, distilled, not a verbatim transcript.
2. `## Rejected alternatives` — what was considered and not chosen, and why.
3. `## Review rounds` — seeded with the header and a single line reading `_No rounds yet._`;
   the review appends round entries here later.
4. `## Question ledger` — every open and resolved question, including any self-picks made under autonomous mode.
