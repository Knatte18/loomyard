# Plan: preflight: split into two Shed rows — a generic one, and loom's own

```yaml
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
slug: 'preflight-loom-agnostic'
approved: false
started: '20260820-090852'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: preflightshed-package
    file: 01-preflightshed-package.md
    depends-on: []
    verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
  - number: 2
    name: loomengine-checkseed
    file: 02-loomengine-checkseed.md
    depends-on: []
    verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
  - number: 3
    name: wire-two-rows
    file: 03-wire-two-rows.md
    depends-on: [1, 2]
    verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
  - number: 4
    name: delete-composite
    file: 04-delete-composite.md
    depends-on: [3]
    verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
  - number: 5
    name: docs
    file: 05-docs.md
    depends-on: [4]
    verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: every-batch-compiles

- **Decision:** every batch leaves `go build ./...` green and the full three-command verify passing.
  The batch order is chosen for exactly this reason: Go has no partial-compile state, so a card that deletes an exported symbol must land in the same batch as every card that retargets its callers.
  Concretely, batch 3 retargets `internal/loomshed` and `internal/loomcli` off `loomengine.Preflight` *before* batch 4 deletes it, rather than deleting first and repairing after.
- **Rationale:** a batch whose verify cannot run is a batch mill-go cannot gate.
- **Applies to:** all batches

### Decision: verify-is-the-discussion-s-three-commands

- **Decision:** every batch's `verify:` is the identical three-command chain the discussion's Testing section pins: `go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli`.
- **Rationale:** this repo's Tier-1 suite measures ~1 s and Tier-2 ~5 s on Linux (`docs/benchmarks/running-tests.md`), so whole-repo scoping costs nothing and is what the repo's own `done_gate` already runs.
  Narrower per-package scoping would be actively wrong here: this task edits `internal/fabricengine` comments, adds a package that the `cmd/lyx` guard suite walks (tier purity, hermetic git env, fabric vocabulary), and changes `internal/loomshed`'s producer count that `internal/loomcli` reads — three failure modes a package-scoped verify would miss.
  The third command is not padding: `internal/loomcli/smoke_test.go` carries `//go:build smoke`, so neither `go test` invocation compiles it, yet batch 4 both edits that file and removes the symbol its line 641 calls.
  All three commands are green at this task's baseline (verified against `1e0fb742`).
- **Applies to:** all batches

### Decision: told-names-never-come-from-the-producer-name-field

- **Decision:** `loomengine.CheckSeed` takes the expected `current_producer` and the tolerated history-producer set as explicit parameters, and `internal/loomshed`'s row-2 producer passes the `NameLoomPreflight` / `{NamePreflight, NameLoomPreflight}` constants directly — never its own `p.name` field.
- **Rationale:** `internal/shedadapters`' package doc records the convention that a producer's told `name` is "used only as a log field and in error text — never compared, parsed, or used for control flow".
  Reusing `p.name` as the on-disk identity compared against `current_producer` would silently make a log label load-bearing.
- **Applies to:** loomengine-checkseed, wire-two-rows

### Decision: docs-land-in-the-task-not-the-card

- **Decision:** the Documentation Lifecycle's same-commit rule is discharged at the task level: batch 5 carries every doc edit, and `mill-merge` squash-merges the whole task branch into `main` as one commit.
- **Rationale:** `main`'s own history shows this shape (`dce9fde8 producers-standalone: squash-merge standalone-producers into main`), so the doc edits and the code edits genuinely do land in one commit on `main`.
  Splitting doc prose across five code batches would spread `manifest/designs/loom.md`'s renumbering over four files' worth of unrelated context.
- **Applies to:** all batches

### Decision: fabric-vocabulary-ban-on-the-new-package

- **Decision:** `internal/preflightshed`'s **production** files (`doc.go`, `ctx.go`, `preflight.go`) must not contain the tokens `weft` or `warp` in any identifier, string literal, or comment, nor any fabric-sense "host <repo-noun>" phrase.
  Use `internal/preflight/doc.go`'s own vocabulary instead: "worktree pair cleanliness", "paired sibling", "fabric readiness/sync".
