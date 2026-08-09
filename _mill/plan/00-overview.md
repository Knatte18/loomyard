# Plan: plan-format: drop the v3 suffix and sweep every reference by script

```yaml
task: 'plan-format: drop the v3 suffix and sweep every reference by script'
slug: plan-format-drop-v3-suffix
approved: true
started: '20260809-123134'
parent: main
root: ""
verify: go build ./...
```

## Prior failure

- **Holistic round 1** (self-resolved, no reviewer dispatch reached): `millpy-review-code.py --stage prepare` failed both attempts with `verdict: ERROR`, reason: `[resolve_ref_paths] referenced path not found: '.scratch/sweep/main.go'; not in plan creates_union, not on disk`.
  Batch 1 card 2 listed `.scratch/sweep/main.go` under `Edits:`, but the file is a gitignored, never-committed scratch artifact (per the `scratch-is-never-staged` Decision) that batch 4 card 14 correctly deleted as a self-fix for `internal/lyxcwd`'s tree-scan gate — leaving it neither on disk, in `creates_union`, nor in `deletes_union`.
  Fix: dropped the bullet from card 2's `Edits:` list and from this file's `All Files Touched` section, and noted the exclusion inline in card 2's `Requirements:`.

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

- **Decision:** the sweep and the **final** acceptance grep have **different** exclusion sets, and the difference is the point.
  - Excluded from the **scripted sweep**: `manifest/designs/shed-followups.md` (whole file), `manifest/roadmap.md` line 18 only, and the `_mill/`, `.scratch/`, `.git/` directories.
  - Excluded from the **final acceptance grep** (batch 4 card 14): `manifest/designs/shed-followups.md` and the same three directories — **but not** `manifest/roadmap.md:18`, which by then has been rewritten by hand and must pass the gate like every other line.
- **Rationale:** both files carry the pattern as quoted spec text that a *blind replacement* destroys, but only one of them has to keep carrying it afterwards. `shed-followups.md:228` **is** the sentence defining this task's own pattern set, so sweeping it destroys the acceptance criterion it states — and the file is a historical record of what each task was told at scoping time, which is why it keeps its references permanently. `manifest/roadmap.md:18` reads "mechanical rename sweep, `plan-format-v3.md` → `plan-format.md`", which sweeps to a self-referential no-op — but the sentence does not *need* the old filename to say what it says, so batch 2 card 7 rewrites it by hand instead and the exclusion ends there. `_mill/` is task state, torn down on merge.
- **Operator decision, 2026-08-09:** every plan-format-v3 reference the task can reach is removed, and `manifest/roadmap.md:18` can be reached — by rewriting the line rather than by sweeping it. `manifest/designs/shed-followups.md` is the sole surviving site, and batch 4's override notes state plainly why.
- **Applies to:** all batches

### Decision: roadmap-18-is-rewritten-not-swept

- **Decision:** `manifest/roadmap.md:18` is excluded from the sweeper and then rewritten by hand in batch 2 card 7, so the finished line names no version at all. The task slug `plan-format-drop-v3-suffix` stays on that line unchanged.
- **Rationale:** a blind replacement collapses "`plan-format-v3.md` → `plan-format.md`" into "`plan-format.md` → `plan-format.md`", destroying the record of what the task did — but naming the old file was never load-bearing for the sentence, which is a one-line summary of the task, not a citation. Rewriting it to describe the change instead of spelling both filenames keeps the record intact and removes the exclusion. The slug is a task name, not a format reference, and matches none of the six patterns (`plan-format-drop-v3` breaks the `plan-format-v3` adjacency, and `plan-v3` does not occur), so it survives the acceptance grep untouched.
- **Rejected:** leaving line 18 permanently excluded — it is the one excluded site that does not have to stay excluded, and the operator's instruction is that every reachable plan-format-v3 reference goes.
- **Applies to:** rename-and-sweep, doc-prose-v2-erasure, override-notes-and-acceptance

### Decision: exclusions-anchor-on-the-path-field

- **Decision:** every acceptance grep filters exclusions on the **path** field, never on line content. Use `--exclude=shed-followups.md` (filename filter, applied before matching). The intermediate gates in batch 1 card 3 and batch 2 card 8 additionally filter `manifest/roadmap.md`'s line 18, anchored as `^\./manifest/roadmap\.md:18:`; batch 4's final gate carries no roadmap filter at all.
- **Rationale:** a bare `grep -v 'shed-followups.md'` drops every output line whose *text* mentions that filename, so a genuine unfixed hit in another file that happens to cite `shed-followups.md` would be silently exempted and the gate would pass on an incomplete sweep. The roadmap filter is anchored to one file and one line number for the same reason, and it exists only for the window between the sweep and card 7's hand rewrite — once that rewrite lands the filter must come off, or the gate would stop checking a line that is now expected to be clean.
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
