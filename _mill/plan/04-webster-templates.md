# Batch: webster-templates

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'webster-templates'
number: 4
cards: 4
verify: go test ./internal/websterengine/...
depends-on: [3]
```

## Batch Scope

The two webster prompt templates and their Go renderers — each template is half
of a Go-parsed contract, so this batch also lands the property tests that pin
every contract statement (co-versioning discipline). The master template drives
the bracket-verb loop; the fork template is the thin per-batch prompt
`begin-batch` writes to a file that Master points each fork at. External
interface: `MasterTemplate`/`ForkTemplate` accessors,
`RenderMasterPrompt`/`RenderForkPrompt`/`RenderBatchIndex`/`RenderProgress`
consumed by batches 5 and 7.

## Cards

### Card 15: fork template

- **Context:**
  - `internal/builderengine/implementer-template.md`
  - `internal/builderengine/report.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/fork-template.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** The thin fork-implementer prompt. Markers (all required):
  `{{.batch_file}}`, `{{.batch_name}}`, `{{.report_path}}`,
  `{{.self_fix_cap}}`, `{{.worktree_root}}`, `{{.prev_digest}}`. Content —
  deliberately thin because the fork inherits the Master session's full
  context (no cold orientation, no codebase tour): (1) you are an implementer
  fork for batch `{{.batch_name}}`; read your batch file `{{.batch_file}}` and
  the plan overview only, never another batch's file; (2) the FRESH-READ rule:
  re-read every file your batch touches before editing — file content
  inherited through the fork may be stale, only your own reads are current;
  (3) prior-batch context: `{{.prev_digest}}` (the literal string
  `none (first batch)` when rendering batch 1); (4) implement cards in order,
  commit each completed card to the HOST repo (normal dev git), never touch
  the weft, never call the Agent tool, never pass a name when spawned; (5) run
  the batch `verify:` with at most `{{.self_fix_cap}}` self-fix attempts;
  (6) final action: write the batch report YAML to `{{.report_path}}` — copy
  the report schema section (fields `batch`, `status: done|stuck`,
  `tests: green|red|skipped`, `stuck_reason`, `out_of_scope`) verbatim in
  structure from builder's `implementer-template.md`, since
  `builderengine.ParseReport` parses both.
- **Commit:** `webster: fork implementer template`

### Card 16: master template

- **Context:**
  - `internal/builderengine/orchestrator-template.md`
  - `internal/builderengine/digest.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/master-template.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** The Master session's prompt. Markers (all required):
  `{{.batch_index}}`, `{{.progress}}`, `{{.outcome_path}}`,
  `{{.summary_path}}`, `{{.self_fix_cap}}`, `{{.poll_wait_s}}`. Content:
  (1) orientation: read the codebase structure, conventions, `CONSTRAINTS.md`,
  and the whole plan ONCE up front — this stable context is what every fork
  inherits; (2) the batch list `{{.batch_index}}` driven STRICTLY in order and
  the resume trail `{{.progress}}` (skip batches already reported); (3) the
  bracket loop, verbatim sequence per batch: call `lyx webster begin-batch <NN>`
  FIRST (never fork without it), then spawn ONE fork via the Agent tool with
  `subagent_type: "fork"` and NO name, whose prompt is exactly
  `Read this file and follow it exactly: <prompt_path from the begin-batch envelope>`
  — forwarded verbatim, no additions; when the fork returns, call
  `lyx webster record-batch <NN>` and read ONLY the digest fields it returns
  (enumerate the ten `builderengine.Digest` fields as backtick bullets:
  `batch`, `status`, `tests`, `stuck_reason`, `out_of_scope`,
  `drift_unreported`, `files_changed`, `dirty`, `dead_reason`, `elapsed_s`);
  (4) the failure ladder: `no_report` → re-fork the same batch once → still no
  report → `lyx webster recover-batch <NN>`; `stuck` → `recover-batch <NN>`;
  recover-batch returns `running` → re-call it until terminal (each call
  blocks at most `{{.poll_wait_s}}` seconds); stuck deferred-verify chain →
  `begin-batch <NN> --restart-chain`; (5) a paused refusal
  (`{"paused": true}`) ends the run: write `outcome: paused` and exit;
  (6) hard bans, stated literally for the property tests: NEVER run any git
  command against the weft; NEVER edit, create, or delete any file other than
  `{{.outcome_path}}` and `{{.summary_path}}`; NEVER use a `/model` switch
  (model changes are injected by Go, not chosen by you); NEVER spawn a
  non-fork or named subagent; (7) final action, every run: write
  `{{.outcome_path}}` (keys `outcome: done | stuck | paused`, `stuck_reason`,
  `batches_done` — count per the whole plan, resume-inclusive) AND
  `{{.summary_path}}` — first line `# <title>`, then a narrative of what was
  actually built including deviations from the original task (required when
  `outcome: done`).
