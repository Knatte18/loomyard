---
name: board
description: Read or write the board — create or promote an entry, find or list entries, prune done ones — always through `lyx board`, never by touching `_board/*.json`.
---

# board

## What it does

The board is the tracker: one entry per piece of work, each with a tier and a type.
It is read and written only through `lyx board`.
Never read, grep or edit `_board/*.json` or the rendered README directly.
Every verb prints JSON, except where `--text` asks for the compact listing.

## Tiers

Every entry carries a numeric `tier`:

- `1` Planned: concretized and claimable.
- `2` Next Up: planned next, not yet concretized.
- `3` Someday: a loose idea.

A new entry without `tier` lands at 3, so claimable work must pass `tier: 1`.

## Types

Every entry carries a `type`:

- `feature`: new behavior.
- `bug`: a defect to fix.
- `chore`: maintenance with no new behavior.
- `design`: a decision or design document, not shipped code.

A recipe name goes in `recipe`, never in `type`.

## Verbs

Each verb below shows one payload.
`lyx board <verb> --help` lists the full key set, so this skill does not restate it.

Create or update an entry, demotion included:

```
lyx board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary","tier":1,"type":"feature"}'
```

Promote an entry to a lower tier number; without `tier` it moves one tier:

```
lyx board promote '{"slug":"my-task","tier":1}'
```

Find entries whose slug, title, brief or body contains the text, done entries included:

```
lyx board find --text retry backoff
```

List every entry, with `--text` for a one-line-per-entry scan, and fetch one entry in full:

```
lyx board list --text
lyx board get '{"slug":"my-task"}'
```

Remove every done entry:

```
lyx board prune
```

## Writing a body

A `body` renders as ordinary markdown with semantic line breaks.
A single newline is a soft break, so structure a body with headings and lists rather than relying on line breaks for layout.
