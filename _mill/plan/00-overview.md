# Plan: Add typed file-ops to lyx's plan-format

```yaml
task: "Add typed file-ops to lyx's plan-format"
slug: "plan-format-file-ops"
approved: true
started: "20260712-064118"
parent: "internal-builder"
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: parser-v2
    file: 01-parser-v2.md
    depends-on: []
    verify: go test ./internal/builderengine/...
  - number: 2
    name: move-checks
    file: 02-move-checks.md
    depends-on: [1]
    verify: go test ./internal/builderengine/...
  - number: 3
    name: card-checks
    file: 03-card-checks.md
    depends-on: [2]
    verify: go test ./internal/builderengine/...
  - number: 4
    name: templates-help
    file: 04-templates-help.md
    depends-on: [1, 2, 3]
    verify: go test ./internal/builderengine/... ./internal/buildercli/...
  - number: 5
    name: docs
    file: 05-docs.md
    depends-on: [3]
    verify: null
```

## Shared Decisions

### Decision: v2-card-grammar

- **Decision:** Plan-format v2's card grammar, implemented identically by parser
  (batch 1), validator (batches 2-3), templates (batch 4), and doc (batch 5):
  - Card heading: `### Card NN.C — <short title>` where `NN` is the batch's
    zero-padded two-digit number (same as the filename prefix) and `C` restarts at 1
    within each batch. ASCII `-`/`--` accepted wherever the em dash `—` is (same
    tolerance as v1's Batch Index separator).
  - Card fields, in order: `**What:**` (prose, may span lines until the next field
    label), then `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`,
    `**Moves:**` (all five REQUIRED on every card), then optional `**Commit:**`,
    then optional `**verify:**` (value on the same line, as in v1).
  - A file-op field's value is either the literal `none` on the label line, or one
    or more subsequent lines of the form `` - `path` `` (backtick-wrapped, one path
    per bullet, no commentary, no line-range suffixes), terminated by the next
    `**Label:**` line, `### ` heading, or `## ` heading.
  - `Moves:` bullets use the two-path form `` - `old/path` -> `new/path` `` (ASCII
    ` -> ` separator).
  - `**Commit:**` value is backtick-wrapped: `` **Commit:** `NN.C: <short what>` ``.
  - Batch Index entry: `- NN — <batch-slug> (C cards) — <one-line intent>`; the
    `(C cards)` segment is mandatory (`(1 card)` singular accepted).
  - Batch frontmatter gains optional `root: <dir>`; `//`-prefixed paths are always
    worktree-root-relative.
- **Rationale:** one pinned grammar prevents parser/doc drift; every mechanical
  check depends on it (discussion `typed-card-fields`, `moves-grammar-and-rename-mechanic`,
  `card-numbering-batch-dot-card`, `per-card-commit-field`,
  `per-batch-root-path-shorthand`, `batch-index-card-counts`).
- **Applies to:** all batches

### Decision: lenient-card-parse

- **Decision:** `ParsePlan` stays fail-loud on document structure (missing/unterminated
  frontmatter, missing `## Batch Index` heading, unparseable index line — including a
  missing `(C cards)` segment, missing batch file, `verify: deferred` + `## verify:`
  conflict, glob in Scope). Card-level defects are NOT parse errors: a missing field is
  recorded as absent (nil slice / empty string), a `Moves:` bullet that fails the pair
  grammar is retained verbatim in `PlanCard.MovesRaw`, and a malformed path is stored
  normalized-best-effort. The validator turns these into findings (`card-missing-field`,
  `move-format`, `scope-malformed`).
- **Rationale:** findings-lists beat one-error-at-a-time discovery for a Planner fixing
  a plan; document-structure failures still stop everything per v1's fail-loud
  discipline (discussion `parser-shape`).
- **Applies to:** parser-v2, move-checks, card-checks

### Decision: normalize-at-parse

- **Decision:** Card path normalization happens once, in the parser:
  strip surrounding backticks, then `//x` -> `x` (worktree-root-relative, whether or
  not `root:` is set), otherwise join `<root>/<path>` when `root:` is non-empty. The
  result is a forward-slash worktree-relative path. `PlanCard`'s five field slices and
  `Moves` pairs hold ONLY normalized paths; validator, context estimate, and any
  future consumer never re-resolve. Absolute paths, `..` segments, and unclean paths
  are NOT rejected at parse time — the existing `scope-malformed` check (extended to
  card paths in batch 3) flags them via `scopeEntryMalformedReason`.
- **Rationale:** single normalization point; downstream code stays path-dialect-free
  (discussion `per-batch-root-path-shorthand`, `parser-shape`).
- **Applies to:** parser-v2, move-checks, card-checks

### Decision: findings-shape-unchanged

- **Decision:** `ValidationError{Check, Batch, Detail}` keeps its exact shape and
  `Error()` format. Card-level findings name the card inside `Detail` as
  `card NN.C` — no new struct field. New checks follow the existing deterministic
  ordering convention (check by check, batch order within a check, card order within
  a batch). Check names are pinned in the batch files; `scope-malformed` is reused
  for card-path well-formedness (per discussion `validator-check-set`).
- **Rationale:** minimal churn to `buildercli validate`'s JSON envelope and existing
  tests; the `NN.C` citation is what the numbering decision exists for.
- **Applies to:** move-checks, card-checks

### Decision: package-scoped-verify

- **Decision:** Every batch `verify:` in THIS plan is package-scoped
  (`go test ./internal/builderengine/...`, batch 4 adds `./internal/buildercli/...`).
  No batch runs `go test ./...`. Batch 5 (pure docs) has `verify: null`.
- **Rationale:** hard guardrail from the task and discussion
  (`verify-scope-guardrail`); v2 itself documents the same rule.
- **Applies to:** all batches

### Decision: fixture-self-reference

- **Decision:** v2 testdata fixtures keep v1's trick: card paths point at files that
  actually exist inside the fixture directory itself (the batch .md files), since
  tests pass the fixture dir as `worktreeRoot`. `Moves:` pairs in `plan-valid` use an
  existing fixture file as source and a non-existent path as target, and
  `Creates:`/`Deletes:` entries are chosen so that `plan-valid` stays at ZERO findings
  through every check added in batches 2 and 3 (design the fixture once, in batch 1,
  with those future checks in mind).
- **Rationale:** no extra scaffolding files; `plan-valid` remains the running
  zero-findings regression anchor while checks accumulate.
- **Applies to:** parser-v2, move-checks, card-checks

### Decision: mill-is-precedent-only

- **Decision:** Nothing in lyx references or reads the mill repo. The canonical
  Rename-mechanic text, field grammar, and check semantics are fully specified in
  `_mill/discussion.md` and this plan; implementers never consult mill sources.
- **Rationale:** discussion `no-all-files-touched` / Technical context — mill paths
  are not resolvable from this worktree; the plan must be executable cold.
- **Applies to:** all batches

## All Files Touched

- `docs/modules/builder.md`
- `docs/modules/plan-format.md`
- `docs/overview.md`
- `internal/buildercli/cli.go`
- `internal/buildercli/validate.go`
- `internal/builderengine/doc.go`
- `internal/builderengine/fingerprint.go`
- `internal/builderengine/implementer-template.md`
- `internal/builderengine/orchestrator-template.md`
- `internal/builderengine/plan.go`
- `internal/builderengine/plan_test.go`
- `internal/builderengine/template_test.go`
- `internal/builderengine/testdata/plan-broken-chain/00-overview.md`
- `internal/builderengine/testdata/plan-broken-chain/01-first.md`
- `internal/builderengine/testdata/plan-broken-chain/02-second.md`
- `internal/builderengine/testdata/plan-unapproved/00-overview.md`
- `internal/builderengine/testdata/plan-unapproved/01-only.md`
- `internal/builderengine/testdata/plan-valid/00-overview.md`
- `internal/builderengine/testdata/plan-valid/01-json-flag.md`
- `internal/builderengine/testdata/plan-valid/02-list-tests.md`
- `internal/builderengine/testdata/plan-valid/03-refactor-a.md`
- `internal/builderengine/testdata/plan-valid/04-refactor-b.md`
- `internal/builderengine/testdata/plan-valid/05-oversized.md`
- `internal/builderengine/validate.go`
- `internal/builderengine/validate_test.go`