- **Commit:** `webster: master session template`

### Card 17: template accessors and renderers

- **Context:**
  - `internal/builderengine/template.go`
  - `internal/builderengine/runlevel.go`
  - `internal/stencil/stencil.go`
  - `internal/websterengine/state.go`
  - `internal/builderengine/plan.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/render.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `render.go` (the two `//go:embed` directives and
  accessors live HERE, not in `template.go` — that file stays config-only):
  `MasterTemplate() []byte`, `ForkTemplate() []byte`, each documenting its
  required marker set in builder's accessor style.
  `RenderForkPrompt(batch builderengine.PlanBatch, planDir string, prevDigest string, reportPath, worktreeRoot string, selfFixCap int) ([]byte, error)`
  — fills the fork template via `stencil.Fill`; `prev_digest` gets
  `none (first batch)` for batch 1, else a one-line rendering of the
  preceding batch's persisted `*builderengine.Digest` (batch name, status,
  tests, files_changed, plus stuck_reason/drift when present) — read from
  state by the caller, never re-derived.
  `RenderMasterPrompt(plan *builderengine.Plan, st *State, outcomePath, summaryPath string, selfFixCap, pollWaitS int) ([]byte, error)`
  — fills the master template; `batch_index` from
  `RenderBatchIndex(plan *builderengine.Plan) string` (webster-local; number,
  slug, oversized and chain annotations); `progress` from
  `RenderProgress(plan *builderengine.Plan, st *State) string` — built from
  the PERSISTED `BatchState.Digest`/`Status` entries (never re-parsing
  reports), listing each terminal batch as done/stuck.
- **Commit:** `webster: template accessors and prompt renderers`

### Card 18: template property tests

- **Context:**
  - `internal/builderengine/template_test.go`
  - `internal/builderengine/digest.go`
  - `internal/builderengine/report.go`
  - `internal/stencil/stencil.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/template_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Property tests in builder's `template_test.go` style:
  (1) master template's digest bullet list equals EXACTLY the ten
  `builderengine.Digest` field names in order;
  (2) master template quotes the outcome keys (`outcome`, `stuck_reason`,
  `batches_done`) and the literal `outcome: done | stuck | paused` line, and
  names `summary_path` with the `# <title>` first-line rule;
  (3) master template contains the literal ban statements: `NEVER run any git
  command against the weft`, `NEVER edit, create, or delete any file other
  than`, `NEVER use a ` + backtick + `/model` + backtick + ` switch`, and the
  bracket-sequence statements (`begin-batch` before every fork,
  `subagent_type: "fork"`, no name, verbatim prompt-file forwarding,
  `record-batch` after the fork returns, re-call `recover-batch` until
  terminal);
  (4) fork template pins the report schema keys (`batch`, `status`, `tests`,
  `stuck_reason`, `out_of_scope`) — the `ParseReport` co-versioning half —
  plus the fresh-read rule statement and the host-commit-per-card statement;
  (5) both templates round-trip through `stencil.Fill` with all markers
  supplied (missing marker = loud stencil error, proving the marker sets);
  (6) `RenderForkPrompt` injects `none (first batch)` for batch 1 and the
  digest line otherwise; `RenderProgress` lists only terminal batches.
- **Commit:** `webster: pin template contracts with property tests`

## Batch Tests

`go test ./internal/websterengine/...`. The property tests ARE the batch's
deliverable-half: every Go-parsed contract statement in the two templates is
pinned against the parsing/producing code (`Digest` fields, `ParseReport`
schema, outcome vocabulary, bracket-sequence and ban literals). All untagged,
spawn-free (embedded bytes + `stencil.Fill`).
