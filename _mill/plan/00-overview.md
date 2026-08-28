# Plan: Producer-agnostic final-summary artifact + wire Finalize

```yaml
task: "Producer-agnostic final-summary artifact + wire Finalize"
slug: "final-summary-artifact"
approved: true
started: "20260828-061025"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: summaryparser-leaf
    file: 01-summaryparser-leaf.md
    depends-on: []
    verify: go test ./internal/summaryparser/...
  - number: 2
    name: retarget-callers
    file: 02-retarget-callers.md
    depends-on: [1]
    verify: go vet ./... && go vet -tags integration ./... && go test ./internal/summaryparser/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shedadapters/... ./internal/landingshed/... ./internal/loomcli/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/...
  - number: 3
    name: finalize-message
    file: 03-finalize-message.md
    depends-on: [2]
    verify: go test ./internal/landingshed/... && go test -tags integration ./internal/landingshed/...
  - number: 4
    name: docs-and-specs
    file: 04-docs-and-specs.md
    depends-on: [3]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: package-is-summaryparser

- **Decision:** the artifact's read contract lives in a new stdlib-only leaf package `internal/summaryparser`, owning `FileName`, `Path`, `Summary`, `Parse`, and `Summary.CommitMessage`.
  It is named `summaryparser`, joining the `planparser`/`discussionparser` sole-parser family.
- **Rationale:** it is the only placement under which neither `landingshed` nor `websterengine` depends on a producer for the read contract.
  The operator chose the name for consistency with the existing family, against the discussion's own recommendation of `summaryartifact`.
- **Applies to:** all batches

### Decision: no-compat-wrappers

- **Decision:** `websterengine.SummaryFileName`, `websterengine.SummaryPath`, `websterengine.Summary`, and `websterengine.ParseSummary` are deleted outright.
  No thin deprecated wrapper survives in `websterengine`.
- **Rationale:** two names for one thing is exactly how the next producer picks the wrong one.
  The caller set is small and fully enumerated in `_mill/discussion.md`'s **Technical context** section.
- **Applies to:** retarget-callers

### Decision: write-side-stays-in-websterengine

- **Decision:** `websterengine.ArchiveStaleSummary` and `websterengine.AppendIntegrationFailure` stay in `internal/websterengine`.
  Only their internal `SummaryPath(websterDir)` call changes to `summaryparser.Path(websterDir)`.
- **Rationale:** both are producer-specific *write* policy — the archive discipline reuses `archive.go`'s `firstFreeArchivePath`/`archiveTimestampFormat`, and the integration-failure append is written by webster's own bisect.
  The roadmap item is a read-side requirement.
- **Applies to:** retarget-callers

### Decision: error-prefix-summaryparser

- **Decision:** every error raised inside `internal/summaryparser` is prefixed `summaryparser: `, never `webster: `.
  Callers wrap with their own package prefix, e.g. `landingshed: Finalize: parse summary artifact: %w`.
- **Rationale:** the owning package names itself, as every other package in this tree does.
  A `webster:` prefix on an error raised by a producer-agnostic parser would state the opposite of what this task establishes.
- **Applies to:** summaryparser-leaf, retarget-callers, finalize-message

### Decision: commitmessage-body-trim

- **Decision:** `CommitMessage` returns the bare `Title` when `strings.TrimSpace(Body) == ""`, and otherwise `Title + "\n\n" + strings.TrimLeft(Body, " \t\r\n")`.
  Trailing whitespace is left alone.
  The trim lives in `CommitMessage`, never in `Parse`.
- **Rationale:** `Parse` sets `Body` to the lines after the heading verbatim, so a conventionally formatted artifact yields a `Body` whose first character is `\n`; joining it raw would emit two blank lines between subject and body.
  Trimming inside `Parse` instead would silently change the pull-request body `Publish` has produced since the artifact shipped, which is out of scope.
  Git strips trailing whitespace from a commit message itself, so trimming it here would be a second implementation of an existing normalization.
- **Applies to:** summaryparser-leaf, finalize-message

### Decision: told-final-summary-path

- **Decision:** `landingshed.Deps.WebsterDir` is deleted and replaced by `FinalSummaryPath string`, the told absolute path to the artifact itself.
  `internal/loomcli/landingdeps.go` fills it as `summaryparser.Path(geom.WebsterDir)`.
- **Rationale:** this is the shed-level told path the roadmap item asks for.
  It satisfies the Told-Geometry Invariant — `landingshed` derives no path of its own — and leaves `websterengine.Dir` the sole declarer of the `_lyx/webster` directory segment.
  The consumer now knows a path, not a producer.
- **Applies to:** retarget-callers, finalize-message

### Decision: production-only-invariant-scope

- **Decision:** the new **Summaryparser Sole-Parser Invariant** is scoped to production code.
  Bare `"summary.md"` literals in test files stay legal and are not rewritten.
- **Rationale:** bare literals already exist across the test tree — `internal/websterengine/recordbatch_test.go`, `internal/webstercli/smoke_test.go`, and roughly a dozen sites in `internal/websterengine/runlevel_test.go` — where a fixture writing the literal filename is the clearer test.
  An unscoped invariant would declare every one of those a violation.
  This matches how the Lyxdirs Single-Declarer Invariant already scopes itself to "no other production file".
- **Applies to:** all batches

### Decision: go-verify-no-pythonpath-prefix

- **Decision:** every `verify:` command in this plan is a native Go test-runner invocation with no `PYTHONPATH= ` prefix.
- **Rationale:** this is a Go repository; the `PYTHONPATH= ` shape rule applies to Python/mill projects only.
- **Applies to:** all batches

### Decision: markdown-semantic-line-breaks

- **Decision:** every `.md` file this plan writes or edits uses one sentence per line, with additional breaks at internal independent-clause boundaries.
  No fixed-column hard wrap, no trailing double-space, no backslash line break.
- **Rationale:** the repo's own `CLAUDE.md` rule, so an edit inside a paragraph touches only the changed sentence.
- **Applies to:** summaryparser-leaf, docs-and-specs

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `contracts/specs/final-summary-spec.md`
- `contracts/specs/webster-spec.md`
- `docs/overview.md`
- `internal/landingshed/deps.go`
- `internal/landingshed/finalize.go`
- `internal/landingshed/finalize_integration_test.go`
- `internal/landingshed/finalize_test.go`
- `internal/landingshed/publish.go`
- `internal/landingshed/publish_integration_test.go`
- `internal/landingshed/publish_test.go`
- `internal/landingshed/seam_enforcement_test.go`
- `internal/loomcli/landingdeps.go`
- `internal/loomcli/landingdeps_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/shedadapters/doc.go`
- `internal/shedadapters/webster.go`
- `internal/shedadapters/webster_test.go`
- `internal/shedbuild/fixture_test.go`
- `internal/shedrecipe/recipe.go`
- `internal/shedrecipe/entries_simple_test.go`
- `internal/summaryparser/doc.go`
- `internal/summaryparser/leaf_enforcement_test.go`
- `internal/summaryparser/summary.go`
- `internal/summaryparser/summary_test.go`
- `internal/webstercli/recordbatch.go`
- `internal/websterengine/integration_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/summary.go`
- `internal/websterengine/summary_test.go`
- `manifest/roadmap.md`
