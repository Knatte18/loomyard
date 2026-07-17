# Plan: loom: Discussion producer (interactive interview, auto-mode capable)

```yaml
task: 'loom: Discussion producer (interactive interview, auto-mode capable)'
slug: loom-discussion-producer
approved: true
started: '20260717-182111'
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: hubgeometry-discussion-paths
    file: 01-hubgeometry-discussion-paths.md
    depends-on: []
    verify: go test ./internal/hubgeometry/
  - number: 2
    name: loom-config-module
    file: 02-loom-config-module.md
    depends-on: []
    verify: go test ./internal/loomengine/ ./internal/configreg/ ./internal/initengine/ ./internal/configcli/
  - number: 3
    name: discussion-producer
    file: 03-discussion-producer.md
    depends-on: [1, 2]
    verify: go test ./internal/loomengine/
```

## Shared Decisions

### Decision: mirror-existing-engine-patterns

- **Decision:** Every new piece copies an existing, landed pattern verbatim in
  shape: the config module mirrors `internal/builderengine` (`config.go` +
  `template.yaml` + a `ConfigTemplate()` embed accessor + `configreg`
  registration); the embedded prompt + `stencil` composer mirrors
  `internal/burlerengine` (`review-prompt-template.md` + `template.go` embed +
  `prompt.go` `composePrompt`); the model resolution mirrors
  `internal/builderengine/roles.go` + its spawn-site mapping.
- **Rationale:** these patterns are already reviewed and tested; reusing their
  exact shape minimizes review surface and guarantees consistency. See
  `_mill/discussion.md` decision `mirror-existing-engine-patterns` is implicit in
  every "Technical context" reference there.
- **Applies to:** all batches

### Decision: producer-is-not-a-module

- **Decision:** The discussion producer is Go inside `internal/loomengine`
  (beside Preflight) — a prompt template, a `stencil` composer, and a
  `DiscussionSpec(...) (shuttleengine.Spec, error)` factory. It adds **no** cobra
  command / `lyx` subcommand and no `internal/<x>cli` package.
- **Rationale:** loom.md's module-decomposition table calls producers "not
  modules — just a prompt + profile fed to `shuttle.Run`"; per-phase profiles
  live in loom. (`_mill/discussion.md` decision `producer-lives-in-loomengine-no-module`.)
- **Applies to:** all batches

### Decision: pinned-contract-paths

- **Decision:** The producer targets `_lyx/discussion/decision-record.md` (Plan's
  sole input) and `_lyx/discussion/support-log.md` (review-gate only), resolved
  through `internal/hubgeometry`. The board brief's `discussion.md` /
  `discussion-log.md` names are stale and are NOT used.
- **Rationale:** `docs/reference/discussion-format.md` is the pinned durable
  contract. (`_mill/discussion.md` decision `output-contract-follows-pinned-doc`.)
- **Applies to:** all batches

### Decision: go-comment-and-test-conventions

- **Decision:** Follow the repo's Go conventions: a package/file-level doc
  comment on every new `.go` file (like `builderengine/config.go`'s banner),
  godoc on every exported symbol, and per-file unit tests next to the source. Use
  table-driven tests where natural. No LLM-in-the-loop tests — the producer is
  exercised as pure Go over fixtures.
- **Rationale:** matches `golang-comments` / `golang-testing` skills and every
  existing engine package.
- **Applies to:** all batches

## All Files Touched

- `docs/modules/loom.md`
- `docs/overview.md`
- `docs/roadmap.md`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/hubgeometry/discussionpath_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/configtemplate.go`
- `internal/loomengine/discussion-template.md`
- `internal/loomengine/discussion.go`
- `internal/loomengine/discussion_test.go`
- `internal/loomengine/prompt.go`
- `internal/loomengine/prompt_test.go`
- `internal/loomengine/prompttemplate.go`
- `internal/loomengine/template.yaml`