- **Rationale:** `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` is untagged and walks every production `.go` file under `internal/`, failing any file outside its owner set on a raw substring match. `internal/preflightshed` is not in that owner set.
  `*_test.go` files are excluded from all three of its rules, which is why the migrated integration test may keep its `h.PrimeWeft()` calls verbatim.
- **Applies to:** preflightshed-package

### Decision: untagged-tests-carry-no-spawn-token

- **Decision:** no new untagged (Tier-1) test file may contain the substrings `hubforge.NewHub`, `gitexec.Run`, `exec.Command`, `exec.CommandContext`, or `gitkit.Copy` — not even inside a comment or a string literal.
- **Rationale:** `cmd/lyx/tierpurity_test.go`'s `TestTierPurity_UntaggedTestsSpawnNothing` is a raw substring match, explicitly documented as tripping on "a comment or string-literal mention".
  Every Tier-1 test this plan adds uses `t.TempDir` plus `os.WriteFile`/`state.WriteJSON` only.
- **Applies to:** preflightshed-package, loomengine-checkseed, wire-two-rows

### Decision: preflightshed-integration-test-is-in-package

- **Decision:** the integration test moved into `internal/preflightshed` lands as `package preflightshed`, not `package preflightshed_test`, and its `TestMain` lands in the same package in a `//go:build integration` file.
- **Rationale:** the source file's external-package rationale does not carry over.
  It was external because `internal/loomshed` imports `internal/loomengine`, which is inside `internal/hubforge`'s dependency set — verified here with `go list -deps ./internal/hubforge`, which contains `internal/preflight` but cannot contain `internal/preflightshed`, a new leaf whose only importer is `internal/loomcli`.
  If a cycle nevertheless appears at build time, fall back to `package preflightshed_test` and move the `TestMain` with it; the batch's own verify is what proves which is true.
- **Applies to:** preflightshed-package

### Decision: tier-2-coverage-is-migrated-not-assumed-equivalent

- **Decision:** retiring `internal/loomengine/preflight_integration_test.go` migrates five sub-cases and two whole tests into `internal/preflight/preflight_integration_test.go`, not two whole tests alone.
- **Rationale:** the discussion's disposition table matched at *test-function* granularity, and two of the matched pairs are table tests whose sub-case sets differ.
  Verified directly: `internal/preflight`'s `TestCheckResolved_Dirty` carries two sub-cases (`WarpSide`, `PairedSide`, both untracked-only) against `TestPreflight_WarpDirty`'s five, and `TestCheckResolved_BrokenJunction` covers the `_lyx` junction's `Missing` shape alone against `TestPreflight_JunctionBroken`'s two-junction × three-shape matrix.
  Deleting the loomengine file without migrating those would silently drop Tier-2 coverage of `fabricengine.Healthy`'s typed-`Cause` classification for the `NotALink`/`PointsElsewhere` shapes and for the second, non-`_lyx` junction — coverage that belongs to `internal/preflight` either way, since every one of those cases is a tier-1/tier-2 verdict.
- **Applies to:** delete-composite

## All Files Touched

- `CONSTRAINTS.md`
- `contracts/specs/loom-status-spec.md`
- `docs/overview.md`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/warpclean.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/coherence.go`
- `internal/loomengine/coherence_test.go`
- `internal/loomengine/preflight.go`
- `internal/loomengine/report.go`
- `internal/loomengine/seed.go`
- `internal/loomengine/seed_test.go`
- `internal/loomengine/status.go`
- `internal/loomshed/loompreflight.go`
- `internal/loomshed/loompreflight_test.go`
- `internal/loomshed/loomshed.go`
- `internal/loomshed/loomshed_test.go`
- `internal/loomshed/resume_test.go`
- `internal/loomshed/sequence_test.go`
- `internal/loomshed/stub.go`
- `internal/preflight/preflight_integration_test.go`
- `internal/preflightshed/ctx.go`
- `internal/preflightshed/doc.go`
- `internal/preflightshed/preflight.go`
- `internal/preflightshed/preflight_integration_test.go`
- `internal/preflightshed/preflight_test.go`
- `internal/preflightshed/testmain_integration_test.go`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
