# Plan: Reed and Fabric as standalone modules: public API design

```yaml
task: 'Reed and Fabric as standalone modules: public API design'
slug: 'reed-fabric-standalone-api-design'
approved: false
started: '20260906-125124'
parent: 'main'
root: ""
verify: null
discussion_sha: '6973397e4b434e1585f993b012456024a889ebcd'
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shared-frame-and-reed
    file: 01-shared-frame-and-reed.md
    depends-on: []
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
  - number: 2
    name: fabric-creel-and-close
    file: 02-fabric-creel-and-close.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the-deliverable-is-one-markdown-file-and-no-go-code

- **Decision:** every card in this plan writes into the single new file `manifest/designs/reed-fabric-standalone-api.md`.
  No `.go` file is created, edited, moved or deleted by any card, and no test file is added.
- **Rationale:** `_mill/discussion.md`'s Scope names one design document as the whole deliverable, and its Testing section states there is no Go code in the task and therefore no unit test to write.
  A card that touches a `.go` file is a plan defect, not an implementation choice.
- **Applies to:** all batches

### Decision: discussion-md-is-the-source-of-measurements-not-a-re-measurement-brief

- **Decision:** every quantitative claim the doc states is transcribed from `_mill/discussion.md`'s `## Technical context` section, together with the producing command recorded there.
  Cards do not re-run the measurements and do not substitute freshly derived numbers.
- **Rationale:** `_mill/discussion.md` records that everything in that section was measured in this worktree at `ca388c0ce` with `go list`, `go doc` and `grep`, and it went through five discussion-review rounds.
  Re-deriving a number in a later worktree state would silently desynchronise the doc from the evidence base that was reviewed, and several of the discussion's own first-draft figures were wrong for a documented reason.
  Where a card wants a fact the discussion does not carry, the card's `Context:` names the file to read.
- **Applies to:** all batches

### Decision: every-number-carries-its-producing-command

- **Decision:** no quantitative claim appears in the doc without the command that produced it, stated inline or in an adjacent fenced block.
  The doc also carries, once, the comment-prose contamination caveat: a naive `grep` over a package name matches doc-comment prose as well as code, and two figures in this task's own first draft were wrong for exactly that reason.
- **Rationale:** `_mill/discussion.md`'s "Discovered during exploration" bullet makes reproducibility the condition of the doc's value, since several published background claims were measurably false.
- **Applies to:** all batches

### Decision: markdown-conventions-semantic-line-breaks-and-resolving-links

- **Decision:** the doc is written with semantic line breaks — one sentence per line, plus a break at an internal independent-clause boundary — never fixed-column hard-wrap, never trailing double-spaces or a backslash.
  Table cells and blockquotes stay on one line.
  Every repo-relative markdown link the doc adds must resolve, file part and `#anchor` alike, and no entry may be added to `docsLinkAllowlist`.
- **Rationale:** `CLAUDE.md`'s markdown rule and `CONSTRAINTS.md`'s Markdown Link Integrity invariant, the latter enforced by `TestEnforcement_MarkdownLinks`, which is this plan's `verify:` command for both batches.
  External `http`/`https`/`mailto` targets are skipped by that test, so any external URL is a manual-review item.
- **Applies to:** all batches

### Decision: heading-style-follows-the-prevailing-manifest-designs-shape

- **Decision:** the doc opens with a single H1 naming the subject, uses `##` for each major section, and closes with `## Open questions` and `## Related`, matching the prevailing shape in `manifest/designs/`.
- **Rationale:** `_mill/discussion.md`'s "Where things live" section names `loom.md`, `hardener.md` and `semantic-index.md` as the style reference and lists that heading vocabulary.
- **Applies to:** all batches

### Decision: fabric-vocabulary-and-the-no-constraints-edit-rule

- **Decision:** the doc uses *Fabric* for the wired composite and says *warp* / *weft* only where the two sides must be told apart;
  it never substitutes a bare "repo" for warp.
  No card edits `CONSTRAINTS.md`, and no recommendation the doc makes may require a `CONSTRAINTS.md` amendment.
- **Rationale:** the Fabric Vocabulary Invariant binds every non-owner document, and `_mill/discussion.md`'s Scope explicitly bars CONSTRAINTS.md changes.
  This is what scopes the `logger` decoupling recommendation to the standalone case only.
- **Applies to:** all batches

### Decision: no-roadmap-entry-and-no-module-scaffolding

- **Decision:** no card edits `manifest/roadmap.md`, and no card adds a module directory, a `configreg` entry, or a CLI verb for `creel`.
  The doc names recommendations and a concept;
  promoting either is a separate decision afterwards.
- **Rationale:** `_mill/discussion.md`'s Scope and its `doc-belongs-in-manifest-designs-despite-shipped-modules` decision, plus `CLAUDE.md`'s rule that the roadmap moves only on completing or adding a planned item.
- **Applies to:** all batches

### Decision: verify-is-the-link-integrity-test-and-nothing-wider

- **Decision:** both batches use `verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks`.
  The repo-wide regression check is left to `pipeline.done_gate`, which is already configured in `mill-config.yaml` as `go test ./... && go test -tags integration ./...` and is not modified by this plan.
- **Rationale:** the only mechanical check a markdown-only change can fail is link integrity, and it is the check `_mill/discussion.md`'s Testing section names first.
  Running the repo-wide suite after every implementer round would cost minutes per round for a change that touches no `.go` file, and the done gate already covers the "an accidental code edit slipped in" case once, at the end.
- **Applies to:** all batches

## All Files Touched

- `manifest/designs/reed-fabric-standalone-api.md`
