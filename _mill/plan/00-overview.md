# Plan: fabric: config-driven junction list

```yaml
task: 'fabric: config-driven junction list'
slug: fabric-junction-config
approved: false
started: 20260729-150807
parent: main
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
    name: hubgeometry-inject-names
    file: 01-hubgeometry-inject-names.md
    depends-on: []
    verify: go test -tags integration ./internal/hubgeometry/
  - number: 2
    name: fabricengine-wiring
    file: 02-fabricengine-wiring.md
    depends-on: [1]
    verify: go test -tags integration ./internal/fabricengine/ ./internal/initengine/
  - number: 3
    name: proofs-and-docs
    file: 03-proofs-and-docs.md
    depends-on: [2]
    verify: go test -tags integration ./internal/fabricengine/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: single hub-reserved-names source

- **Decision:** The hardcoded hub-reserved name-set `{_board, _portals, _launchers, _raddle}` lives in exactly ONE place: a new exported `hubgeometry.HubReservedNames() []string`. `IsReservedHubName` consumes it internally, and `fabricengine` calls it for the wiring-guard filter. No `fabricengine`-side copy of these names is ever introduced — a `fabricengine` const would duplicate the set and reintroduce the two-drifting-lists hazard. The four literal token strings appear only inside `hubgeometry` (their sole legal owner per the Hub Geometry Invariant).
- **Rationale:** The reserved-names decision and the wiring-guard decision in `_mill/discussion.md` both require the same set; a single accessor is the only form that guarantees they never drift.
- **Applies to:** all batches.

### Decision: junction name-set is injected, never hardcoded in hubgeometry

- **Decision:** `hubgeometry.HostJunctions`/`HostJunctionsHere`/`IsReservedHubName` take the weft-backed name-set as an explicit `[]string` parameter and hold NO `_lyx`/`_pattern` junction-record literals. `hubgeometry` stays config-blind (imports no config package). Junction records are built by a generic name→record loop: host link `filepath.Join(base, RelPath, name)`, weft target `filepath.Join(weftBase, RelPath, name)`, exactly reproducing today's per-name accessors. Record order follows the injected slice order (no forced sort); the default `pathspec: _lyx _pattern` keeps `_lyx` first as a property of the default value, not an enforced invariant.
- **Rationale:** Preserves the Hub Geometry Invariant's spirit (hubgeometry owns path *construction* and the hub-structural tokens) while the *name set* comes from fabric config. Keeps the methods pure functions of their inputs.
- **Applies to:** all batches.

### Decision: fabric sources names internally; consumer signatures stay stable; no silent fallback

- **Decision:** A `fabricengine` helper `junctionNames(baseDir string) ([]string, error)` returns `LoadConfig(baseDir).Dirs()` with the `hubgeometry.HubReservedNames()` set filtered out (the wiring-guard). Every fabric consumer that needs the wired name-set calls this helper and passes the result into the hubgeometry methods; all public/free-function signatures (`WireJunctions(l, slug)`, `UnwireJunctions(l, slug)`, `removeHostJunction(l, slug)`, `PairInSync(l)`, `checkJunctionHealth(l)`, `junctionRepointedDetail(l)`) are UNCHANGED, so `internal/initengine` and `internal/loomengine` callers are untouched. A config-load failure at any name-needing site is a SURFACED ERROR — propagated (`PairInSync` via its `err` return; wiring/unwiring via their existing `error`) or reported as a distinct junction-health reason naming the config-load fault (`"cannot load fabric.yaml: <err>"`) — never a hardcoded default. The `pathspec` template default (`_lyx _pattern`) applies only at config-scaffold time.
- **Rationale:** Most contained; matches `_mill/discussion.md` name-sourcing + no-fallback Decisions and the operator's explicit Q6 direction that a broken config must be caught, not papered over.
- **Applies to:** batch 2, batch 3.

### Decision: all name-needing sites load config from the weft base; Add uses t.cfg

- **Decision:** Every fabric site that needs the wired name-set loads `fabric.yaml` from the WEFT base — the real, durable, weft-synced directory — never through the host `_lyx` junction. Slug-parameterised sites (`WireJunctions`, `UnwireJunctions`, `removeHostJunction`) use `filepath.Join(l.WeftWorktreePath(slug), l.RelPath)`; Here-anchored sites (`PairInSync`, `checkJunctionHealth`, `junctionRepointedDetail`) use `filepath.Join(l.WeftWorktree(), l.RelPath)` (matching the existing `internal/fabriccli/weft_verbs.go:112` weft-base convention). `Topology.Add`'s `IsReservedHubName` call passes `t.cfg.Dirs()` directly (the acting worktree's already-loaded config — the new slug's own config does not exist yet at add time).
- **Rationale:** Two independent reasons force the weft base, never `l.Cwd`/host: (1) at `WireJunctions` time the host `_lyx` junction may not be wired yet (init/checkout/reconcile), so a host-through-junction read fails before the junction exists; (2) the health sites (`PairInSync`, `checkJunctionHealth`) exist to detect a BROKEN `_lyx` junction — reading config through that same junction would fail exactly when it is broken and mask the real drift reason (would break `TestPairInSync_JunctionDriftShapes`). The weft base is durable and junction-health-independent. Verified by code trace.
- **Applies to:** batch 2, batch 3.

### Decision: genesis first-init pre-seeds fabric config on the weft side before wiring

- **Decision:** `WireJunctions` at `internal/initengine/init.go:91` runs BEFORE `configsync.ReconcileAll` writes `fabric.yaml` (init.go:131). Removing the hardcoded junction names means `WireJunctions` now reads config that, on a genesis first-ever init (no ancestor weft branch carries `fabric.yaml`), does not exist yet. `Init` is therefore reordered so the config is materialised on the weft base BEFORE `WireJunctions`: `os.MkdirAll(hubgeometry.ConfigDir(weftBase))` then `configsync.ReconcileAll(weftBase, true)` (which preserves the legacy warp/weft→fabric migration because it only fires when `fabric.yaml` is absent), where `weftBase = filepath.Join(l.WeftWorktree(), l.RelPath)`. The post-wire `ReconcileAll(cwd, ...)` at the old line 131 is removed — after wiring, `cwd/_lyx` and `weftBase/_lyx` are the same physical directory (junction), so the pre-wire weft-base reconcile produces the identical on-disk result and its `[]Result` feeds `InitResult.Modules`. This honors no-fallback: config genuinely exists (written at scaffold time) when `WireJunctions` reads it; no runtime default is introduced.
- **Rationale:** The genesis ordering gap is the precise cost of removing the `_lyx`/`_pattern` hardcode from wiring; today's code only survives genesis because the names are hardcoded. Writing to the weft base (not the host `cwd`) avoids the `seedLyxJunction` host-pristine guard (which refuses a real host `_lyx` directory) and the junction chicken-and-egg. Reusing `ReconcileAll` (not a raw template write) keeps the legacy-config migration intact.
- **Applies to:** batch 2.

### Decision: Go verify commands, no repo-wide compile between signature batches

- **Decision:** Verify commands are native `go test` (no `PYTHONPATH=` prefix; Go project) run from the git root, with `-tags integration` for every batch that touches integration-tagged tests (all three here). Batch 1 changes hubgeometry method signatures, so `internal/fabricengine` and `internal/initengine` do not compile until batch 2 lands — this is expected for a signature migration. Each batch's own `verify:` compiles only its own package tree; the configured `pipeline.done_gate` (`go test ./...`) gates the fully-assembled tree before the task is marked done.
- **Rationale:** hubgeometry (pure, TDD) is a natural boundary from fabricengine (integration); the intermediate non-compile of downstream packages is inherent to any cross-package signature change and is resolved by batch 2.
- **Applies to:** all batches.

### Decision: docs land within the task, co-located with the change they describe

- **Decision:** The `CONSTRAINTS.md` Hub Geometry Invariant amendment lands in batch 1 (same batch as the invariant-changing literal removal). The `hubgeometry` method godoc is edited inside the same `.go` cards that change those methods (godoc is in-file, so it is same-commit automatically). The `internal/fabricengine/doc.go` and `docs/overview.md` updates land in batch 3 with the fabricengine behavior they describe. No batch of this task merges without its docs; the whole task is one merge unit.
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `docs/overview.md`
- `internal/fabricengine/add.go`
- `internal/fabricengine/add_test.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/junctionnames_test.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/weftwiring.go`
- `internal/hubgeometry/geometry_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/hubgeometry/weft_test.go`
- `internal/initengine/init.go`
