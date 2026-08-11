# Plan: format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate

```yaml
task: 'format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate'
slug: 'format-docs-name-producers'
approved: false
started: '20260811-043750'
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
    name: contract-docs-producer-model
    file: 01-contract-docs-producer-model.md
    depends-on: []
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
  - number: 2
    name: producer-table-and-rename-sweep
    file: 02-producer-table-and-rename-sweep.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: docs-only-zero-go-changes

- **Decision:** no Go file, perch profile, rubric file, or test is written or edited by any card in this plan.
  The `Discussion-Validate` checks are *specified*, not implemented — implementation lands with `Shed`.
  Check 3's build-time assertion is likewise specified, not written.
- **Rationale:** `_mill/discussion.md`'s Scope "Out" excludes implementation outright, and `shed-followups.md:340–343` states the task is docs-only with no test surface of its own.
  A card that touches Go here is a scope violation, not a bonus.
- **Applies to:** all batches

### Decision: producer-names-are-validate-not-gate

- **Decision:** the landed producer `Plan-Review-Gate` is renamed to `Plan-Validate` everywhere it occurs outside `_mill/`, and the new Discussion-side mechanical producer is named `Discussion-Validate`.
  The superseded name `Discussion-Review-Gate` is swept only out of `manifest/roadmap.md:47`;
  all nine occurrences in `manifest/designs/shed-followups.md` keep their original wording.
- **Rationale:** `loom.md` overloads "gate" across two incompatible senses — perch's black-box review gate (sense A) and the cheap deterministic pre-check that runs before the LLM reviewer (sense B).
  Adding a second `-Gate` producer compounds the conflation;
  `-Validate` is verb-shaped, consistent with `Plan-Sweep`/`Plan-Write`, and frees "gate" to mean perch alone.
  `shed-followups.md` is a task-body archive, so sweeping it would falsify the record of what was specified — it gets an `**Override recorded**` note instead, following the convention already at `:289`, `:296`, `:441`, `:449`, `:462`, `:470`.
- **Applies to:** all batches

### Decision: producer-name-lives-in-the-table-not-in-the-contract-file

- **Decision:** the producer name `Discussion-Validate` is pinned **only** in `manifest/designs/loom.md`'s producer-table row.
  `docs/reference/discussion-format.md`'s validation section keeps a **neutral** heading — `## Validation checks (spec for the future validator)` — and never carries the producer name as a heading.
- **Rationale:** this is the shape that already landed on the Plan side: `plan-format.md:187` is a neutral heading and the producer name appears only in `loom.md:54`'s row.
  The contract file describes the artifact's checkable properties;
  the producer table names who runs them.
  Keeping the split means the contract file survives a later rename or split of the producer without an edit.
- **Applies to:** all batches

### Decision: pointer-rule-governs-every-new-cell-and-section

- **Decision:** every Input/Output cell and every new `## Producer and contract` section states a *pointer* to the format-contract file defining the consumed artifact's shape — never a restated or paraphrased copy of that file's content.
- **Rationale:** `shed.md:24–25` makes this the definition of a producer's contract, and the whole point of this task is that `loom.md`'s pointers currently name files that do not exist.
  A cell that inlines the schema instead of pointing at it reintroduces the drift the pointer rule exists to prevent.
- **Applies to:** all batches

### Decision: semantic-line-breaks-on-every-touched-line

- **Decision:** every line any card touches comes out conforming to `CLAUDE.md`'s markdown rule — one sentence per line, plus an internal break at an independent-clause boundary inside a long sentence, plain newlines only (never trailing double-spaces or a backslash).
  Table cells and blockquotes stay on one line.
- **Rationale:** `CLAUDE.md` applies the rule to every `.md` file in the repo, not only newly-written prose.
  A rewritten paragraph that keeps a fixed-column wrap is a defect the reviewer will raise.
- **Applies to:** all batches

### Decision: line-numbers-in-this-plan-are-pre-edit-anchors

- **Decision:** every `:NN` reference in a card's `Requirements:` names the line as it reads in the **pre-batch** file.
  Once an earlier card in the same batch has edited a file, later cards locate their target by the quoted text, not by the stale number.
- **Rationale:** cards 1–6 all edit `docs/reference/discussion-format.md` in sequence, so line numbers shift after card 1.
  Anchoring on quoted text is what keeps a Sonnet implementer from editing the wrong line.
- **Applies to:** all batches

### Decision: verify-is-the-markdown-link-integrity-test

- **Decision:** both batches use `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` as their `verify:` command.
  The repo-wide `go build ./...` / `go test ./...` assertion runs once via the configured `pipeline.done_gate`, not per batch.
- **Rationale:** this is the only machine check with a real surface for a docs-only task — `CONSTRAINTS.md`'s Markdown Link Integrity invariant scans every `.md` file under `manifest/` and `docs/`, which is exactly where all six edited files live.
  The full suite per batch would be pure cost for a task that touches zero Go.
- **Applies to:** all batches

## All Files Touched

- `docs/reference/discussion-format.md`
- `docs/reference/plan-format.md`
- `manifest/designs/loom.md`
- `manifest/designs/shed-followups.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
