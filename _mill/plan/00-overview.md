# Plan: loom: redesign the Discussion format

```yaml
task: 'loom: redesign the Discussion format'
slug: 'loom-redesign-discussion-format'
approved: false
started: '20260824-082547'
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
    name: discussion-format doc and manifest cleanup
    file: 01-discussion-format-doc-and-manifest-cleanup.md
    depends-on: []
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: markdown-only, no Go and no stencil edits

- **Decision:** Every card in this plan edits or creates a `.md` file under `manifest/` only.
  No Go source, no test file, and no file under `contracts/stencils/` is touched — in particular `contracts/stencils/loom/loom-template-discussion.md` is referenced in prose but never edited.
- **Rationale:** The stencil rewrite is the separate `loom: Discussion-Write producer` roadmap item (Wave 2), and `internal/discussionparser` checks only the seven required H2 headings, which this task does not change.
- **Applies to:** all batches

### Decision: single batch, because link integrity is only green at the end

- **Decision:** All six cards live in one batch with one `verify:` at the batch boundary.
- **Rationale:** `TestEnforcement_MarkdownLinks` resolves every inline link and `.md` `#anchor` under `manifest/` and `docs/`.
  Splitting the work would put a verify boundary at a point where a pointer already exists but its target doc or its target heading does not yet — a guaranteed red verify on a correct intermediate state.
  One batch keeps every verify run against a link-complete tree.
- **Applies to:** all batches

### Decision: the pinned heading text is an exact string, not an example

- **Decision:** The new `manifest/designs/loom.md` subsection heading is exactly `### Discussion-Review rubric — what to also flag (relocation and exclusion)`, em dash included, no rewording.
- **Rationale:** Two inbound links resolve against its derived anchor `discussion-review-rubric--what-to-also-flag-relocation-and-exclusion`.
  `docsLinkSlug` in `internal/lyxcwd/docslink_test.go` implements GitHub's rule — lowercase, backticks stripped, punctuation (including the em dash and the parentheses) dropped, spaces to hyphens — so any change to the heading's wording or punctuation silently breaks both links.
- **Applies to:** all batches

### Decision: semantic line breaks on every written or edited line

- **Decision:** One sentence per line;
  break inside a long sentence at an internal independent-clause boundary (a comma plus a coordinating conjunction, or a semicolon).
  Plain newlines only — never trailing double-spaces, never a backslash.
- **Rationale:** Project `CLAUDE.md`'s markdown convention, which binds every `.md` file in this repo, not only newly-written ones.
  Table cells and blockquotes stay on one line.
- **Applies to:** all batches

### Decision: reword the stale supersession sentences, never clause-drop them

- **Decision:** In `manifest/designs/plan-card-format.md` and `manifest/roadmap.md`, remove `contracts/stencils/loom/loom-template-discussion.md` from each "Supersedes …" sentence by rewriting the whole sentence so it reads grammatically, not by deleting the clause in place.
- **Rationale:** Both are joined-list sentences ("Supersedes X, Y, and Z" / "both X and Y outright");
  excising one list member without rewriting leaves broken prose, which the acceptance criteria explicitly reject.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `manifest/designs/discussion-format.md`
- `manifest/designs/loom.md`
- `manifest/designs/plan-card-format.md`
- `manifest/designs/review-finding-classification.md`
- `manifest/roadmap.md`
