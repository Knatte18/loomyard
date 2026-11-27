# Loom parts-spec — the task split into parts

> **Status: Contract — pinned.** This doc pins **parts-format**: the parts file `Parts-Write` produces, which `Parts-Plan-Write` consumes and `internal/partsparser` parses.
> It is `internal/partsparser`'s as-built contract, kept as a durable Go-to-Go reference doc under `contracts/specs/`, not deleted on landing.
> The LLM-facing subset of this format, what `Parts-Write` itself must write, is pinned in the producer's stencil, `contracts/stencils/loom/loom-template-parts.md`, so the agent's prompt never duplicates this file.
> It also pins the part scope a part's plan is checked under and the join of the parts' plans.

## Producer and contract

This format is produced by `Parts-Write` and consumed by `Parts-Plan-Write`.
Its part writers each write the plan of one part, and its chair joins the parts' plans.
It is validated by the `parts` gate and `lym loom validate-parts` over `internal/partsparser`.
The decision record's ids it checks against are read only through `internal/discussionparser`.

## Paths

All paths are under the parts directory `_lym/parts/`, declared only by `internal/partsparser`.

- `parts.md` — the parts file.
- `<part>/` — the directory of one part, holding a plan in the format of `loom-plan-spec.md`, which only `internal/planparser` reads and writes.
- `join.md` — the record the chair's join writes.
- `archive-<stamp>/` — an earlier run's directories, moved aside by the preparations; every archive directory's name starts with the `archive-` prefix.

## Format

````markdown
# Parts: <task title>

## Split note

<how the task splits, or why it does not>

## Part: <name>

- Decisions: D1, D4
- Takes in: <prose>
- Delivers: <prose>
- Exports:
  - `plan:internal/x#NewThing`
  - `internal/x/newfile.go`
- Depends on: <name>, <name>
- Area:
  - `internal/x`
  - `contracts/specs/x-spec.md`

## Shared spot: `<path>`

- Decisions: D3
- Needed by: <name>, <name>
- Needs: <prose>
- Exports:
  - `plan:internal/y#Registered`
- Uses:
  - `plan:internal/x#NewThing`
````

The H1 is `# Parts: <title>`.
One `## Split note`, the `## Part: <name>` sections and the ``## Shared spot: `<path>` `` sections follow, in that order.
A field is a top-level bullet `- <Label>: <value>`.
`Decisions`, `Needed by` and `Depends on` hold comma-separated values.
`Takes in`, `Delivers` and `Needs` hold prose that may continue on indented lines.
`Exports`, `Area` and `Uses` hold indented sub-bullets of one backticked ref each.
`Exports` holds only what the cards create: a `plan:` handle or a file path the worktree does not hold yet.
An existing member or file is never an export, whether the cards keep, re-sign or delete it.
A part requires `Decisions`, `Takes in`, `Delivers` and `Area`; a shared spot requires `Needed by` and `Needs`.
Every other field is optional and is left out when empty, never written as `none`.
Lines inside fenced blocks are read as text.

### Parse errors

`Parse` refuses the file with an error naming the line and what is wrong, with the way forward, for:

- a missing H1 or split note;
- an unknown section heading or field label;
- a required field missing or empty;
- a ref sub-bullet that is not one backticked ref;
- a field repeated in one section.

The way forward is to correct that line to the shape the format above shows.

## Checks

Each finding names the part, or the shared spot's path, and ends in its way forward.

