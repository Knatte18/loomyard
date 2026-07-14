# Plan: Reduce git spawns in warpengine integration tests

```yaml
task: "Reduce git spawns in warpengine integration tests"
slug: "warpengine-spawn-reduction"
approved: true
started: "20260714-081130"
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
    name: reduce-redundant-resolve
    file: 01-reduce-redundant-resolve.md
    depends-on: []
    verify: go test -tags integration -run 'TestSiblingLayout|TestStatus|TestReconcile|TestResolveSpawns' ./internal/hubgeometry ./internal/warpengine
```

## Shared Decisions

### Decision: byte-for-byte equivalence is the acceptance bar

- **Decision:** The change must leave `Status`/`Reconcile` observable output
  (the returned `StatusResult` / `ReconcileResult` and their JSON) identical for
  every worktree. `SiblingLayout(root)` must produce a `Layout` deep-equal to
  `hubgeometry.Resolve(root)` for any hub-sibling worktree root; the guarded
  fallback covers the one case (`filepath.Dir(root) != l.Hub`) where they would
  diverge.
- **Rationale:** This is a spawn-reduction refactor, not a behavior change. The
  equivalence test is the safety net; the spawn-count guard locks the win.
- **Applies to:** all batches

### Decision: no cache, no retained state

- **Decision:** `SiblingLayout` derives a sibling `Layout` purely from an
  already-resolved `Layout` plus a known worktree root — no memoization, no
  package-level state. It performs zero git spawns.
- **Rationale:** A keyed cache would risk staleness; the derivation is valid only
  within one operation's consistent `List` snapshot, which the call sites already
  hold.
- **Applies to:** all batches

### Decision: Go test conventions

- **Decision:** Both new tests are `//go:build integration` files (they spawn real
  git). Reuse existing `internal/lyxtest` fixture helpers (`CopyHostHub`,
  `CopyPairedLocal`, `MustRun`, `SeedConfig`, `WireJunctions`). Both packages
  already wire `lyxtest.HermeticGitEnv()` via their `TestMain` — no new test infra.
- **Rationale:** Untagged placement would trip `cmd/lyx/tierpurity_test.go`.
- **Applies to:** all batches

### Decision: Hub Geometry Invariant preserved

- **Decision:** All geometry construction stays in `internal/hubgeometry`
  (`SiblingLayout` is a `Layout` method there). The warpengine `hostLayoutFor`
  helper only *calls* `hubgeometry` (`SiblingLayout` / `Resolve`) and uses
  `filepath.Dir` for the guard — it constructs no geometry token, so the invariant
  (enforced by `internal/hubgeometry/enforcement_test.go`) is untouched.
- **Rationale:** `CONSTRAINTS.md` Hub Geometry Invariant.
- **Applies to:** all batches

## All Files Touched

- `docs/benchmarks/fixture-copy.md`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/siblinglayout_test.go`
- `internal/warpengine/hostlayout.go`
- `internal/warpengine/reconcile.go`
- `internal/warpengine/spawncount_test.go`
- `internal/warpengine/status.go`
