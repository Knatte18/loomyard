# Plan: plan-format: drop the v3 suffix and sweep every reference by script

```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
slug: plan-format-drop-v3-suffix
approved: false
started: '20260809-123134'
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
    name: rename-and-sweep
    file: 01-rename-and-sweep.md
    depends-on: []
    verify: go build ./... && go test -tags integration ./internal/planparser/... ./internal/webstercli/... ./internal/websterengine/... ./internal/loomengine/... ./internal/batcher/... ./cmd/lyx/...
  - number: 2
    name: doc-prose-v2-erasure
    file: 02-doc-prose-v2-erasure.md
    depends-on: [1]
    verify: null
  - number: 3
    name: go-comments-and-guards
    file: 03-go-comments-and-guards.md
    depends-on: [1]
    verify: go test ./internal/planparser/... ./internal/webstercli/... ./internal/websterengine/...
  - number: 4
    name: override-notes-and-acceptance
    file: 04-override-notes-and-acceptance.md
    depends-on: [2, 3]
    verify: go build ./... && go test ./...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: no-sed-ever

- **Decision:** No `sed` invocation anywhere in this task. The only bulk find/replace is the temporary Go program at `.scratch/sweep/main.go`; every other edit is a hand-written `Edit` on a named file.
- **Rationale:** `CLAUDE.md` and the `mill:conversation` skill ban `sed` outright — it triggers a permission prompt that stops autonomous work. The task's own `temporary-go-sweeper-in-scratch` decision picks Go for exactly this reason.
- **Applies to:** all batches

### Decision: scratch-is-never-staged

- **Decision:** `.scratch/` is never staged, never committed, and never passed to `git add`. `.gitignore:19`'s `**/.scratch/` entry already ignores it; an explicit `git add .scratch/...` would error rather than silently succeed, so do not attempt it.
- **Rationale:** the sweeper is a one-shot tool, deliberately temporary (`temporary-go-sweeper-in-scratch`). Acceptance gate 7 is "no file under `.scratch/` is staged". The Go toolchain skips dot-prefixed directories during pattern matching, so `go build ./...` and `go test ./...` never see it either.
- **Applies to:** all batches

### Decision: exclusion-set-is-part-of-the-criterion

- **Decision:** three paths are excluded from **both** the scripted sweep and the completion grep: `manifest/designs/shed-followups.md` (whole file), `manifest/roadmap.md` line 18 only, and the `_mill/` directory (plus `.scratch/` and `.git/`). The acceptance grep is defined as zero hits *outside* this set.
- **Rationale:** each carries the pattern as quoted spec text that a blind replacement destroys. `shed-followups.md:228` **is** the sentence defining this task's own pattern set; sweeping it destroys the acceptance criterion. `manifest/roadmap.md:18` reads "`plan-format-v3.md` → `plan-format.md`", which sweeps to a self-referential no-op. `_mill/` is task state, torn down on merge.
- **Applies to:** all batches

### Decision: exclusions-anchor-on-the-path-field

- **Decision:** every acceptance grep filters exclusions on the **path** field, never on line content. Use `--exclude=shed-followups.md` (filename filter, applied before matching) and anchor the roadmap filter as `^\./manifest/roadmap\.md:18:`.
- **Rationale:** a bare `grep -v 'shed-followups.md'` drops every output line whose *text* mentions that filename, so a genuine unfixed hit in another file that happens to cite `shed-followups.md` would be silently exempted and the gate would pass on an incomplete sweep.
- **Applies to:** all batches

### Decision: yaml-v3-is-structurally-unreachable

- **Decision:** `gopkg.in/yaml.v3` is never touched. The exclusion is enforced structurally — all six sweep patterns require a `plan` prefix, so the import string is unmatchable by construction — and verified afterwards by a count assertion rather than implemented as a skip-list.
- **Rationale:** 32 Go files import it, including `internal/planparser/parse.go`. A skip-list would be redundant complexity guarding against a broad bare-`v3` replacement this task never performs. The count check catches the failure mode anyway, and it is silent if unchecked.
- **Applies to:** all batches

### Decision: v1-is-out-of-scope-entirely

- **Decision:** no `v1`/`V1` token anywhere in the tree is touched.
- **Rationale:** every one belongs to an unrelated vocabulary — scout V1, reed v1, the shuttle v1 engine, crucible V1, `hn.algolia.com/api/v1`. There is no `plan-format v1` reference anywhere; the class is empty.
- **Applies to:** all batches

### Decision: schema-version-field-is-not-the-doc-name

- **Decision:** the `format: 3` frontmatter key in `00-overview.md`, `internal/planparser`'s `format-unsupported` check, the worked example's frontmatter, and `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:44`'s reference to it are all left alone.
- **Rationale:** that is the plan schema's own version field, not the document's filename. Renaming the doc does not renumber the schema.
- **Applies to:** all batches

### Decision: markdown-semantic-line-breaks

- **Decision:** every hand-written markdown edit uses semantic line breaks — one sentence per line, an internal break at independent-clause boundaries, plain newlines only, never a fixed-column hard wrap and never trailing double-spaces.
- **Rationale:** repo rule in `CLAUDE.md`. A pure find/replace preserves existing line structure, so the scripted pass is safe by construction; only the hand edits need the discipline applied.
- **Applies to:** doc-prose-v2-erasure, override-notes-and-acceptance

### Decision: no-new-tests

- **Decision:** this task adds no test. The existing suite plus the grep gates is the acceptance criterion.
- **Rationale:** `shed-followups.md:232-233` is explicit that "the meaningful failure mode is incompleteness, checked by grep rather than by an assertion in a test file". Behaviour-preservation coverage already exists in `internal/planparser`'s parse/validate tests, `internal/webstercli/cli_test.go`, `internal/websterengine/template_test.go`, and `cmd/lyx/helptree_test.go`.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `.scratch/sweep/main.go`
- `README.md`
- `docs/overview.md`
- `docs/reference/model-spec.md`
- `docs/reference/plan-format.md`
- `docs/reference/webster-contract.md`
- `internal/batcher/doc.go`
- `internal/loomengine/plan-template.md`
- `internal/loomengine/plan.go`
- `internal/loomengine/plantemplate.go`
- `internal/planparser/doc.go`
- `internal/planparser/normalize.go`
- `internal/planparser/parse.go`
- `internal/planparser/parse_test.go`
- `internal/planparser/plan.go`
- `internal/planparser/sections.go`
- `internal/planparser/validate.go`
- `internal/planparser/validate_test.go`
- `internal/webstercli/cli.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/validate.go`
- `internal/websterengine/classify.go`
- `internal/websterengine/doc.go`
- `internal/websterengine/integration-template.md`
- `internal/websterengine/master-template.md`
- `internal/websterengine/runlevel_test.go`
- `internal/websterengine/template_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/review-finding-classification.md`
- `manifest/designs/scout-plan-symbol-fields.md`
- `manifest/designs/shed-followups.md`
- `manifest/designs/webster-parallel-execution.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
