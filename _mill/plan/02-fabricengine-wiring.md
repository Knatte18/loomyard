# Batch: fabricengine-wiring

```yaml
task: 'fabric: config-driven junction list'
batch: fabricengine-wiring
number: 2
cards: 7
verify: go test -tags integration ./internal/fabricengine/ ./internal/initengine/
depends-on: [1]
```

## Batch Scope

This batch makes `fabricengine` source the junction name-set from `fabric.yaml` `pathspec` and pass it into the batch-1 hubgeometry methods, keeping every public/free-function signature stable so `initengine` and `loomengine` callers are untouched. It adds the `junctionNames` helper (with the wiring-guard filter), threads config-loading into the six free-function call sites and `Add`'s reserved-name check, surfaces config-load failure as an error (no silent default), and reorders `Init` to pre-seed the fabric config on the weft side so genesis first-init still wires. After this batch the whole repo compiles again.

The external behavior batch 3 proves: `pathspec: _lyx _pattern _extra` wires all three junctions with no code change; a hub-reserved name in `pathspec` is never wired; an unloadable `fabric.yaml` at a name-needing site is a surfaced error.

Batch-local decisions (all elaborated in the overview's Shared Decisions): weft-base config load for slug sites, `l.Cwd` for Here sites, `t.cfg.Dirs()` for `Add`; distinct `"cannot load fabric.yaml: <err>"` reason in `checkJunctionHealth`; genesis pre-seed via `ReconcileAll(weftBase, ...)` moved ahead of `WireJunctions`.

## Cards

### Card 5: Add the `junctionNames` name-sourcing helper with the wiring-guard filter

- **Context:**
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/junctionnames.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/junctionnames.go` (package `fabricengine`). Add `func junctionNames(baseDir string) ([]string, error)`: call `LoadConfig(baseDir)`; on error return `(nil, err)` (no default — the no-fallback rule); otherwise return `filterHubReserved(cfg.Dirs())`. Add an unexported `func filterHubReserved(names []string) []string` that drops every name present in `hubgeometry.HubReservedNames()` (the wiring-guard: a hub-structural name mis-added to `pathspec` must never wire a per-worktree junction), preserving input order for the rest. Document that the returned slice is the wired name-set (hub-reserved names filtered out); the raw, unfiltered `Config.Dirs()` is used only by `Add`'s reserved-name union, not here. Do not introduce any `_lyx`/`_pattern`/hub-token string literal in this file — names come from runtime config values and `hubgeometry.HubReservedNames()`.
- **Commit:** `feat(fabricengine): add junctionNames helper sourcing wired names from pathspec`

### Card 6: Thread config-sourced names through junction.go wiring/unwiring

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `WireJunctions(l, slug)`, `UnwireJunctions(l, slug)`, `seedLyxJunction(l, slug)`, `seedGitExclude(l, slug)`, `unseedLyxJunction(l, slug)`, and `unseedGitExclude(l, slug)`, replace every `l.HostJunctions(slug)` call with `l.HostJunctions(slug, names)`, where `names, err := junctionNames(filepath.Join(l.WeftWorktreePath(slug), l.RelPath))` is loaded once per public entry point and the error is returned unwrapped-or-wrapped (never defaulted). Keep the public signatures `WireJunctions(l *hubgeometry.Layout, slug string) error` and `UnwireJunctions(l *hubgeometry.Layout, slug string) (UnwireResult, error)` unchanged — load `names` inside them and pass down to the seed/unseed helpers as a new `names []string` parameter on the unexported helpers (`seedLyxJunction`, `seedGitExclude`, `unseedLyxJunction`, `unseedGitExclude`) so each helper receives the already-loaded slice rather than re-loading. `unseedLyxJunction` continues to delegate to `unseedJunctionRecords(l.HostJunctions(slug, names))`; `removeJunctionRecords`/`unseedJunctionRecords` themselves (which already take a `[]hubgeometry.HostJunction`) are unchanged. The weft base `filepath.Join(l.WeftWorktreePath(slug), l.RelPath)` is chosen because the host `_lyx` junction may not be wired yet when `WireJunctions` runs (see overview Decision "baseDir per call site"). Update the affected godoc to state the junction set is sourced from `fabric.yaml` `pathspec` (hub-reserved names filtered).
- **Commit:** `feat(fabricengine): source junction.go wiring names from fabric config`

### Card 7: Thread names through weftwiring.go removeHostJunction

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/remove.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `removeHostJunction(l, slug)` (`internal/fabricengine/weftwiring.go:131`), load `names, err := junctionNames(filepath.Join(l.WeftWorktreePath(slug), l.RelPath))` and pass `l.HostJunctions(slug, names)` into `removeJunctionRecords`. Keep the signature `removeHostJunction(l *hubgeometry.Layout, slug string) error`; on config-load error return that error (there are no removal errors yet at that point — return the load error directly). `removeHostJunction` runs at `remove.go` step 5, before the weft worktree is torn down (step 9), so the weft base config is normally present. Update the godoc to note the name-set is config-sourced AND to document the residual risk this introduces (per no-fallback, no default names are substituted): if `fabric.yaml` is genuinely unreadable at removal time, `removeHostJunction` removes no junctions and returns the error — which `remove.go:91` discards via `_ =` — so a nested junction (`RelPath != "."`) may leak past `fslink.RemoveLinksIn`'s root-level-only safety net (step 6). This is the conscious no-fallback trade-off (surface the fault, never guess a name-set); a best-effort `_lyx`/`_pattern` default sweep is explicitly rejected because it would reintroduce the hardcoded literals this task removes and violate the no-fallback rule. Do not change `remove.go`.
- **Commit:** `feat(fabricengine): source removeHostJunction names from fabric config`

### Card 8: Thread names through drift.go PairInSync

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/loomengine/preflight.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/drift.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `PairInSync(l)` (`internal/fabricengine/drift.go:39`), before the junction loop load `names, err := junctionNames(filepath.Join(l.WeftWorktree(), l.RelPath))` (the weft base — durable and independent of the host `_lyx` junction whose health this function checks; the local `weftWorktree := l.WeftWorktree()` already computed at `:54` may be reused); on error return `false, fmt.Sprintf("cannot load fabric.yaml: %v", err), nil` — a determinable out-of-sync verdict REASON with a `nil` Go error, NOT a hard `err`. This is deliberate and mirrors card 9's `checkJunctionHealth`: `PairInSync` is the Here-anchored twin of `checkJunctionHealth`, and `internal/loomengine/preflight.go:120` propagates a non-nil `err` straight into `return Report{}, err` (an infra-escalation reserved for "could not determine an answer at all"); a missing/corrupt `fabric.yaml` is a determinable "pair unhealthy: bad config" state, so it must surface as a reason (the pair is reported not-in-sync) not as a hard error. Surfacing it as a reason still fully honors no-fallback — nothing is defaulted, the operator sees the fault. Replace `l.HostJunctionsHere()` at `:79` with `l.HostJunctionsHere(names)`. Keep the signature `PairInSync(l *hubgeometry.Layout) (ok bool, reason string, err error)` unchanged so `preflight.go:120` is untouched. Preserve the existing drift `reason` strings verbatim (loom preflight consumes them). Reading from the weft base (not through the host junction) is what keeps the existing `TestPairInSync_JunctionDriftShapes` drift reasons correct when the `_lyx` junction is the broken one. Relax the "stateless, consults no registry" godoc note to "loads `fabric.yaml` for the junction name-set; still consults no registry/status.md," and document that a config-load failure is reported as an unhealthy reason (not a hard error) for parity with `checkJunctionHealth`.
- **Commit:** `feat(fabricengine): source PairInSync junction names from fabric config`

### Card 9: Thread names through reconcile.go health/detail with a distinct config-load reason

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/status.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `checkJunctionHealth(hostLayout)` (`internal/fabricengine/reconcile.go:323`, returns `(bool, string)`), before the loop load `names, err := junctionNames(filepath.Join(hostLayout.WeftWorktree(), hostLayout.RelPath))` (the weft base — durable and independent of the `_lyx` junction whose health this function checks); on error return `false, fmt.Sprintf("cannot load fabric.yaml: %v", err)` — a reason that distinctly names the config-load fault so an operator is not misdirected toward junction repair (this reason is surfaced through `status.go:149`→`pair.JunctionReason`). Replace `hostLayout.HostJunctionsHere()` at `:324` with `hostLayout.HostJunctionsHere(names)`. In `junctionRepointedDetail(hostLayout)` (`:364`, returns `string`), load the same names from the same weft base; on error return `"junction re-pointed: cannot load fabric.yaml: " + err.Error()` (this function only runs after `checkJunctionHealth` found the pair unhealthy AND `WireJunctions` succeeded, so config is present in practice — the error branch is defensive). Replace `hostLayout.HostJunctionsHere()` at `:365` with `hostLayout.HostJunctionsHere(names)`. `hostLayout` is the per-host-worktree layout built by `hostLayoutFor`, so `hostLayout.WeftWorktree()` resolves that pair's weft sibling. Keep both function signatures unchanged.
- **Commit:** `feat(fabricengine): source junction-health names from fabric config with distinct load-failure reason`

### Card 10: Pass pathspec names into Add's reserved-name check

- **Context:**
  - `internal/fabricengine/topology.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At `internal/fabricengine/add.go:119`, change `hubgeometry.IsReservedHubName(slug)` to `hubgeometry.IsReservedHubName(slug, t.cfg.Dirs())` — the acting worktree's already-loaded config (the new slug's own config does not exist yet at add time), passing the RAW unfiltered `Dirs()` (the reserved union must include every `pathspec` junction name, so appending a name to `pathspec` also reserves it as a slug). Do not call the `junctionNames` helper here — its wiring-guard filter would wrongly drop hub-reserved names from the reserved union. Update the reserved-name list in the step-0 comment (`add.go:51`, currently `_lyx, _raddle, _board, _portals, _launchers`) and the reserved-name prose comment (`add.go:111-118`) to state that the junction/weft-backed names now come from `fabric.yaml` `pathspec` (unioned with the hub-structural `hubgeometry.HubReservedNames()` set), rather than a fixed five-name list. **Fix the existing test in the same card:** `TestAdd_RejectsReservedHubNameSlug` (`add_test.go:124`) constructs `fabricengine.NewTopology(fabricengine.Config{})` (empty `Config`, so `t.cfg.Dirs()` is empty); after card 1 removes `_lyx`/`_pattern` from `HubReservedNames()`, its `{"LyxDir", "_lyx"}` sub-case would get `IsReservedHubName("_lyx", [])` == false and fail. Change that `Topology` construction to `fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _pattern"})` so `_lyx`/`_pattern` stay rejected via the injected `pathspec` union while `_board`/`_portals`/`_launchers`/`_raddle` stay rejected via `HubReservedNames()`. This MUST land in batch 2 (not batch 3's card 14) because batch 2's own `verify:` runs `./internal/fabricengine/` and would otherwise fail on this pre-existing test.
- **Commit:** `feat(fabricengine): union pathspec junction names into Add slug-reservation check`

### Card 11: Reorder Init to pre-seed fabric config on the weft side before wiring

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/configsync/configsync.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/initengine/init.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Init` (`internal/initengine/init.go:51`) currently calls `fabricengine.WireJunctions(l, slug)` at `:91` and only later writes config via `configsync.ReconcileAll(cwd, true)` at `:131`. Because `WireJunctions` now reads `fabric.yaml`, materialise the config on the weft base BEFORE wiring so genesis first-init still succeeds: compute `weftBase := filepath.Join(l.WeftWorktree(), l.RelPath)`, `os.MkdirAll(hubgeometry.ConfigDir(weftBase), 0o755)`, then `results, err := configsync.ReconcileAll(weftBase, true)` — placed after the weft-pairing existence check (`:62-65`) and before `fabricengine.WireJunctions(l, slug)`. Remove the now-redundant post-wire `configsync.ReconcileAll(cwd, true)` at `:131` and build `InitResult.Modules` from the pre-wire `results` instead (the `result.Modules` construction loop currently at `:136-146` stays, fed by the pre-wire `results`). Rationale, verified: after wiring, `cwd/_lyx` and `weftBase/_lyx` are the same physical directory (the junction), so a pre-wire weft-base reconcile yields the identical on-disk config; writing to the weft base (not the host `cwd`) avoids `seedLyxJunction`'s host-pristine guard (which refuses a real host `_lyx` directory) and the junction chicken-and-egg; `ReconcileAll` (not a raw template write) preserves the legacy warp/weft→fabric migration, which only fires when `fabric.yaml` is absent. Leave the host-side `os.MkdirAll(lyxDir/patternDir)` (`:103-108`) and `hubgeometry.ConfigDir(cwd)` mkdir (`:110-114`) and `.gitignore` handling in place (they run through the now-wired junction, redundant-but-harmless per their existing comments). Do not change `Init`'s signature or `InitResult` shape.
- **Commit:** `fix(initengine): materialise fabric config on weft before wiring for genesis init`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/ ./internal/initengine/` compiles and runs both packages against the batch-1 hubgeometry signatures — the point at which the repo compiles again after batch 1's signature change. Existing `fabricengine` integration tests (`junction_pattern_integration_test.go`, `remove_junctions_integration_test.go`, `reconcile_stale_registration_test.go`, etc.) exercise the threaded call sites through real host+weft fixtures whose weft side already carries `fabric.yaml`, so they validate that config-sourced names reproduce today's `_lyx`+`_pattern` wiring with no behavior change. `initengine`'s own init tests validate the reorder. `-tags integration` is required for the git-spawning fixtures. New proof tests (extensibility, wiring-guard, no-fallback error) are added in batch 3; this batch relies on the existing suite staying green to confirm the threading is behavior-preserving.
