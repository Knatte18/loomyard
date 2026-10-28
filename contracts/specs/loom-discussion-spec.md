# Discussion spec — the decision record, the support log and the relevant list

> **Status: Contract — pinned.** This doc pins the format of the three files a design step writes: the decision record, the support log and the relevant list.
> Durable reference doc — kept, not deleted on landing.
> `internal/discussionparser` checks the format.

## The decision record

The decision record is the plan writer's design input, and `relevant.md` is its code head start;
the plan writer reads nothing else from the discussion directory.
It carries these H2 sections, in this exact order, and no others besides the two optional ones:

1. `## Goal`
2. `## Scope`
3. `## Decisions`
4. `## Constraints`
5. `## Auto-mode assumptions`
6. `## Open risks`
7. `## Acceptance criteria`
8. `## PATTERN changes` (optional — the PATTERN entries the task adds, changes or removes, placed after `## Acceptance criteria` and before `## Notes for the plan writer`)
9. `## Notes for the plan writer` (optional — design hints for the plan writer, never a completeness requirement;
   a hint may name the symbol it is about)

No frontmatter: no `format:` field, no `approved:` field.

### The PATTERN changes section

The designer lists here every PATTERN entry its design adds, changes or removes.
The PATTERN edit itself lands in a Webster card, never in the discussion.
Each entry is one bullet, with `<id>` matching `[a-z0-9-]+`:

- `` - `PATTERN-<id>` — adds: <new entry text> ``
- `` - `PATTERN-<id>` — changes: <new entry text> ``
- `` - `PATTERN-<id>` — removes ``

An entry ID is listed at most once.
An `adds` or `changes` bullet prescribes the overview line `` - `PATTERN-<id>` — <new entry text> ``.
The discussion check refuses that line, as a `pattern-change-over-cap` finding, when its rule text, the new entry text without a trailing `(test)` or background link, is longer, in characters, than the PATTERN rule-text cap the plan gate's `pattern-entry-line-cap` applies;
the cap is the same number whatever the entry's name, and `lym loom decision add --pattern-text` applies it when the decision is recorded;
detail that does not fit belongs in the entry's background file.
`lym loom decision add` appends a bullet with the suffix ` — added after Discussion (<by>, <date>)`.
A record without the section is valid.

### Decision ids and acceptance coverage

Each decision under `## Decisions` is one H3 headed `### D<n> — <title>`, with `n` a positive integer and ids unique within the record.
An `### Added after Discussion` entry carries no id.
Any other H3 there is refused, as a `decision-heading-malformed` finding, and a repeated id as a `decision-id-duplicate` finding.
Each id appears as a whole word in a bullet of `## Acceptance criteria`, so `D1` is not met by `D10`, and a range such as "D1 to D4" covers only the ids it spells.
An id no bullet names is a `decision-not-in-acceptance` finding.
Both checks run for loom and darn records alike, in the discussion gates and `lym loom validate-discussion`, and not in `lym loom decision add`.
Lines inside fenced blocks are not read.

### Citations in a loom record

On a loom run, the discussion check tests the paths and symbols the record cites in backticks; a darn record's citations are not tested.

- A token is a path when it holds a `/`, none of `#>*{}$:`, no `...` segment, and its first segment is a root entry git tracks, an entry staged and never committed included; `_lym/` and `.lym/` paths are never tested.
  It must exist in the worktree, and a token ending in `/` must be a directory; otherwise the check reports `citation-path-missing`.
- A token is a glyph when it parses as a member glyph `<unit>#<member>` whose unit holds a `/` or `.` and does not name an existing file other than a `.go` or `.cs` one, so `docs/overview.md#Modules`, `C#` and `## Decisions` are no glyph.
  A Go symbol is cited qualified with its package, `pkg.Member` or `pkg.Member.Sub` with a trailing call parenthesis ignored, or as a glyph.
  A qualified symbol resolves as the glyph of the one package directory named `pkg`; two such directories are `citation-symbol-ambiguous`, and the way forward is to spell the symbol as a glyph.
  A glyph or symbol that does not resolve is `citation-symbol-missing`, and one declared in several places is `citation-symbol-ambiguous`; cite the declaring file by its path in place of the symbol.
- A task that creates or removes the cited thing marks it with ` (new)` or ` (removed)` right after the closing backtick, and a marked citation is not tested.
- A token holding whitespace or `<` is not tested, so a name that must not exist is written as a `<placeholder>`.
- The parts no writer authors are not tested: fenced blocks, `### Added after Discussion` entries, appended `## PATTERN changes` bullets and carry-over blocks.

Bound: a marked token or one holding `<` is exempt, so a writer can excuse a wrong claim that way and review still reads it.
A path outside backticks, a slash-free file name, a package pattern with `...`, a token under an untracked root entry, and run-state paths under `_lym/` or `.lym/` are not tested, nor is a member after the first of `pkg.Member.Sub`.
The Discussion-Write and Discussion-Burler-Fix gates halt on a quarry outage as an infrastructure fault.

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

- `### Added after Discussion (<by>, <date>): <title>` entries under `## Decisions` are appended only by `lym loom decision add`.
  A writer never edits or removes one, and a record that drops an entry its history held fails the format check.
- A `## PATTERN changes` bullet carrying the ` — added after Discussion (<by>, <date>)` suffix is appended only by `lym loom decision add`.
  A writer never edits or removes one, and a record that drops a bullet its history held fails the format check wherever the check for dropped `### Added after Discussion` entries runs: the darn discussion gate, `lym loom validate-discussion` on a darn run and the verb's darn-record check.
  A loom run's discussion check reads no history for either.
- `<!-- lym:carry-over … -->` blocks under `## Open risks` are written only by Go.
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

`relevant.md` is the third file in `_lym/discussion/`, beside the decision record and the support log.
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
