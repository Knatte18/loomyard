# Batch: inbound-refs-nondoc

```yaml
task: 'loom: pin the spawn/handover status schema + discussion-format.md'
batch: inbound-refs-nondoc
number: 4
cards: 4
verify: 'bash -c "go build ./... && ! grep -rIn --exclude-dir=_mill --exclude-dir=.git --exclude-dir=.lyx -e modules/plan-format.md -e modules/builder-contract.md internal tools/sandbox docs/reviews docs/reference/model-spec.md"'
depends-on: [3]
```

## Batch Scope

Retarget every non-narrative inbound reference to the two relocated docs: Go godoc comments, the
implementer template, the sandbox suite doc, the builder-review prompt, and `model-spec.md`'s
relative link. All are comment/string-only edits (Decision `spec-only-no-functional-go`).
Depends on batch 3 (the files must already live at `docs/reference/`). The narrative docs
(`loom.md`, `overview.md`, `roadmap.md`, `long-term-ideas.md`) are handled in batch 5 to keep
their content reconciliation and their inbound links in one place.

## Cards

### Card 5: Retarget plan-format.md path in Go godoc comments

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/buildercli/cli.go`
  - `internal/builderengine/validate.go`
  - `internal/builderengine/report.go`
  - `internal/builderengine/doc.go`
  - `internal/builderengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each listed file, replace the full-path token `docs/modules/plan-format.md`
  with `docs/reference/plan-format.md` inside godoc comments. Known sites:
  `hubgeometry.go` (2 comments, near lines 223 and 247), `cli.go` (the `Long:` help string near
  line 117), `validate.go` (line 2), `report.go` (line 3), `doc.go` (line 3),
  `template_test.go` (line 285). Do NOT touch bare-filename mentions that encode no path (e.g.
  "plan-format.md pins", "plan-format v2") — only the `docs/modules/plan-format.md` full path
  changes. Change nothing else; no functional Go changes.
- **Commit:** `docs(builder): retarget plan-format.md path in godoc comments`

### Card 6: Retarget doc paths in implementer template and sandbox suite

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/implementer-template.md`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace full-path tokens `docs/modules/plan-format.md` →
  `docs/reference/plan-format.md` and `docs/modules/builder-contract.md` →
  `docs/reference/builder-contract.md`. Known sites: `implementer-template.md` (line 2,
  plan-format); `SANDBOX-BUILDER-SUITE.md` (line 12 builder-contract, line 60 plan-format,
  line 149 builder-contract). Bare-filename mentions unchanged; no other edits.
- **Commit:** `docs(sandbox): retarget contract-doc paths after relocation`

### Card 7: Retarget doc paths in builder-review-prompt.md

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/reviews/builder-review-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace every full-path token `docs/modules/plan-format.md` →
  `docs/reference/plan-format.md` and `docs/modules/builder-contract.md` →
  `docs/reference/builder-contract.md` throughout the file (8 occurrences, near lines 47, 50, 61,
  62, 68, 150, 244, 452). Leave bare-filename mentions and links to other docs untouched.
- **Commit:** `docs(reviews): retarget contract-doc paths in builder-review-prompt`

### Card 8: Retarget model-spec.md relative link to plan-format.md

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/reference/model-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/reference/model-spec.md` (line 5), the link
  `[plan-format v2](../modules/plan-format.md)` must become `[plan-format v2](plan-format.md)` —
  `plan-format.md` is now a sibling of `model-spec.md` in `docs/reference/`. Change only that
  link target; leave the link text and all other content unchanged.
- **Commit:** `docs(model-spec): fix plan-format link after relocation`

## Batch Tests

`verify: 'bash -c "go build ./... && ! grep -rIn --exclude-dir=_mill --exclude-dir=.git --exclude-dir=.lyx -e modules/plan-format.md -e modules/builder-contract.md internal tools/sandbox docs/reviews docs/reference/model-spec.md"'`
— `go build ./...` confirms the godoc-comment edits did not break compilation (Decision
`spec-only-no-functional-go`); the negated grep confirms no `modules/plan-format.md` or
`modules/builder-contract.md` path token survives in any file this batch edits (`internal/`,
`tools/sandbox/`, `docs/reviews/`, `docs/reference/model-spec.md`). Bare-filename mentions never
match the `modules/…` pattern and are correctly ignored. The unbounded directory grep is justified
here because the retargeting spans many files across several trees; scoping it to the edited trees
keeps it cheap.
