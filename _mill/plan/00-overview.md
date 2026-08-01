# Plan: fabric: audit and migrate all remaining direct git mutations onto Fabric

```yaml
task: 'fabric: audit and migrate all remaining direct git mutations onto Fabric'
slug: webster-bisect-fabric-migrate
approved: false
started: 20260801-184107
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: fabric-warp-methods
    file: 01-fabric-warp-methods.md
    depends-on: []
    verify: go test -tags integration -run TestFabricWarp ./internal/fabricengine/
  - number: 2
    name: webster-bisect-migrate
    file: 02-webster-bisect-migrate.md
    depends-on: [1]
    verify: go test -tags integration -run 'TestIntegrationStage|TestBisectAndEscalate|TestShouldRunIntegration' ./internal/websterengine/
  - number: 3
    name: builder-resethard-migrate
    file: 03-builder-resethard-migrate.md
    depends-on: [1]
    verify: go test -tags integration -run 'TestRestartChain|TestSpawnBatch|TestHeadSHA|TestChangedFiles|TestDirty|TestChainMembers|TestChainEndFor' ./internal/builderengine/
  - number: 4
    name: regression-guard-and-constraints
    file: 04-regression-guard-and-constraints.md
    depends-on: [2, 3]
    verify: go test -run 'TestNoRawGitMutation|TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv' ./cmd/lyx/
```

## Shared Decisions

### Decision: verb-only Fabric method names (illusion-preserving)

- **Decision:** The four new warp-mutating methods on `*fabricengine.Fabric` are named after the git verb only — `CheckoutDetached`, `RestoreBranch`, `CurrentBranch`, `ResetHard` — with no `Warp`/`Weft` token anywhere in the public signature. Each is a one-line delegation to the corresponding `f.Warp.X()` method.
- **Rationale:** `manifest/designs/fabric-unified-view.md` frames Fabric as the one-repo-illusion portal; no external consumer should learn the backend is two repos. Zero external callers of `Fabric.Warp`/`Fabric.Weft` or of `SnapshotWarpSHA` exist today, so these methods must not be the first to leak the split.
- **Applies to:** all batches

### Decision: consumer-side interface seam, satisfied by both `*Fabric` and `*gitrepo.Repo`

- **Decision:** Each migrated consumer package defines a narrow interface (webster's `WarpBisector`, builder's `WarpResetter`) covering exactly the verbs it uses. The migrated functions depend on the interface, never a concrete type. Production wires a real `*fabricengine.Fabric` (constructed inline); tests wire a `*gitrepo.Repo` over their existing single scratch repo (which structurally satisfies the same interface), preserving real-git coverage with no weft fixture.
- **Rationale:** `fabricengine.New` stat-checks that BOTH warp and weft paths exist. The bisect and chain-restart tests build `&hubgeometry.Layout{WorktreeRoot: ..., Cwd: ...}` with `Hub` empty, so `Layout.WeftWorktree()` resolves to a non-existent path and a real `*Fabric` cannot be constructed from these fixtures. The interface seam lets those tests inject `gitrepo.New(worktree)` instead — a single warp-only handle, no weft — while production leaves the seam field nil and constructs the paired `*Fabric` inline.
- **Applies to:** webster-bisect-migrate, builder-resethard-migrate

### Decision: nil-defaulted seam field on the Deps struct (Clock/Starter idiom)

- **Decision:** Both `websterengine.RunDeps` and `builderengine.SpawnDeps` gain one optional interface-typed seam field (`Bisector WarpBisector` / `Resetter WarpResetter`). When nil (production), the point-of-use (`runIntegrationStage` / `SpawnBatch`) constructs `fabricengine.New(deps.Layout.WorktreeRoot, deps.Layout.WeftWorktree())` inline. When non-nil (tests), the injected handle is used verbatim. This mirrors the existing `RunDeps.Clock` (nil → `realClock{}`) and `MasterStarter` seams exactly.
- **Rationale:** The bisect path is only reachable through `websterengine.Run(...)` and the chain-restart path only through `builderengine.SpawnBatch(...)`; neither exposes the handle as a direct call argument a test can pass, so the seam must live on the Deps struct the tests already populate. A nil default keeps every production RunDeps/SpawnDeps construction site unchanged.
- **Applies to:** webster-bisect-migrate, builder-resethard-migrate

### Decision: Go test tiering and verify commands

- **Decision:** New tests that spawn real git (`internal/fabricengine`'s four-method coverage) carry `//go:build integration` (Tier 2) and are run with `-tags integration`. The new regression-guard test in `cmd/lyx` reads file source as text only, spawns nothing itself, and stays untagged (Tier 1). Verify commands are plain Go (`go test ...`), git-root-relative, no `PYTHONPATH=` prefix (this is a Go project, and `_paths.resolve_hub_path() == _paths.resolve_git_root()` here — not a nested layout).
- **Rationale:** Test Tier Purity Invariant (CONSTRAINTS.md) — a real-git test must be tagged; a text-scan guard must not be. `-run` filters keep each batch's verify scoped to the tests it touches rather than the whole multi-package suite.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/rawgitmutation_test.go`
- `cmd/lyx/tierpurity_test.go`
- `internal/builderengine/chain.go`
- `internal/builderengine/chain_test.go`
- `internal/builderengine/gitquery.go`
- `internal/builderengine/gitquery_test.go`
- `internal/builderengine/spawn.go`
- `internal/builderengine/spawn_test.go`
- `internal/fabricengine/fabric.go`
- `internal/fabricengine/warpforward.go`
- `internal/fabricengine/warpforward_integration_test.go`
- `internal/websterengine/integration.go`
- `internal/websterengine/integration_test.go`
- `internal/websterengine/runlevel.go`
