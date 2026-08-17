# Plan: config degrades to embedded template

```yaml
task: "config degrades to embedded template"
slug: "config-template-fallback"
approved: false
started: "20260817-150405"
parent: "standalone-producers"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: configengine-loadortemplate
    file: 01-configengine-loadortemplate.md
    depends-on: []
    verify: go test ./internal/configengine/...
  - number: 2
    name: producer-loaders-repointed
    file: 02-producer-loaders-repointed.md
    depends-on: [1]
    verify: go test ./internal/shuttleengine/... ./internal/reedengine/... ./internal/perchengine/... ./internal/websterengine/...
  - number: 3
    name: docs-and-invariant
    file: 03-docs-and-invariant.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints._

### Decision: fallback-only-on-proven-absence

- **Decision:** the degrading path resolves the embedded template **only** when the missing thing is provably absent — `errors.Is(err, configengine.ErrNotInitialized)` for the `_lyx/` check, and `os.IsNotExist(err)` for the config-file read.
  Every other failure at either point (permission, IO, stat error) propagates unchanged, on both the strict and the degrading path.
- **Rationale:** `FindBaseDir` returns a bare `stat _lyx: %w` for a permission or IO error.
  Falling back on "the `FindBaseDir` failure" as a whole would turn a genuinely broken `_lyx/` into a silent set of template defaults — a producer would then run against a hub it could not read.
- **Applies to:** all batches

### Decision: configengine-load-is-behaviourally-unchanged

- **Decision:** `configengine.Load`'s observable behaviour does not change.
  Every existing `TestLoad_*` and `TestFindBaseDir_*` test in `internal/configengine/config_test.go` stays as written and passing;
  that is what proves the shared-body refactor preserved strict behaviour.
  The single internal change is that `FindBaseDir`'s absent-`_lyx/` error now wraps `ErrNotInitialized` — its message text is byte-identical, so the four strict callers' `strings.Contains(err.Error(), "not initialized")` rewraps keep working untouched.
- **Rationale:** four hub-scoped callers (`fabricengine`, `boardengine`, `loomengine`, `batcher`) depend on the strict refusal, and this task does not open those packages.
- **Applies to:** all batches

### Decision: strict-when-present-stays-strict

- **Decision:** a config file that *exists* but is broken still errors — missing template keys, empty, or comments-only.
  Only an *absent* `_lyx/` or an *absent* config file degrades to the template.
- **Rationale:** the fallback exists to support a config-less invocation, not to paper over a malformed config an operator wrote.
- **Applies to:** all batches

### Decision: no-strings-contains-migration

- **Decision:** the four strict callers are **not** migrated onto `errors.Is`.
  They keep their `strings.Contains(err.Error(), "not initialized")` rewraps.
  No file under `internal/fabricengine`, `internal/boardengine`, `internal/loomengine`, or `internal/batcher` is opened by this task.
- **Rationale:** the new sentinel makes that migration possible and the new invariant records it as available, but doing it here would touch four packages this task has no other reason to open.
- **Applies to:** all batches

### Decision: no-machine-guard-for-the-new-invariant

- **Decision:** the Config Strictness Invariant lands as CONSTRAINTS.md text with review-obligation enforcement.
  No guard is built, no file under `cmd/lyx/` is created or edited, and `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map gains no entry.
  The invariant's `Enforced by` line records the guard's exact shape and names T10 as its home.
- **Rationale:** T1 (this wave's sibling) made the same call on the same grounds;
  `producers-standalone.md` places new-invariant work at T10;
  and a new `cmd/lyx` guard file here would silently invalidate the design's ten-task file-contention analysis.
- **Applies to:** all batches

### Decision: go-comment-and-markdown-style

- **Decision:** Go comments follow the `golang:golang-comments` skill's godoc rules — an exported identifier's doc comment starts with its own name.
  Markdown uses semantic line breaks: one sentence per line, plus a break at internal independent-clause boundaries;
  no fixed-column hard wrap;
  table cells stay on one line.
- **Rationale:** repo-wide CLAUDE.md rules, and the doc batch edits two markdown files.
- **Applies to:** all batches

### Decision: no-test-asserts-the-log-line

- **Decision:** no test in this task asserts on the `logger.Info` fallback record.
- **Rationale:** under `go test` the durable sink is disarmed unless `LYX_TRACE=1`, and stderr sits above the default Warn threshold, so such a test would assert on the harness's environment rather than on the code.
  The fallback's *behaviour* is fully tested;
  its logging is not.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `docs/shared-libs/configengine.md`
- `internal/configengine/config.go`
- `internal/configengine/config_test.go`
- `internal/perchengine/config.go`
- `internal/perchengine/config_test.go`
- `internal/reedengine/config.go`
- `internal/reedengine/config_test.go`
- `internal/shuttleengine/config.go`
- `internal/shuttleengine/config_test.go`
- `internal/websterengine/config.go`
- `internal/websterengine/config_test.go`
