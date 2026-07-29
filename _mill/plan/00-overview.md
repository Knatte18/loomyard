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

### Decision: HYBRID name-sourcing — mutating primitives take `names`; read-only health checks load internally

- **Decision:** (Revised after plan-review rounds 6–7 surfaced the config-lives-inside-`_lyx` seam problem; see `_mill/discussion.md`'s revised name-sourcing Decision.) Split fabric's junction functions by whether they *mutate* junctions or only *read* them.
  - **Mutating primitives take the wired name-set as an explicit `[]string` parameter and load NO config:** `WireJunctions(l, slug, names)`, `UnwireJunctions(l, slug, names)`, `removeHostJunction(l, slug, names)`. Because they no longer read config, `WireJunctions` keeps its documented missing-target self-heal (it `os.MkdirAll`s a missing weft `_lyx` target as today), and direct test callers pass names literally (no weft-`_lyx` config seeding). Their callers supply `names`: `Topology.Reconcile`/`Topology.Remove` pass `filterHubReserved(t.cfg.Dirs())` (config already in `t.cfg`); `checkout` and `init`/`undo` load fabric config from a safe context and pass `filterHubReserved(cfg.Dirs())`.
  - **Read-only health checks keep their current signatures and load internally** from the weft base: `PairInSync(l)`, `checkJunctionHealth(l)`, `junctionRepointedDetail(l)` call `junctionNames(filepath.Join(l.WeftWorktree(), l.RelPath))` (the durable, junction-health-independent weft base, per the `internal/fabriccli/weft_verbs.go:112` convention). `internal/loomengine/preflight.go` stays untouched.
  - `Topology.Add`'s `IsReservedHubName` call passes `t.cfg.Dirs()` (raw, unfiltered).
- **Helpers (in `internal/fabricengine/junctionnames.go`):** `filterHubReserved(names []string) []string` applies the `hubgeometry.HubReservedNames()` wiring-guard filter; `junctionNames(baseDir string) ([]string, error)` = `filterHubReserved(LoadConfig(baseDir).Dirs())` for the sites that load (health checks; `init`/`undo`/`checkout` callers).
- **No-fallback (unchanged intent, new site split):** a config-load failure is a SURFACED FAILURE, never a hardcoded default. The name-loading *callers* (`init`, `checkout`, `undo`) propagate the load error up their existing `error` returns before ever calling the mutating primitive. The read-only health checks surface it as a verdict reason that (i) names the fault and (ii) contains the substring `"junction"` — `PairInSync` returns `(false, "host junction check unavailable: cannot load fabric.yaml: <err>", nil)` and `checkJunctionHealth` returns the identical reason string. The `"junction"` substring is required because `preflight.go:125-148`'s check-3 classifier keys on `strings.Contains(reason, "junction")` to set `check3BlocksSeed = true` (its godoc warns any reword must keep it), so a config-load failure correctly blocks seed and check 4 reports `CheckSeedUnreadable`, not a phantom `CheckSeedMissing` — with `loomengine/preflight.go` untouched. The mutating primitives have no config-load path at all. The `pathspec` template default (`_lyx _pattern`) applies only at config-scaffold time.
- **Rationale:** Loading config inside `WireJunctions` broke its `_lyx`-target self-heal (config lives in the very dir it materialises) and rippled into ~10 integration tests that delete/nest the weft `_lyx`. Threading `names` into the mutating primitives dissolves both while preserving self-heal; keeping the read-only health checks internal-loading contains the churn (threading them would push fabric-config loading into `loomengine/preflight`, the one coupling worth avoiding, for no self-heal benefit). Accepts a `WireJunctions`/`UnwireJunctions`/`removeHostJunction` signature change and its `init`/`undo`/`checkout`/`reconcile`/`remove` caller churn as the smaller cost.
- **Applies to:** batch 2, batch 3.

### Decision: genesis first-init materialises fabric config on the weft base, then loads names, then wires

- **Decision:** `WireJunctions` at `internal/initengine/init.go:91` runs BEFORE `configsync.ReconcileAll` writes `fabric.yaml` (init.go:131). Under the hybrid seam `WireJunctions` no longer reads config, but `Init` must now obtain `names` to pass to it, and on a genesis first-ever init (no ancestor weft branch carries `fabric.yaml`) no config exists yet. `Init` is therefore reordered so config is materialised on the weft base BEFORE the name-load + wire: `os.MkdirAll(hubgeometry.ConfigDir(weftBase))` then `results, err := configsync.ReconcileAll(weftBase, true)` (preserves the legacy warp/weft→fabric migration, which only fires when `fabric.yaml` is absent), where `weftBase = filepath.Join(l.WeftWorktree(), l.RelPath)`; then `names, err := junctionNames(weftBase)`; then `fabricengine.WireJunctions(l, slug, names)`. The post-wire `ReconcileAll(cwd, ...)` at the old line 131 is removed — after wiring, `cwd/_lyx` and `weftBase/_lyx` are the same physical directory (junction), so the pre-wire weft-base reconcile produces the identical on-disk result and its `[]Result` feeds `InitResult.Modules`. Because `WireJunctions` itself no longer loads config, there is NO chicken-and-egg (the old fragility where `WireJunctions` read config from the `_lyx` target it was about to materialise is gone); this pre-seed exists only so `Init` has a `pathspec` to read for `names`.
- **Rationale:** Writing to the weft base (not the host `cwd`) avoids the `seedLyxJunction` host-pristine guard (which refuses a real host `_lyx` directory). Reusing `ReconcileAll` (not a raw template write) keeps the legacy-config migration intact. The hybrid seam removes the self-heal break entirely — `WireJunctions(l, slug, names)` still `os.MkdirAll`s a missing weft `_lyx` target as today.
- **Applies to:** batch 2.

### Decision: Go verify commands, no repo-wide compile between signature batches

- **Decision:** Verify commands are native `go test` (no `PYTHONPATH=` prefix; Go project) run from the git root, with `-tags integration` for every batch that touches integration-tagged tests (all three here). TWO cross-package signature changes mean intermediate non-compile is expected and resolved within a batch: batch 1 changes the `hubgeometry` method signatures (so `fabricengine`/`initengine`/`configcli`/`loomengine` do not compile until batch 2 migrates them), and batch 2 changes the `WireJunctions`/`UnwireJunctions`/`removeHostJunction` signatures (migrated across all four packages inside batch 2, which is why batch 2's `verify:` spans `fabricengine`, `initengine`, `configcli`, and `loomengine`). The configured `pipeline.done_gate` (`go test ./...`) gates the fully-assembled tree before the task is marked done.
- **Rationale:** hubgeometry (pure, TDD) is a natural boundary from fabricengine; the compiler-forced migration of every call site is exactly what makes the hybrid seam safe — a missed site fails the build rather than silently mis-wiring at runtime.
- **Applies to:** all batches.

### Decision: docs land within the task, co-located with the change they describe

- **Decision:** The `CONSTRAINTS.md` Hub Geometry Invariant amendment lands in batch 1 (same batch as the invariant-changing literal removal). The `hubgeometry` method godoc is edited inside the same `.go` cards that change those methods (godoc is in-file, so it is same-commit automatically). The `internal/fabricengine/doc.go` and `docs/overview.md` updates land in batch 3 with the fabricengine behavior they describe. No batch of this task merges without its docs; the whole task is one merge unit.
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `docs/overview.md`
- `docs/shared-libs/hubgeometry.md`
- `internal/configcli/configcli_integration_test.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/add_test.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/checkout_index_refresh_test.go`
- `internal/fabricengine/checkout_rollback_test.go`
- `internal/fabricengine/config_driven_junctions_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/drift.go`
- `internal/fabricengine/junction.go`
- `internal/fabricengine/junction_pattern_integration_test.go`
- `internal/fabricengine/junction_repoint_test.go`
- `internal/fabricengine/junctionnames.go`
- `internal/fabricengine/junctionnames_test.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/reconcile_stale_registration_test.go`
- `internal/fabricengine/remove.go`
- `internal/fabricengine/remove_junctions_integration_test.go`
- `internal/fabricengine/weftwiring.go`
- `internal/hubgeometry/geometry_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/hubgeometry/hubgeometry_test.go`
- `internal/hubgeometry/weft_test.go`
- `internal/initengine/init.go`
- `internal/initengine/undo.go`
- `internal/loomengine/preflight_integration_test.go`
- `manifest/designs/fabric-unified-view.md`
