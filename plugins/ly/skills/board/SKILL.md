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

Every entry carries `labels`, validated against two lists in `_lyx/config/board.yaml`:

- `types`: the type labels, such as `bug` and `enhancement`. A note carries exactly one, and a task one or more.
- `labels`: every other label, such as an area or `undecided`.

A label in neither list is refused; add it to `board.yaml` first.
A recipe name goes in `recipe`, never in a label.

## Verbs

Each verb below shows one payload.
`lyx board <verb> --help` lists the full key set, so this skill does not restate it.

Create or update an entry, demotion included:

```
lyx board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary","kind":"task","labels":["enhancement"]}'
```

Read a long body from a file, or from stdin with `-`, instead of embedding it in the payload:

```
lyx board upsert '{"slug":"my-task","title":"My Task","kind":"task","labels":["bug"]}' --body-file body.md
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
lyx board list --label bug --label undecided
lyx board get '{"slug":"my-task"}'
```

Remove every done entry:

```
lyx board prune
```

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
- `close` ends a noise issue with a stated `reason` and writes nothing to the board.

## Writing a body

A `body` renders as ordinary markdown with semantic line breaks.
A single newline is a soft break, so structure a body with headings and lists rather than relying on line breaks for layout.
Never name another task as a dependency in a task's brief or body: put it in `depends_on` with `lyx board set-deps`.
The README computes After and Before from `depends_on`, drops a finished entry from both, and `prune` strips it, while a dependency written as prose goes stale once that entry lands.
A relation between notes has no `depends_on`, so it is a sentence in a body.
