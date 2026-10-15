# Discussion spec — the decision record, the support log and the relevant list

> **Status: Contract — pinned.** This doc pins the format of the three files a design step writes: the decision record, the support log and the relevant list.
> Durable reference doc — kept, not deleted on landing.
> `internal/discussionparser` checks the format.

## The decision record

The decision record is the plan writer's design input, and `relevant.md` is its code head start;
the plan writer reads nothing else from the discussion directory.
It carries these H2 sections, in this exact order, and no others besides the optional eighth:

1. `## Goal`
2. `## Scope`
3. `## Decisions`
4. `## Constraints`
5. `## Auto-mode assumptions`
6. `## Open risks`
7. `## Acceptance criteria`
8. `## Notes for the plan writer` (optional — design hints for the plan writer, never a completeness requirement;
   a hint may name the symbol it is about)

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

## The relevant list

`relevant.md` is the third file in `_lyx/discussion/`, beside the decision record and the support log.
It lists the files, and the members in them, that the writer read and found relevant, each with one line of why.
The list of code the writer read belongs here, never in the decision record.

Loom's Discussion-Write lead alone writes the file; a darn writer does not.
The writer records only what it already read and never searches further to fill the list.
The list is a head start for the plan writer, never a completeness claim: the plan writer explores the codebase itself.

The format is a markdown bullet list, optionally under one H1 title:

- Each top-level bullet opens with one backticked worktree-relative file path.
- The path is followed by backticked member names in that file, in plain Go spelling and unqualified (`PlanTable`, `Config.Validate`), wherever the writer read specific members.
- Then comes ` — ` (space, em dash, space) and one line saying why the file is relevant.
- No glyphs, signatures, `file:line` citations or code excerpts.
- No nested bullets and no other non-blank line.

An example entry:

```markdown
- `internal/x/y.go` `PlanTable`, `Config.Validate` — builds the table the new column joins
```

`internal/discussionparser` checks a present list; a missing list is never a finding.
