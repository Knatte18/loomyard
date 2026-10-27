---
name: board
description: Read or write the board — create or promote an entry, find or list entries, prune done ones, take in the hub repository's GitHub issues — always through `lym board`, never by touching `_board/*.json`.
---

# board

## What it does

The board is the tracker: one entry per piece of work or idea, each with a kind and labels.
It is read and written only through `lym board`.
Never read, grep or edit `_board/*.json` or the rendered README directly.
Every verb prints JSON, except where `--text` asks for the compact listing.

## Kinds

Every entry carries a `kind`:

- `task`: concrete and claimable; only a task can run, and only a task carries `depends_on`.
- `note`: an idea or observation that is not yet a task, one note per entry.
- `project`: a group of entries, with the handle letter `P`.
  A task or note joins one through its `project` field, naming the project's slug, and a project lists the order of its members in `order`.
  A project's `beam` field marks it a beam, a long-lived pair that spawns its members from its own worktree; it is set only by `lym beam create`, never by a board write.
  `lym board list` and `lym board get` carry an entry's `parent_beam`: the slug of the nearest beam above it, empty when the prime is its parent.

A new entry without `kind` lands as a note, so claimable work must pass `kind: "task"`.
A note stays as written until the operator decides to build it; then suitable notes are merged into one task, promoted, and removed.

## Handles

The board gives every entry a short handle, such as `T4`, `P1`, `B2` or `E7`: `T` for a task, `P` for a project, and for a note the first letter of its type label, upper-cased.
The number is the lowest of its letter not in use, and it stays while the entry lives; a promote or a type change gives the entry a new handle of its new letter.
The operator names entries by handle, and every verb that takes `slug` or `id` takes `handle` instead, or the bare handle as its whole argument:

```
lym board get B4
lym board set-status '{"handle":"T4","status":null}'
```

A new task is `draft`; a null status on a task releases it rather than clearing it to a plain empty status.

## Labels

Every entry carries `labels`, validated against two maps in the hub's `board.yaml`, each from label to description:

- `types`: the type labels, such as `bug` and `enhancement`. A note carries exactly one, and a task one or more.
- `labels`: every other label, such as an area.

Run `lym board labels` before choosing labels: it prints both maps in file order with their descriptions.
Beside its type label, every entry carries a label for each lym module it touches; an entry that spans the whole tool takes the modules its work would change first.
A label in neither map is refused; add it with `lym config board --set labels.<name>=<description>`, or edit the maps in the `lym config board` editor.
A recipe name goes in `recipe`, never in a label: `loom` or `darn`, empty meaning `loom`.
A run profile goes in `profile`, never in `run_settings`: `lym board profiles` prints the hub's profiles with one settings block per recipe, and a task without one takes its recipe's default profile, listed under `defaults`.

## Verbs

Each verb below shows one payload.
`lym board <verb> --help` lists the full key set, so this skill does not restate it.
Every verb that takes a slug states the slug length limit there too.

Create or update an entry, demotion included:

```
lym board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary","kind":"task","labels":["enhancement"]}'
```

A note's optional `priority` is `high`, `normal` or `low`; absent means normal, and `"priority":"normal"` clears it.
A task carries none: a write giving a task a priority is refused, and `promote` clears a note's.

Read a long body from a file, or from stdin with `-`, instead of embedding it in the payload:

```
lym board upsert '{"slug":"my-task","title":"My Task","kind":"task","labels":["bug"]}' --body-file body.md
```

`merge` takes the same flag and sets the body of its `upsert` entry:

```
lym board merge '{"remove_slugs":["old"],"upsert":{"slug":"my-task","title":"My Task","kind":"task","labels":["bug"]}}' --body-file body.md
```

A merge carries the removed entries' issues onto the upserted entry, after the numbers it already records, whether or not the payload names `issues`.

Edit a body as a file: `get --body` prints the body alone, verbatim, so write it out, edit it and read it back in:

