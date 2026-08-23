# Plan: landing: parent-fabric resolution chain

```yaml
task: 'landing: parent-fabric resolution chain'
slug: landing-parent-fabric-resolution-chain
approved: false
started: 20260823-103045
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
    name: fabricengine-parent-resolution
    file: 01-fabricengine-parent-resolution.md
    depends-on: []
    verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
  - number: 2
    name: loomengine-scratch-dir
    file: 02-loomengine-scratch-dir.md
    depends-on: []
    verify: go test ./internal/loomengine/...
  - number: 3
    name: landingshed-comment-fixes
    file: 03-landingshed-comment-fixes.md
    depends-on: []
    verify: go test ./internal/landingshed/...
  - number: 4
    name: loomcli-landing-wiring
    file: 04-loomcli-landing-wiring.md
    depends-on: [1, 2]
    verify: go test ./internal/loomcli/...
  - number: 5
    name: docs-roadmap-and-design
    file: 05-docs-roadmap-and-design.md
    depends-on: [1, 2, 3, 4]
    verify: go test ./internal/lyxcwd/... -run TestEnforcement_MarkdownLinks
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: verify-scoped-per-package

- **Decision:** every batch's `verify:` runs `go test` scoped to the one package the batch touches, chaining a second `-tags integration` invocation of the same package only when the batch edits or creates a `//go:build integration` file.
  No batch runs the repo-wide suite.
- **Rationale:** `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) already runs the repo-wide gate once at Handoff;
  a per-batch verify only needs to catch a regression in the package that batch just edited.
- **Applies to:** all batches.

### Decision: fabric-vocabulary-owner-confinement

- **Decision:** every new `warp`/`weft`-naming identifier, string literal, or comment this plan introduces lands only inside `internal/fabricengine` (a Fabric Vocabulary Invariant owner package).
  `internal/loomcli` is not an owner and reaches fabric's push primitive only through the neutral `Fabric.PushBranch` method this plan adds — never `fabricengine.PushWarpRebaseFreeAt` or any bare `warp`/`weft` token.
- **Rationale:** `internal/loomcli` is absent from `fabricVocabularyOwners` (`internal/lyxcwd/enforcement_test.go:597`), and `TestEnforcement_FabricVocabulary`'s AST walk matches a selector's `Sel` — so `fabricengine.PushWarpRebaseFreeAt(...)` written anywhere in `internal/loomcli` fails the check even as a qualified call.
- **Applies to:** batch 1 (`fabricengine-parent-resolution`), batch 4 (`loomcli-landing-wiring`).

### Decision: no-config-schema-change

- **Decision:** this plan adds no `mill-config.yaml` key and modifies no existing one.
- **Rationale:** every config file this plan touches (`landing.yaml`'s seed fixture in batch 4) is a per-package test fixture, not the repo's own `mill-config.yaml`.
- **Applies to:** all batches — `skip_checks` stays the empty set for the whole plan; the `wiki-config-mutation` validator check never fires.

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `internal/fabricengine/doc.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/matchparent_test.go`
- `internal/fabricengine/openparent_integration_test.go`
- `internal/fabricengine/worktreelist.go`
- `internal/fabricengine/worktreelist_test.go`
- `internal/landingshed/deps.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/drive.go`
- `internal/loomcli/landingdeps.go`
- `internal/loomcli/landingdeps_test.go`
- `internal/loomcli/seedinput.go`
- `internal/loomcli/seedinput_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
