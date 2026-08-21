# Plan: Shed recipe: loader/builder

```yaml
task: 'Shed recipe: loader/builder'
slug: shed-recipe-loader-builder
approved: true
started: '20260821-120310'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: shedbuild-loader
    file: 01-shedbuild-loader.md
    depends-on: []
    verify: go test ./internal/shedbuild/...
  - number: 2
    name: shedbuild-builder
    file: 02-shedbuild-builder.md
    depends-on: [1]
    verify: go test ./internal/shedbuild/...
  - number: 3
    name: loom-equivalence
    file: 03-loom-equivalence.md
    depends-on: [2]
    verify: go test ./internal/shedbuild/...
  - number: 4
    name: docs
    file: 04-docs.md
    depends-on: [3]
    verify: null
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: package layout and file split

- **Decision:** the whole task ships one new package, `internal/shedbuild`, split into five production files — `doc.go` (package doc only), `recipe.go` (the `Recipe` and `Row` types), `parse.go` (`Parse`, `Load`, and their unexported shape checks), `build.go` (`Build`), and `check.go` (`Check`).
  Its production import set is exactly stdlib, `gopkg.in/yaml.v3`, `github.com/Knatte18/loomyard/internal/shedrecipe`, `github.com/Knatte18/loomyard/internal/shedengine`, and `github.com/Knatte18/loomyard/internal/shedcheck`.
  No file in `internal/shedrecipe`, `internal/shedengine`, `internal/shedcheck`, `internal/loomshed`, or `internal/loomcli` is edited by any batch.
- **Rationale:** one identifier per file family matches every sibling in this stack, and the one-way import direction is what the new seam-enforcement test machine-enforces.
- **Applies to:** all batches

### Decision: `Parse` decodes straight into `Recipe`, with no intermediate document struct

- **Decision:** `Parse` decodes into a `Recipe` value directly, so the exported `Recipe` and `Row` types are the only yaml-tagged structs in the package.
  There is no unexported `recipeDoc` mirror type.
- **Rationale:** the settled `Recipe`/`Row` field set is already exactly the document shape, and both types carry explicit `yaml:"..."` tags per the discussion, so a second struct would be a field-for-field copy plus a conversion function with nothing to convert.
  The one visible consequence is that `yaml.v3`'s unknown-key text names `shedbuild.Recipe` and `shedbuild.Row` rather than an unexported type name;
  the discussion's `shedbuild.recipeDoc` was illustrative of yaml's wording, and every test asserts on the offending key name plus the `shedbuild: ` prefix rather than on yaml's own sentence.
- **Applies to:** all batches

### Decision: error posture — first error, `shedbuild: ` prefix, decoder text verbatim

- **Decision:** every exported function returns on its first error.
  Every error message starts with the literal `shedbuild: `.
  Errors produced by `yaml.v3` itself are wrapped with `fmt.Errorf("shedbuild: %w", err)` and otherwise passed through unaltered, keeping yaml's `line N:` position.
  Errors this package raises itself, after a successful decode, name the offending row's zero-based list index and its `name`.
- **Rationale:** matches `internal/shedrecipe`'s accessors and `internal/shedengine`'s own validation, both strictly first-error, and keeps yaml's line number — the better pointer for a defect that lives in a file.
- **Applies to:** all batches

### Decision: tests are in-package, table-driven, and `go test`-only

- **Decision:** every test file declares `package shedbuild` (not `shedbuild_test`), so a test may reach an unexported helper.
  Tables are `[]struct{...}` literals driven by `t.Run(tc.name, ...)`.
  No third-party assertion library, no new module dependency of any kind.
- **Rationale:** the pattern every sibling package in this stack already uses.
- **Applies to:** all batches

### Decision: comment and doc-comment style

- **Decision:** every production file opens with a file-header comment naming the file and what it implements, before the `package` clause.
  Every exported identifier carries a godoc comment starting with its own name.
  Test fixtures and fakes carry a doc comment stating that they are never called, where that is true.
- **Rationale:** the house style throughout `internal/shedrecipe`, `internal/shedcheck`, and `internal/loomshed`.
- **Applies to:** all batches

### Decision: markdown files use semantic line breaks

- **Decision:** every `.md` file this task touches is written one sentence per line, with additional breaks at internal independent-clause boundaries, and never hard-wrapped at a fixed column.
- **Rationale:** the repo-wide rule in `CLAUDE.md`.
- **Applies to:** docs

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `docs/overview.md`
- `internal/shedbuild/build.go`
- `internal/shedbuild/build_engines_test.go`
- `internal/shedbuild/build_test.go`
- `internal/shedbuild/check.go`
- `internal/shedbuild/check_test.go`
- `internal/shedbuild/doc.go`
- `internal/shedbuild/equivalence_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedbuild/load_test.go`
- `internal/shedbuild/parse.go`
- `internal/shedbuild/parse_test.go`
- `internal/shedbuild/recipe.go`
- `internal/shedbuild/seam_enforcement_test.go`
- `internal/shedbuild/testdata/loom-recipe.yaml`
- `manifest/designs/shed-recipe.md`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
