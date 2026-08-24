# Plan: Migrate planparser.Card to Edits/Uses fields

```yaml
task: Migrate planparser.Card to Edits/Uses fields
slug: planparser-card-format-migration
approved: false
started: 20260824-124808
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: planparser core — format-4 model, classifier, parser, validator
    file: 01-planparser-core.md
    depends-on: []
    verify: go build ./...
  - number: 2
    name: planparser tests — golden fixture and per-check suite
    file: 02-planparser-tests.md
    depends-on: [1]
    verify: go test ./internal/planparser/...
  - number: 3
    name: consumer fixtures — websterengine, loomshed, loomrecipe, loomcli, webstercli
    file: 03-consumer-fixtures.md
    depends-on: [2]
    verify: go test -tags integration ./internal/websterengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/loomcli/... ./internal/webstercli/... ./internal/batcher/...
  - number: 4
    name: docs — spec rewrite, stencils, sandbox suite, stale figures, roadmap
    file: 04-docs-spec-stencils.md
    depends-on: [3]
    verify: go test ./internal/loomengine/... ./internal/webstercli/... ./internal/planparser/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: compile-atomic-first-batch

- **Decision:** batch 1 carries every non-test Go change in one batch — `internal/planparser`'s five sources plus `internal/websterengine/render.go` — and its `verify:` is `go build ./...`, not a test run.
  Batch 2 restores `go test ./internal/planparser/...`;
  batch 3 restores the remaining packages' test compilation.
- **Rationale:** removing `Card.ContextFiles`/`EditsFiles`/`CreatesFiles`/`DeletesFiles`/`Moves`/`DependsOn`/`HasWhat` and renaming `Card.Intent` → `Card.Summary` breaks compilation in every file that reads them simultaneously.
  There is no ordering of those edits that keeps `go build ./...` green partway through, so the batch boundary is drawn where the compiler's own atomicity is.
  Test files do not participate in `go build ./...`, which is what makes batches 2 and 3 separable at all.
- **Applies to:** all batches

### Decision: shape-classification-at-validation

- **Decision:** symbol-vs-path classification is one pure function in `internal/planparser/classify.go`.
  It is called by `normalizeCard` and by four validator checks (`card-path-malformed`, `path-missing`, `prosa-symbol-target`, and the two union helpers), never at parse time, and it never spawns a process or stats the disk.
- **Rationale:** the Test Tier Purity Invariant bars a process spawn from tier1, and the Planparser Sole-Parser Invariant keeps `planparser` a leaf.
  Classifying at parse time would bake a heuristic's misjudgement permanently into the model;
  classifying at validation keeps the parser dumb, matching `doc.go`'s existing lenient-card-parse decision.
- **Applies to:** all batches

### Decision: retired-labels-stay-recognized

- **Decision:** the eight format-3 card labels — `**What:**`, `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`, `**Depends-on:**`, and the lowercase `**verify:**` — stay in `cardLabels` and route into a new `Card.RetiredLabels` slot.
  Deleting them from `cardLabels` is banned.
- **Rationale:** `parseCardBody`'s fallthrough is `default: i++`, so a label absent from `cardLabels` is silently discarded, and a stray one is swallowed into `**Intent:**`'s collect-until-next-label prose.
  A half-migrated card would then parse clean and validate clean while carrying instructions nobody reads — the exact silent misparse this format's fail-loud discipline exists to prevent.
- **Applies to:** planparser core, planparser tests

### Decision: no-behavior-change-in-webster

- **Decision:** no scheduler, no dependency graph, no topological sort appears anywhere in this task's diff.
  Cards still execute in strict declared plan order.
  `internal/websterengine`'s `HasSymbolFields()` seam stays dead;
  only its doc comment is corrected.
- **Rationale:** the roadmap entry rules this out explicitly, and Wave 3's `webster: DAG-derived card sequencing` item owns it.
  A reviewer must be able to confirm it by inspection.
- **Applies to:** all batches

### Decision: sixteen-checks-counted-by-id

- **Decision:** the post-migration validator check count is **16**, counted as distinct `ValidationError.Check` IDs, and the rewritten spec's numbered list carries **one row per ID** — `format-unrecognized` and `plan-unapproved` unbundled into separate rows.
- **Rationale:** the repo's existing "14" is the spec's numbered-**row** count (row 1 bundles two IDs), while the code emits 15 distinct IDs today.
  A finding's ID is what a consumer greps for and what `ValidationError.Check` carries;
  a row is a presentation choice.
  Keeping a 15-row list under a "16 checks" banner would recreate the same divergence this task resolves.
- **Applies to:** planparser core, docs

### Decision: go-verify-no-python-prefix

- **Decision:** every `verify:` command in this plan is a bare `go` invocation with no `PYTHONPATH= ` prefix, and is written in the plain-string form (implying `cwd: git_root`).
- **Rationale:** this is a Go repository, not a Python one — the `verify-not-isolated` validator check applies the `PYTHONPATH= ` rule only to Python projects.
  `_paths.resolve_hub_path()` and `_paths.resolve_git_root()` resolve to the same worktree root here, so there is no nested layout and no reason for the `{cwd, command}` mapping form.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `contracts/specs/loom-plan-spec.md`
- `contracts/stencils/loom/loom-template-plan.md`
- `contracts/stencils/webster/webster-body-implementer.md`
- `internal/loomcli/validate_test.go`
- `internal/loomengine/plan_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomshed/planvalidate_test.go`
- `internal/planparser/classify.go`
- `internal/planparser/classify_test.go`
- `internal/planparser/doc.go`
- `internal/planparser/normalize.go`
- `internal/planparser/normalize_test.go`
- `internal/planparser/parse.go`
- `internal/planparser/parse_test.go`
- `internal/planparser/plan.go`
- `internal/planparser/sections_test.go`
- `internal/planparser/testdata/goodplan/00-overview.md`
- `internal/planparser/testdata/goodplan/01-json-row-type.md`
- `internal/planparser/testdata/goodplan/02-json-flag.md`
- `internal/planparser/testdata/goodplan/03-json-emission.md`
- `internal/planparser/testdata/goodplan/04-legacy-rows-delete.md`
- `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
- `internal/planparser/testdata/goodplan/06-helppins-move.md`
- `internal/planparser/testdata/goodplan/07-json-docs.md`
- `internal/planparser/validate.go`
- `internal/planparser/validate_test.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/validate.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/render.go`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/plan-card-format.md`
- `manifest/designs/scout-plan-symbol-fields.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