```
lym board get '{"slug":"my-task"}' --body > body.md
lym board upsert '{"slug":"my-task"}' --body-file body.md
```

Promote a note to a task:

```
lym board promote '{"slug":"my-note"}'
```

Rename an entry, keeping every other field and rewriting the old slug in every other entry's `depends_on`:

```
lym board rename '{"slug":"old-name","to":"new-name"}'
```

Prose is never rewritten: `mentions` lists the entries whose body or brief still name the old slug, so fix those by hand.
A claimed entry cannot be renamed.

Find entries whose slug, title, brief or body contains the text, done entries included; `--label` narrows to entries carrying every named label and repeats:

```
lym board find --text retry backoff
lym board find --label bug retry
```

`list` names the machine that owns the hub's prime: `prime_owner` in its JSON, and the first line of its `--text` output.
List every entry, with `--text` for a one-line-per-entry scan that opens with each entry's handle and shows its age and time since its last revision, `--sort age` or `--sort revised` to put the longest-lying entries first, and fetch one entry in full:

```
lym board list --text
lym board list --text --sort age
lym board list --label bug --label enhancement
lym board get '{"slug":"my-task"}'
```

Remove every done entry:

```
lym board prune --apply
```

## Claimed entries

An entry whose status reads `<state> · <producer>`, such as `running · Webster`, is claimed by a run.
A claimed entry takes no scope edits: a write that changes anything but its status, or removes it, is refused.
Record a new finding as a note of its own with `lym board upsert` and `"kind":"note"`.
The refusal names the way to unlock an abandoned run's entry.
Owners sit on root and beam projects: a task's owner is its beam's or root's, and it moves only through `lym adopt <beam>` or `lym adopt root`; an upsert naming another owner is refused.

## Tasks from a run session

A session whose `LYM_STRAND_NAME` carries a task slug under a role other than `orch` or a numbered orch role such as `orch-2` cannot create or promote a task: only the operator decides what becomes a task.
File the follow-up with `lym board upsert` and `"kind":"note"`; the operator may promote it.

## Intake

Intake takes the GitHub issues of the hub's own repository onto the board, the one list: the repository the hub's recorded binding names, else its origin remote.
It never reads the repository `selfreport` files LoomYard bugs to, unless that is the hub's own.
Run intake only when the operator asks for it.
Take each open issue onto the board, then close it:

```
lym board intake list
lym board intake list --full
lym board intake import '{"issue":12,"slug":"retry-backoff"}'
lym board intake import '{"issue":13,"into":"retry-backoff"}'
lym board intake close '{"issue":14,"reason":"Duplicate of #12."}'
```

- `list` prints the number and title of each open issue no entry records; `--full` adds body, labels, url, comment count and creation time.
- `import` with `slug` records the issue as a new note; with `into` it folds the issue into an existing entry. Either way it comments on the issue with a pointer and closes it.
  A new note gets its type and module labels through `import`'s `labels`.
  `import` prints the slug, issue, labels and dropped labels, not the entry; `lym board get` shows the entry.
  A label `upsert` on a folded entry carries the entry's whole label list, since `labels` replaces it.
- `close` ends a noise issue with a stated `reason` and writes nothing to the board.

## Writing a body

A `body` renders as ordinary markdown with semantic line breaks.
A single newline is a soft break, so structure a body with headings and lists rather than relying on line breaks for layout.
Never name another task as a dependency in a task's brief or body: put it in `depends_on` with `lym board set-deps`.
The README computes each task's After column and its Ready or layer subsection from `depends_on`, drops a finished entry from After, and `prune` strips it, while a dependency written as prose goes stale once that entry lands.
Whenever you set `depends_on`, check the other tasks waiting on the same entry for overlap, not only the running ones, and act on the `notice` warning `set-deps` and the upsert verbs print by chaining overlapping tasks with `lym board set-deps`.
A relation between notes has no `depends_on`, so it is a sentence in a body.