| Check | Raised when | Way forward |
|---|---|---|
| `parts-missing` | the parts file does not exist | write it |
| `parts-malformed` | `Parse` refuses the file | correct the line the error names |
| `parts-none` | the file holds no part | write at least one part, a single part covering the whole task when it does not split |
| `parts-over-max` | more parts than `tapestry.parts_max` | merge parts until at most the maximum remain |
| `part-name-invalid` | a name `seatengine.ValidateSeatName` refuses | rename the part to a lower-case name that starts with a letter |
| `part-name-repeated` | a name an earlier part holds | rename one of them |
| `part-name-archive` | a name beginning with `archive-`, whose directory the preparations would skip as an earlier archive | rename the part |
| `part-dependency-order` | a `Depends on` name that is no earlier part (later, unknown or the part itself), so file order is a topological order | move the depended-on part earlier or drop the dependency |
| `spot-needer-unknown` | a `Needed by` name that is no part | name a part of the file |
| `spot-export-order` | a spot whose `Uses` holds an export of a part that comes after the first part its `Needed by` names, since no card position satisfies both; an export of that first part itself, or of an earlier part, passes | reorder the parts or move the dependency into a part |
| `export-claimed-twice` | a ref two parts, two spots, or a part and a spot both export | keep it on one of them |
| `spot-in-area` | a spot path a part's `Area` lists as a file entry of its own; a spot inside a package directory an area names passes | drop it from the area |
| `export-existing` | a part's or spot's export that is no `plan:` handle and names a member (it holds `#`) or a path the worktree already holds | spell a new member as a `plan:` handle, and drop an existing member or file, saying in `Delivers` when the cards re-sign or delete it |
| `handle-malformed` | a `plan:` export with no unit, or with a file as its unit, judged as a part's Create declaration under the plan format's default language | spell it `plan:<package directory>#<Name>` |
| `fabric-vocabulary-name` | a `plan:` export whose member `loom-plan-checks-spec.md`'s check of that name refuses | rename the member |
| `decision-unnamed` | a decision id of the record no part or spot names | name it in the part or spot that carries it |
| `decision-unknown` | a named id the record does not hold | drop or correct it |

`Validate` reads the parts file and parses it, then reads the decision record.
It runs the checks over the record's decision ids and the worktree's files.
An absent record is not a finding:
it is Discussion-Write's committed output, which the `Parts-Write` agent cannot write,
so the gate ends at once with an error naming `lym loom validate-discussion` for the parent.

## Part plans

### Part scope

A part's plan directory is checked with a part scope, which `lym loom validate-plan --part` sets:
the plan checks of `loom-plan-checks-spec.md` run unchanged, and the scope adds to what they read.
An ordinary plan has no scope.

- The scope lists the `plan:` handles and new file paths that other parts and shared spots export to the part, and the path of every shared spot.
- `handle-dangling` counts a listed handle as claimed; it never counts toward `handle-collision`.
- `path-missing` counts a listed path as satisfied, as a `Create` target is.
  `glyph-not-found` leaves out a target path the part's external exports hold.
- `caller-uncovered` is not reported, since its verdict rests on cards outside the part; the joined plan's gate checks it.
- `part-spot-target` refuses a card's target, never a `Uses` entry, that is a shared spot's path, since the chair writes the spots' cards.
- `part-export-missing` refuses an export of the part's contract that no card of the part creates.

### Joining the parts' plans

`planparser.JoinParts` is the one writer of the joined plan: Go numbers and orders the cards.
It parses each part's plan in the parts file's order.
Before writing anything, it refuses with an error naming the part and the reason when:

- a part's plan does not parse;
- two parts' overviews differ in `language:` or `root:`;
- a part's export is not met: no card creates it (a `Create` target or `Rename` destination).
  A path export is met by its disk path, and a `plan:` handle exactly or by a declared handle that reads to the same unit and name;
- a `plan:` handle or a new file path is created by cards of two parts, exported or not.

Otherwise it numbers the cards from 1 in part order.
Each card keeps its slug;
its heading and `Commit:` prefix take the new number, and each card id naming a card of its own part is rewritten.
It removes every top-level card file and `00-overview.md` of the plan directory, leaving subdirectories in place.
It then writes the joined cards, and writes `00-overview.md` last.
The overview carries `approved: false`, the parts' shared `root:` and `language:`, and a framing paragraph naming the parts for the chair to rewrite.
It also carries the Card Index, each part's `## Shared Decisions` body under `### Part <name>`,
and the canonical `## Rename mechanic` when any part has one.

A read or write failure is not a refusal: running the join again rewrites the whole plan directory from the parts.
