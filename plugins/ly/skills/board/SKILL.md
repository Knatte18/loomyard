---
name: board
description: Read or write the board — create or promote an entry, find or list entries, prune done ones, take in GitHub inbox issues — always through `lyx board`, never by touching `_board/*.json`.
---

# board

## What it does

The board is the tracker: one entry per piece of work or idea, each with a kind and labels.
It is read and written only through `lyx board`.
Never read, grep or edit `_board/*.json` or the rendered README directly.
Every verb prints JSON, except where `--text` asks for the compact listing.

## Kinds

Every entry carries a `kind`:

- `task`: concrete and claimable; only a task can run, and only a task carries `depends_on`.
- `note`: an idea or observation that is not yet a task, one note per entry.

A new entry without `kind` lands as a note, so claimable work must pass `kind: "task"`.
A note stays as written until the operator decides to build it; then suitable notes are merged into one task, promoted, and removed.

## Labels

Every entry carries `labels`, validated against two maps in the hub's `board.yaml`, each from label to description:

- `types`: the type labels, such as `bug` and `enhancement`. A note carries exactly one, and a task one or more.
- `labels`: every other label, such as an area.

Run `lyx board labels` before choosing labels: it prints both maps in file order with their descriptions.
Beside its type label, every entry carries a label for each lyx module it touches; an entry that spans the whole tool takes the modules its work would change first.
A label in neither map is refused; add it with `lyx config board --set labels.<name>=<description>`, or edit the maps in the `lyx config board` editor.
A recipe name goes in `recipe`, never in a label.

## Verbs

Each verb below shows one payload.
`lyx board <verb> --help` lists the full key set, so this skill does not restate it.
Every verb that takes a slug states the slug length limit there too.

Create or update an entry, demotion included:

```
lyx board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary","kind":"task","labels":["enhancement"]}'
```

Read a long body from a file, or from stdin with `-`, instead of embedding it in the payload:

```
lyx board upsert '{"slug":"my-task","title":"My Task","kind":"task","labels":["bug"]}' --body-file body.md
```

`merge` takes the same flag and sets the body of its `upsert` entry:

```
lyx board merge '{"remove_slugs":["old"],"upsert":{"slug":"my-task","title":"My Task","kind":"task","labels":["bug"]}}' --body-file body.md
```

A merge carries the removed entries' issues onto the upserted entry, after the numbers it already records, whether or not the payload names `issues`.

Edit a body as a file: `get --body` prints the body alone, verbatim, so write it out, edit it and read it back in:

```
lyx board get '{"slug":"my-task"}' --body > body.md
lyx board upsert '{"slug":"my-task"}' --body-file body.md
```

Promote a note to a task:

```
lyx board promote '{"slug":"my-note"}'
```

Find entries whose slug, title, brief or body contains the text, done entries included; `--label` narrows to entries carrying every named label and repeats:

```
lyx board find --text retry backoff
lyx board find --label bug retry
```

List every entry, with `--text` for a one-line-per-entry scan, and fetch one entry in full:

```
lyx board list --text
lyx board list --label bug --label enhancement
lyx board get '{"slug":"my-task"}'
```

Remove every done entry:

```
lyx board prune
```

## Claimed entries

An entry whose status reads `<state> · <producer>`, such as `running · Webster`, is claimed by a run.
A claimed entry takes no scope edits: a write that changes anything but its status, or removes it, is refused.
Record a new finding as a note of its own with `lyx board upsert` and `"kind":"note"`.
The refusal names the way to unlock an abandoned run's entry.

## Intake

GitHub is the inbox that `selfreport` files issues to, and the board is the one list.
Take each open inbox issue onto the board, then close it:

```
lyx board intake list
lyx board intake import '{"issue":12,"slug":"retry-backoff"}'
lyx board intake import '{"issue":13,"into":"retry-backoff"}'
lyx board intake close '{"issue":14,"reason":"Duplicate of #12."}'
```

- `list` prints the open issues no entry records.
- `import` with `slug` records the issue as a new note; with `into` it folds the issue into an existing entry. Either way it comments on the issue with a pointer and closes it.
  An imported note carries only a type label, so give it its module labels with `upsert` at once.
- `close` ends a noise issue with a stated `reason` and writes nothing to the board.

## Writing a body

A `body` renders as ordinary markdown with semantic line breaks.
A single newline is a soft break, so structure a body with headings and lists rather than relying on line breaks for layout.
Never name another task as a dependency in a task's brief or body: put it in `depends_on` with `lyx board set-deps`.
The README computes After and Before from `depends_on`, drops a finished entry from both, and `prune` strips it, while a dependency written as prose goes stale once that entry lands.
A relation between notes has no `depends_on`, so it is a sentence in a body.
