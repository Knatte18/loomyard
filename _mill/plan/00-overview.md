# Plan: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
task: 'shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions'
slug: 'shed-producer-typology-sweep'
approved: true
started: '20260811-150036'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shed-md-and-constraints
    file: 01-shed-md-and-constraints.md
    depends-on: []
    verify: go test ./internal/lyxcwd
  - number: 2
    name: loom-md-pointers-and-kind-column
    file: 02-loom-md-pointers-and-kind-column.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd
  - number: 3
    name: roadmap-overview-hardener-followups
    file: 03-roadmap-overview-hardener-followups.md
    depends-on: [2]
    verify: go test ./internal/lyxcwd
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: docs-only-no-go-source

- **Decision:** this task changes Markdown only.
  No Go file is created, edited, renamed, or deleted;
  no package is added or removed;
  no test file is written.
  In particular `internal/loomengine/coherence.go`'s `validPhases` map and `docs/reference/status-schema.md`'s matching phase enum stay byte-identical — the flat producer list replaces that enum rather than editing it, and the realignment lands with the `Shed` build task.
- **Rationale:** the task's whole deliverable is reconciling design-doc text to a decision already recorded on `manifest/roadmap.md`.
  Any code change would be out of scope and would break the "no interim phase set" reasoning the deferral record itself states.
- **Applies to:** all batches

### Decision: roadmap-wording-is-the-source

- **Decision:** `manifest/roadmap.md` lines 57-61 carry the recorded resolution of the atomicity precondition, and their wording is the source for every new design-doc sentence.
  Use its exact terms — "simple, single-agent-spawn producer" and "bespoke, multi-spawn producer" — never a synonym such as "atomic producer", "composite producer", "leaf producer", or "loop-owning producer".
  Name the shared `Shed`-level "LLM-Producer" type only as a **candidate**, never as a decided design.
  Those roadmap lines are themselves **verify-only**: read them, do not rewrite them.
- **Rationale:** the roadmap is the recorded source of the 2026-08-11 resolution.
  Introducing a synonym in the design docs makes the two disagree in vocabulary while agreeing in substance, which is the exact drift this sweep exists to remove.
- **Applies to:** all batches

### Decision: shed-md-is-authoritative-loom-md-points

- **Decision:** the full producer-typology text lives in `manifest/designs/shed.md`'s `### Producer contract vs. producer definition` section. `manifest/designs/loom.md` never restates it — it links to `shed.md#producer-contract-vs-producer-definition` instead.
  The anchor appears in `loom.md` in exactly four sentences, each making its own distinct claim: the atomicity sentence that currently sits at `loom.md:44` (card 7a), the sentence introducing the producer table (card 7b), the two-part-contract sentence at `loom.md:70` (card 9a), and the pointer-rule sentence at `loom.md:71` (card 9b).
  The rule that actually binds is not a link count but this: the anchor never appears inside a table cell, is never repeated per row, and never accompanies a restatement of the text it points at.
  Twelve identical per-row links would violate that rule;
  four distinct sentences each replacing a restatement with a pointer are the rule being obeyed.
- **Rationale:** `shed.md:3` declares itself "the authoritative description of `Shed`'s own generic mechanism" and `loom.md:43` already defers to it for the mechanism, owning only `loom`'s concrete list.
  The typology is a property of `Shed`'s generic contract.
  This is also the pointer rule applied to the docs themselves — the same discipline this task codifies in `CONSTRAINTS.md`.
- **Applies to:** all batches

### Decision: no-heading-renames-anchors-are-load-bearing

- **Decision:** no batch renames, adds, or removes a Markdown heading in any edited file.
  Every edit is prose, table, or list content under an existing heading.
  The anchors that must stay valid are `shed.md#producer-contract-vs-producer-definition`, `loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots`, `loom.md#the-gate`, `loom.md#webster--a-black-box-loom-drives-the-sibling-of-perch`, and `loom.md#module-decomposition`.
  The single exception is `CONSTRAINTS.md`, which gains one new `## Producer Pointer-Rule Invariant` heading and renames nothing.
- **Rationale:** `TestEnforcement_MarkdownLinks` in `internal/lyxcwd/docslink_test.go` resolves the `#anchor` of every `.md` link under `manifest/` and `docs/`, so renaming a heading breaks inbound links repo-wide.
  This task adds new cross-doc anchor links, which makes it the most exposed task in the chain to that failure.
- **Applies to:** all batches

### Decision: semantic-line-breaks

- **Decision:** every Markdown line this task writes or edits follows `CLAUDE.md`'s semantic-line-break rule — one sentence per line, with an extra soft break inside a long sentence only at an internal independent-clause boundary (a comma plus a coordinating conjunction, or a semicolon, where what follows has its own subject and verb).
  Never a fixed-column hard wrap, never a trailing double-space, never a trailing backslash.
  Table cells and blockquotes stay on one line each.
- **Rationale:** the rule binds every `.md` file in the repo, including lines merely edited inside an existing paragraph.
  A fixed-column rewrap would make every later edit in the paragraph touch every wrapped line.
- **Applies to:** all batches

### Decision: shed-followups-is-grep-exempt

- **Decision:** `manifest/designs/shed-followups.md` is a permanently grep-exempt historical record.
  Its pre-rename paths, stale citations, and `Discussion-Review-Gate` naming are deliberate and must not be "fixed".
  The only edit this task makes to it is one appended supersession block at the head of section E (card 17).
  Every acceptance grep excludes it, as does `docs/reference/status-schema.md`, which carries the out-of-scope phase enum.
- **Rationale:** the file's own `**Override recorded 2026-08-09 (task B, as landed)**` note at lines 227-231 records the convention, and tasks A, B and C each amended it by appended block rather than by body edit.
  An acceptance grep must never be satisfiable only by editing text this task declares out of scope.
- **Applies to:** all batches

### Decision: use-live-producer-names

- **Decision:** all new text uses the landed producer names — `Discussion-Validate` and `Plan-Validate`, never `Discussion-Review-Gate` or `Plan-Review-Gate` — and names `internal/treadleengine` as the current home of the round loop formerly described as `perchengine`'s.
- **Rationale:** task C landed the mechanical pre-checks under the `*-Validate` names precisely to free the word "gate" for `perch` alone, and `roadmap.md:60` already names `internal/treadleengine` as the round loop's home.
  Both older names survive in `shed-followups.md` as historical record only.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `docs/overview.md`
- `manifest/designs/hardener.md`
- `manifest/designs/loom.md`
- `manifest/designs/shed-followups.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
