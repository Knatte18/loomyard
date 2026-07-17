# Plan: loom: pin the spawn/handover status schema + discussion-format.md

```yaml
task: 'loom: pin the spawn/handover status schema + discussion-format.md'
slug: loom-contracts
approved: false
started: 20260717-111021
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: status-schema-doc
    file: 01-status-schema-doc.md
    depends-on: []
    verify: test -f docs/reference/status-schema.md
  - number: 2
    name: discussion-format-doc
    file: 02-discussion-format-doc.md
    depends-on: []
    verify: test -f docs/reference/discussion-format.md
  - number: 3
    name: relocate-contracts
    file: 03-relocate-contracts.md
    depends-on: []
    verify: 'bash -c "test -f docs/reference/plan-format.md && test -f docs/reference/builder-contract.md && test ! -e docs/modules/plan-format.md && test ! -e docs/modules/builder-contract.md"'
  - number: 4
    name: inbound-refs-nondoc
    file: 04-inbound-refs-nondoc.md
    depends-on: [3]
    verify: 'bash -c "go build ./... && ! grep -rIn --exclude-dir=_mill --exclude-dir=.git --exclude-dir=.lyx -e modules/plan-format.md -e modules/builder-contract.md internal tools/sandbox docs/reviews docs/reference/model-spec.md"'
  - number: 5
    name: reconcile-narrative
    file: 05-reconcile-narrative.md
    depends-on: [1, 2, 3, 4]
    verify: 'bash -c "! grep -rIn --exclude-dir=_mill --exclude-dir=.git --exclude-dir=.lyx -e modules/plan-format.md -e modules/builder-contract.md . && ! ( grep -n -e plan-format.md -e builder-contract.md docs/modules/loom.md | grep -v reference )"'
```

## Shared Decisions

### Decision: spec-only-no-functional-go

- **Decision:** This task writes and moves documentation only. The sole edits to `*.go` files
  are inside **godoc comments** (retargeting a moved doc's path); no types, logic, behavior, or
  APIs change. `go build ./...` (batch 4 verify) exists purely to confirm a comment edit did not
  break compilation — it is a safety net, not a feature gate.
- **Rationale:** "No Go code = no *functional* Go" (discussion Constraints). Editing a doc-path
  string in a comment is not writing Go code.
- **Applies to:** all batches

### Decision: retarget-mapping

- **Decision:** Every reference that encodes the OLD location of the two relocated contract docs
  becomes the new `docs/reference/…` location. Mapping by context:
  - Full-path `docs/modules/plan-format.md` → `docs/reference/plan-format.md`;
    `docs/modules/builder-contract.md` → `docs/reference/builder-contract.md`.
  - Relative `modules/<x>.md` (from a doc at `docs/` root) → `reference/<x>.md`.
  - Relative `../modules/<x>.md` (from a doc in `docs/reference/`) → `<x>.md` (now a sibling).
  - Same-folder sibling `(<x>.md)` inside `docs/modules/loom.md` (loom.md stays put) →
    `(../reference/<x>.md)`; if the link's display TEXT also reads `modules/<x>.md`, retext it to
    `<x>.md` so no `modules/<x>.md` token survives.
  - **Left untouched:** bare-filename mentions that encode no path (e.g. a Go comment reading
    "plan-format.md pins …"), links to docs that are NOT moving (`modules/loom.md`,
    `modules/hardener.md`, `../overview.md`), and the two moved files' mutual sibling link
    (`plan-format.md` ↔ `builder-contract.md`, both now in `docs/reference/`).
- **Rationale:** discussion Scope + Testing (link integrity, both link forms).
- **Applies to:** relocate-contracts, inbound-refs-nondoc, reconcile-narrative

### Decision: contract-doc-house-style

- **Decision:** The two new docs (`status-schema.md`, `discussion-format.md`) follow the
  house style of the existing pinned contracts (`plan-format.md`, `builder-contract.md`): a
  status/scope blurb up top, the pinned shape, per-field/section notes, a **short**
  validation-check list, and a **compact** worked example — deliberately lighter than
  plan-format's 18-check / full-example depth (discussion `doc-rigor-moderate`).
- **Rationale:** These pin real contracts a future validator/consumer honours, but are smaller
  than plan-format.
- **Applies to:** status-schema-doc, discussion-format-doc

## All Files Touched

- `docs/long-term-ideas.md`
- `docs/modules/loom.md`
- `docs/overview.md`
- `docs/reference/builder-contract.md`
- `docs/reference/discussion-format.md`
- `docs/reference/model-spec.md`
- `docs/reference/plan-format.md`
- `docs/reference/status-schema.md`
- `docs/reviews/builder-review-prompt.md`
- `docs/roadmap.md`
- `internal/buildercli/cli.go`
- `internal/builderengine/doc.go`
- `internal/builderengine/implementer-template.md`
- `internal/builderengine/report.go`
- `internal/builderengine/template_test.go`
- `internal/builderengine/validate.go`
- `internal/hubgeometry/hubgeometry.go`
- `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
