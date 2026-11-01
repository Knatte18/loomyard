# Loom parts-spec — the task split into parts

> **Status: Contract — pinned.** This doc pins **parts-format**: the parts file `Parts-Write` produces, which `Parts-Plan-Write` consumes and `internal/partsparser` parses. This is `internal/partsparser`'s own as-built contract — every check below is already implemented, not a future spec — kept as a durable Go-to-Go reference doc under `contracts/specs/`, not deleted on landing. The LLM-facing subset of this format — what `Parts-Write` itself must write — is pinned separately in the producer's own stencil, `contracts/stencils/loom/loom-template-parts.md`, so the agent's prompt never duplicates this file and the two cannot drift from being the same doc.

## Producer and contract

This format is produced by `Parts-Write` and consumed by `Parts-Plan-Write`, whose part writers each write the plan of one part and whose chair joins the parts' plans.
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

The H1 is `# Parts: <title>`; then one `## Split note`, the `## Part: <name>` sections and the ``## Shared spot: `<path>` `` sections, in that order.
A field is a top-level bullet `- <Label>: <value>`.
`Decisions`, `Needed by` and `Depends on` hold comma-separated values.
`Takes in`, `Delivers` and `Needs` hold prose that may continue on indented lines.
`Exports`, `Area` and `Uses` hold indented sub-bullets of one backticked ref each.
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
| `decision-unnamed` | a decision id of the record no part or spot names | name it in the part or spot that carries it |
| `decision-unknown` | a named id the record does not hold | drop or correct it |

`Validate` reads the parts file, parses it, reads the decision record and runs the checks over the record's decision ids.
An absent record is not a finding: it is Discussion-Write's committed output, which the `Parts-Write` agent cannot write, so the gate ends at once with an error naming `lym loom validate-discussion` for the parent.
