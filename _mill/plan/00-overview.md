# Plan: Build modelspec - the model-spec parser + registry

```yaml
task: Build modelspec - the model-spec parser + registry
slug: modelspec
approved: false
started: 20260711-063050
parent: main
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: modelspec-core
    file: 01-modelspec-core.md
    depends-on: []
    verify: go test ./internal/modelspec/...
  - number: 2
    name: models-registration
    file: 02-models-registration.md
    depends-on: [1]
    verify: go test ./internal/configreg/... ./internal/configsync/... ./internal/configcli/... ./internal/initengine/... ./cmd/lyx/...
  - number: 3
    name: shuttle-version
    file: 03-shuttle-version.md
    depends-on: [2]
    verify: go test ./internal/shuttleengine/...
```

The DAG is deliberately linear. Batch 2 has a real code dependency on batch 1
(`configreg` references `modelspec.ConfigTemplate`). Batch 3 has no code dependency on
batch 2, but both batch 1 and batch 3 edit `docs/reference/model-spec.md` and
`docs/overview.md`; serializing 3 after 2 (and 2 after 1) avoids any
parallel-modifies-overlap on doc files at zero cost, since mill-go executes batches
sequentially anyway.

## Shared Decisions

### Decision: authoritative-contract

- **Decision:** `docs/reference/model-spec.md` is the pinned contract every card
  implements against; `_mill/discussion.md` holds the task-level decisions (seed-only
  reconcile, fable alias, strict grammar, validation split). Where a card's Requirements
  conflict with either doc, the card is wrong — flag it, do not improvise.
- **Rationale:** Both files were review-approved before planning.
- **Applies to:** all batches

### Decision: error-style

- **Decision:** All modelspec errors are `fmt.Errorf` values prefixed `modelspec: ` and
  name the offending token/character and, where relevant, the offending file path or
  spec string. Fail loud, never silently ignore (contract's Fail-loud section). Follow
  the existing `shuttle: ` prefix style in `internal/shuttleengine/spec.go`. claudeengine
  errors keep that package's existing style.
- **Rationale:** Matches repo convention; fail-loud is pinned in the contract.
- **Applies to:** all batches

### Decision: closed-vocabularies-never-gate-model-names

- **Decision:** `internal/modelspec` owns two package-level sets:
  `knownParams = {"effort", "version"}` and `knownEngines = {"claude"}`. They gate param
  KEYS and engine NAMES only — model names/aliases are NEVER validated against any
  closed list, anywhere (modelspec or claudeengine). This preserves the pinned
  new-model-without-recompile requirement: a brand-new model is adopted via a models.yaml
  entry or the escape form on an old binary.
- **Rationale:** Discussion decision "New-model-without-recompile (pinned requirement)".
- **Applies to:** all batches

### Decision: test-style

- **Decision:** Table-driven tests, standard library `testing` only, mirroring existing
  package tests (`internal/shuttleengine/spec_test.go`,
  `internal/configsync/configsync_test.go`). Filesystem fixtures use `t.TempDir()`; any
  `_lyx/config/models.yaml` path in test code is built via
  `hubgeometry.ConfigFile(baseDir, "models")` — the Hub Geometry Invariant applies to
  test code too.
- **Rationale:** Repo convention + machine-enforced invariant.
- **Applies to:** all batches

### Decision: strict-yaml-decode

- **Decision:** models.yaml entries decode via `gopkg.in/yaml.v3` `yaml.Decoder` with
  `KnownFields(true)` into a struct with exactly `engine`, `model`, `defaults` fields;
  unknown fields in an entry are a loud error naming the alias and the file path.
- **Rationale:** Fail-loud contract; yaml.v3 already in go.mod.
- **Applies to:** modelspec-core

### Decision: commit-style

- **Decision:** Card commits use `<type>(<scope>): <summary>` with scopes `modelspec`,
  `configreg`, `configsync`, `shuttle`, `claudeengine`, `docs`.
- **Rationale:** Matches recent history (`docs(skills): …`, `docs(roadmap): …`).
- **Applies to:** all batches

### Decision: no-roadmap-edit

- **Decision:** `docs/roadmap.md` is NOT touched. modelspec is infrastructure inside the
  still-open `builder` spine milestone (roadmap line 56–63 references the model-spec
  notation as part of that milestone); no planned milestone completes with this task.
- **Rationale:** CLAUDE.md roadmap rule: planned milestones only, no delivered-work notes.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `docs/overview.md`
- `docs/reference/model-spec.md`
- `docs/shared-libs/README.md`
- `internal/configreg/configreg.go`
- `internal/configreg/configreg_test.go`
- `internal/configsync/configsync.go`
- `internal/configsync/configsync_test.go`
- `internal/modelspec/leaf_enforcement_test.go`
- `internal/modelspec/load.go`
- `internal/modelspec/load_test.go`
- `internal/modelspec/modelspec.go`
- `internal/modelspec/parse.go`
- `internal/modelspec/parse_test.go`
- `internal/modelspec/registry.go`
- `internal/modelspec/registry_test.go`
- `internal/modelspec/template.go`
- `internal/modelspec/template.yaml`
- `internal/modelspec/template_test.go`
- `internal/shuttleengine/claudeengine/claudeengine.go`
- `internal/shuttleengine/claudeengine/command.go`
- `internal/shuttleengine/claudeengine/command_test.go`
- `internal/shuttleengine/claudeengine/prepare_test.go`
- `internal/shuttleengine/spec.go`
- `internal/shuttleengine/spec_test.go`
