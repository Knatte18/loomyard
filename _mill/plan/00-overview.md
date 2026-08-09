# Plan: Scope the Shed producer-model rewrite into buildable tasks

```yaml
task: "Scope the Shed producer-model rewrite into buildable tasks"
slug: shed-producer-model-scoping
approved: false
started: "20260809-060437"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: code-task-bodies
    file: 01-code-task-bodies.md
    depends-on: []
    verify: null
  - number: 2
    name: docs-task-bodies
    file: 02-docs-task-bodies.md
    depends-on: []
    verify: null
  - number: 3
    name: wiki-publish-and-summary
    file: 03-wiki-publish-and-summary.md
    depends-on: [1, 2]
    verify: test -f /home/knatte/Code/loomyard/wiki/proposal-builder-retire.md && test -f /home/knatte/Code/loomyard/wiki/proposal-plan-format-drop-v3-suffix.md && test -f /home/knatte/Code/loomyard/wiki/proposal-format-docs-name-producers.md && test -f /home/knatte/Code/loomyard/wiki/proposal-raddle-finalize-fold-and-link-repair.md && test -f /home/knatte/Code/loomyard/wiki/proposal-shed-model-contradiction-sweep.md && test -f /home/knatte/Code/loomyard/wiki/proposal-batcher-standalone-split.md
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the deliverable is six task bodies, staged as files first, published to the wiki last

- **Decision:** each of the six follow-up tasks is authored as one staged file under `_mill/followup/`, named `<LETTER>-<slug>.md`.
  Batch 3 reads those six files and publishes them to the wiki in a single `wiki._client.upsert_tasks_batch` call.
  No batch calls `upsert_task` per task, and no batch writes wiki files directly.
- **Rationale:** the wiki daemon owns the wiki repo and serializes every write (CLAUDE.md, `## Mill wiki — never touched directly`), so the bodies cannot be authored in place.
  Staging them as ordinary files makes the authoring reviewable by mill-go's own code review before it reaches the wiki, and makes a re-run idempotent — the upsert is a pure function of the six files.
  A single batch call also gives the wiki one commit instead of six, and lets the six tasks reference each other in `depends_on` without ordering constraints (`Store.upsert_tasks_batch` validates against a projected snapshot containing all six).
- **Applies to:** all batches

### Decision: staged-file format — one fenced yaml header, then the proposal body verbatim

- **Decision:** every file under `_mill/followup/` starts with a fenced ` ```yaml ` block as its first non-empty content, carrying exactly four keys — `slug`, `title`, `brief`, `depends_on` — and everything after that block's closing fence is the task body, published verbatim as the wiki's `proposal-<slug>.md`.
  `title` is a double-quoted scalar (every title contains a colon).
  `brief` is a `|` block scalar, one paragraph, no bullet lists.
  `depends_on` is a flow-style list of slug strings, `[]` when empty.
- **Rationale:** the wiki client takes `title`, `brief`, `body`, and `depends_on` as separate fields, and `_render.py` writes `body` into `proposal-<slug>.md` unaltered.
  Splitting the metadata into one machine-parsable header keeps batch 3's publish step a mechanical parse rather than a judgement call, and keeps the body free of mill-specific frontmatter that would leak into the wiki page.
- **Applies to:** all batches

### Decision: body shape follows the existing proposals in the wiki

- **Decision:** each body opens with an H1 identical to the header's `title`, then `## Why`, `## What needs to happen`, `## Scope`, `## Sequencing`, `## Acceptance`.
  `## What needs to happen` is a numbered list; the per-file inventories sit as sub-bullets under their step.
- **Rationale:** the wiki's existing proposal pages (`proposal-scout-seam-conversion.md`, `proposal-fabric-host-to-warp-rename.md`, and the rest) already use the `# title` / `## Why` / `## What needs to happen` / `## Scope` shape, and a reader moving between them should not have to re-learn the layout.
  `## Sequencing` and `## Acceptance` are added because this set has real `depends_on` wiring and per-task acceptance criteria that the discussion already decided.
- **Applies to:** all batches

### Decision: bodies transcribe the discussion, they do not re-derive it

- **Decision:** every inventory, ownership boundary, rejected alternative, and acceptance criterion in a body is transcribed from `_mill/discussion.md`.
  The implementer does not open the repository files the inventories name, does not re-check the line numbers cited, and does not add sites the discussion did not name.
- **Rationale:** the discussion is the decided state — it went through five review rounds, and each of its inventories carries an explicit instruction about whether the receiving task re-derives its own list (B re-derives by grep; D and E re-read their files end to end).
  Re-verifying line numbers here would duplicate work each follow-up task is already told to do for itself, would put dozens of unrelated repository files into card `Context:` lists, and would risk a body that silently disagrees with the discussion it is supposed to carry.
- **Applies to:** all batches

### Decision: this task changes no repository content outside `_mill/`

- **Decision:** no card edits `manifest/`, `docs/`, `CONSTRAINTS.md`, `README.md`, or any Go file.
  The pointer-rule invariant, the roadmap edits, and every doc repair named in the discussion belong to the six follow-up tasks, not to this plan.
- **Rationale:** the discussion's Scope section puts every one of those edits explicitly out of scope for this task and assigns each to a lettered follow-up.
  Touching one here would give that file two owners — exactly the collision the discussion serialized task E to avoid.
- **Applies to:** all batches

### Decision: markdown follows the repo's semantic-line-break rule

- **Decision:** the staged files and the summary doc use one sentence per line, with additional breaks at internal independent-clause boundaries.
  No fixed-column hard wrap, no trailing double-space, no backslash line continuations.
- **Rationale:** CLAUDE.md's `## Markdown: semantic line breaks` rule applies to every `.md` file in this repo, and these files are committed here even though the bodies are also published to the wiki.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `_mill/followup-tasks.md`
- `_mill/followup/A-builder-retire.md`
- `_mill/followup/B-plan-format-drop-v3-suffix.md`
- `_mill/followup/C-format-docs-name-producers.md`
- `_mill/followup/D-raddle-finalize-fold-and-link-repair.md`
- `_mill/followup/E-shed-model-contradiction-sweep.md`
- `_mill/followup/F-batcher-standalone-split.md`
