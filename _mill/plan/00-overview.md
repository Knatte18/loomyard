# Plan: Build burler - the review+fix round worker

```yaml
task: "Build burler - the review+fix round worker"
slug: "internal-burler"
approved: false
started: "20260708-060559"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: engine-core
    file: 01-engine-core.md
    depends-on: []
    verify: go build ./... && go test ./internal/burlerengine/...
  - number: 2
    name: engine-round
    file: 02-engine-round.md
    depends-on: [1]
    verify: go build ./... && go test ./internal/burlerengine/...
  - number: 3
    name: burler-cli
    file: 03-burler-cli.md
    depends-on: [2]
    verify: go build ./... && go test ./internal/burlercli/...
  - number: 4
    name: registration-and-suite
    file: 04-registration-and-suite.md
    depends-on: [3]
    verify: go build ./... && go vet -tags smoke ./internal/burlerengine/... && go test ./internal/burlercli/... ./cmd/lyx/... ./tools/sandbox/...
  - number: 5
    name: docs-lifecycle
    file: 05-docs-lifecycle.md
    depends-on: [4]
    verify: go build ./... && go test ./...
```

## Shared Decisions

### Decision: source-of-truth

- **Decision:** `_mill/discussion.md` is the authoritative spec for every batch. Where this
  plan and the discussion disagree, the discussion wins; where the discussion and
  `docs/modules/burler.md` disagree, the discussion wins (it post-dates the doc and records
  the review-resolved decisions, e.g. the `{overlay, source}` FixScope rename).
- **Rationale:** the discussion went through three review rounds; the design doc is deleted
  in batch 5.
- **Applies to:** all batches

### Decision: naming-and-error-style

- **Decision:** packages `internal/burlerengine` (domain kernel, no cobra) and
  `internal/burlercli` (thin cobra wrapper). Every error message is prefixed `burler: `.
  Every `.go` file opens with the repo's file-level doc-comment convention (see
  `internal/shuttleengine/spec.go` for the style). Enum-like values are typed strings with
  exported constants: `Verdict` (`VerdictApproved = "APPROVED"`, `VerdictBlocking =
  "BLOCKING"`), `Severity` (`SeverityBlocking = "BLOCKING"`, `SeverityMedium = "MEDIUM"`,
  `SeverityLow = "LOW"`, `SeverityNit = "NIT"`), `FixScope` (`FixScopeOverlay = "overlay"`,
  `FixScopeSource = "source"`).
- **Rationale:** matches shuttleengine/muxengine conventions; CLI/Cobra Invariant naming.
- **Applies to:** all batches

### Decision: no-shuttleengine-changes

- **Decision:** zero edits under `internal/shuttleengine/` (and no new fields on `Spec`).
  burlerengine imports shuttleengine for types only (`Spec`, `Result`, `Outcome`) and never
  imports `claudeengine`; the real engine is wired in burlercli (and in the smoke test,
  which acts as the caller).
- **Rationale:** discussion decisions tool-use-prompt-level and shuttle-seam; Shuttle
  Provider-Seam Invariant.
- **Applies to:** all batches

### Decision: result-carries-findings

- **Decision:** `Result` carries `Findings []Finding` alongside the fields the discussion
  lists (`Outcome`, `Verdict`, `ReviewPath`, `FixerReportPath`, `SessionID`, `StrandGUID`,
  `LastAssistantMessage`).
- **Rationale:** `ParseReview` already validates the findings; discarding them would force
  perch (the consumer that keys cycle detection on finding `id`s) to re-read and re-parse
  the review file burler just parsed.
- **Applies to:** engine-core, engine-round, burler-cli

### Decision: yaml-strictness-split

- **Decision:** the review-file frontmatter parse tolerates unknown extra keys (top-level
  and per-finding) while enforcing every pinned rule from the discussion's
  review-file-format-and-parse decision; the CLI's profile-YAML decode is strict
  (`yaml.Decoder.KnownFields(true)`) so an operator typo (`fixscope:` for `fix-scope:`)
  fails loudly instead of silently zeroing a field.
- **Rationale:** agent-written metadata (a `date:` line) is harmless noise; a mistyped
  profile key silently changes safety-critical behavior.
- **Applies to:** engine-core, burler-cli

### Decision: absolute-paths-at-the-seam

- **Decision:** `(*Profile).validate(worktreeRoot string)` resolves every path field
  (`Target.Paths`, `Fasit.Paths`, `PriorReviews`, `PriorFixerReports`, `ReviewPath`,
  `FixerReportPath`) to a cleaned absolute path in place — already-absolute entries kept
  verbatim, relative entries joined onto `worktreeRoot` — mirroring
  `shuttleengine.Spec.validate`. Everything downstream (prompt, spec, `Result`) sees only
  absolute paths.
- **Rationale:** discussion decision artifact-paths (r3 NOTE): callers open
  `Result.ReviewPath` directly, no re-resolving.
- **Applies to:** engine-core, engine-round

### Decision: go-composes-template-blocks

- **Decision:** the embedded prompt template has exactly eight markers, all at top level:
  `{{.target}}`, `{{.fasit}}`, `{{.rubric}}`, `{{.fix_scope_rules}}`,
  `{{.tool_use_rules}}`, `{{.prior_rounds}}`, `{{.review_path}}`,
  `{{.fixer_report_path}}`. Go composes each block's full text (path lists, commit rules,
  clean-room hydration text, the "first round — no prior files" fallback) and passes them
  as marker values. No `{{if}}`/`{{range}}` in the template.
- **Rationale:** `stencil.Fill` only empty-checks TOP-LEVEL markers; a required marker
  inside a conditional branch would render silently blank when present-but-empty (see
  `internal/stencil/stencil.go`).
- **Applies to:** engine-round

### Decision: go-native-verify

- **Decision:** all `verify:` commands are Go-native (`go build` / `go vet` / `go test`),
  no `PYTHONPATH=` prefix.
- **Rationale:** the `verify-not-isolated` rule is Python-project-specific; this is a Go
  repo.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `docs/modules/README.md`
- `docs/modules/hardener.md`
- `docs/modules/loom.md`
- `docs/modules/perch.md`
- `docs/overview.md`
- `docs/reviews/README.md`
- `docs/roadmap.md`
- `docs/shared-libs/stencil.md`
- `internal/burlercli/cli.go`
- `internal/burlercli/cli_test.go`
- `internal/burlercli/run.go`
- `internal/burlerengine/doc.go`
- `internal/burlerengine/engine.go`
- `internal/burlerengine/engine_test.go`
- `internal/burlerengine/profile.go`
- `internal/burlerengine/profile_test.go`
- `internal/burlerengine/prompt.go`
- `internal/burlerengine/prompt_test.go`
- `internal/burlerengine/review-prompt-template.md`
- `internal/burlerengine/smoke_round_test.go`
- `internal/burlerengine/template.go`
- `internal/burlerengine/template_test.go`
- `internal/burlerengine/verdict.go`
- `internal/burlerengine/verdict_test.go`
- `sandbox-burler-suite.cmd`
- `tools/sandbox/SANDBOX-BURLER-SUITE.md`
- `tools/sandbox/main.go`
- `tools/sandbox/main_test.go`
- `tools/sandbox/suite.go`
