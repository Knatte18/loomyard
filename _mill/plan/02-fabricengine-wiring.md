# Batch: fabricengine-wiring

```yaml
task: 'fabric: config-driven junction list'
batch: fabricengine-wiring
number: 2
cards: 7
verify: go test -tags integration ./internal/fabricengine/ ./internal/initengine/ ./internal/configcli/ ./internal/loomengine/
depends-on: [1]
```

## Batch Scope

This batch makes `fabricengine` source the junction name-set from `fabric.yaml` `pathspec` and pass it into the batch-1 hubgeometry methods, keeping every public/free-function signature stable so `initengine` and `loomengine` callers are untouched. It adds the `junctionNames` helper (with the wiring-guard filter), threads config-loading into the six free-function call sites and `Add`'s reserved-name check, surfaces config-load failure as an error (no silent default), and reorders `Init` to pre-seed the fabric config on the weft side so genesis first-init still wires. After this batch the whole repo compiles again.

The external behavior batch 3 proves: `pathspec: _lyx _pattern _extra` wires all three junctions with no code change; a hub-reserved name in `pathspec` is never wired; an unloadable `fabric.yaml` at a name-needing site is a surfaced error.

Batch-local decisions (all elaborated in the overview's HYBRID Shared Decisions): the mutating primitives `WireJunctions`/`UnwireJunctions`/`removeHostJunction` take an explicit `names []string` and load NO config; their callers supply names (`t.cfg` for `Topology` methods; `WiredNames(baseDir)` for `init`/`undo`). Read-only health checks (`PairInSync`/`checkJunctionHealth`/`junctionRepointedDetail`) keep loading internally from the weft base. The signature change is compiler-checked: every call site across `fabricengine`, `initengine`, `configcli`, and `loomengine` (prod + test) must be migrated for the tree to build — this batch's `verify:` widens to cover all four packages.

## Cards

### Card 5: Add the `junctionNames` name-sourcing helper with the wiring-guard filter

- **Context:**
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/topology.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/junctionnames.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/junctionnames.go` (package `fabricengine`) with three helpers. (1) `func filterHubReserved(names []string) []string` (unexported): drops every name present in `hubgeometry.HubReservedNames()` (the wiring-guard — a hub-structural name mis-added to `pathspec` must never wire a per-worktree junction), preserving input order for the rest. (2) `func junctionNames(baseDir string) ([]string, error)` (unexported, for in-package load sites — the health checks and `checkout`): call `LoadConfig(baseDir)`; on error return `(nil, err)` (no default — no-fallback); otherwise return `filterHubReserved(cfg.Dirs())`. (3) `func WiredNames(baseDir string) ([]string, error)` (EXPORTED, a thin wrapper over `junctionNames`) — so out-of-package callers (`internal/initengine`'s `init`/`undo`) can obtain the wired name-set without duplicating the filter. Document that the returned slice is the wired name-set (hub-reserved names filtered out); the raw, unfiltered `Config.Dirs()` is used only by `Add`'s reserved-name union (card 10), not here. `Topology` methods that already hold config (`Reconcile`, `Remove`) call `filterHubReserved(t.cfg.Dirs())` directly. Do not introduce any `_lyx`/`_pattern`/hub-token string literal in this file — names come from runtime config values and `hubgeometry.HubReservedNames()`.
- **Commit:** `feat(fabricengine): add junction name-set helpers (filter, load, exported WiredNames)`

### Card 6: Give WireJunctions/UnwireJunctions a `names` param and migrate every call site

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/topology.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/checkout_rollback_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the mutating primitives to take the wired name-set explicitly and load NO config: `WireJunctions(l *hubgeometry.Layout, slug string, names []string) error` and `UnwireJunctions(l *hubgeometry.Layout, slug string, names []string) (UnwireResult, error)`. Inside them, replace every `l.HostJunctions(slug)` with `l.HostJunctions(slug, names)` and pass `names` down to the unexported helpers (`seedLyxJunction`, `seedGitExclude`, `unseedLyxJunction`, `unseedGitExclude`) as a new `names []string` parameter (each helper receives the slice, never loads). `unseedLyxJunction` delegates to `unseedJunctionRecords(l.HostJunctions(slug, names))`; `removeJunctionRecords`/`unseedJunctionRecords` (already take `[]hubgeometry.HostJunction`) are unchanged. Because `WireJunctions` no longer reads config, its documented missing-target self-heal is preserved verbatim (`seedLyxJunction` still `os.MkdirAll`s a missing weft target first). Update the godoc to state the wired name-set is now supplied by the caller (sourced from `fabric.yaml` `pathspec`, hub-reserved names filtered). **Migrate every call site (compiler-forced):** (a) fabricengine PROD — `checkout.go:152` (`Checkout` is a `*Topology` method) passes `filterHubReserved(t.cfg.Dirs())`; `reconcile.go:155` (`Reconcile` is a `*Topology` method) passes `filterHubReserved(t.cfg.Dirs())`. (b) All test callers pass the literal default set `[]string{"_lyx", "_pattern"}` (they wire the default pair), EXCEPT any test needing a different set: `junction_pattern_integration_test.go` and `remove_junctions_integration_test.go` — pass `[]string{"_lyx", "_pattern"}` at their existing call sites; the `MaterialisesMissingWeftTarget` test (`junction_pattern_integration_test.go:86`) and the nested `TestRemove_TearsDownNestedJunction` (`remove_junctions_integration_test.go:61`) now need NO config seeding — passing names directly is exactly what fixes the R6/R7 breakages, and self-heal of the deleted/absent weft target still works. Test files to migrate: `junction_pattern_integration_test.go`, `junction_repoint_test.go`, `remove_junctions_integration_test.go`, `checkout_rollback_test.go`, `checkout_index_refresh_test.go`, `reconcile_stale_registration_test.go` (fabricengine); `configcli/configcli_integration_test.go:59` and `loomengine/preflight_integration_test.go:41` (other packages — pass `[]string{"_lyx", "_pattern"}`). `initengine`'s `init.go`/`undo.go` callers are migrated in card 11.
- **Commit:** `feat(fabricengine): thread names param into WireJunctions/UnwireJunctions and migrate call sites`

### Card 7: Give removeHostJunction a `names` param; caller supplies from t.cfg

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/topology.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/remove.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `removeHostJunction(l, slug)` (`internal/fabricengine/weftwiring.go:131`) to `removeHostJunction(l *hubgeometry.Layout, slug string, names []string) error`, passing `l.HostJunctions(slug, names)` into `removeJunctionRecords`; it loads no config. Update its caller `remove.go:91` (`Topology.Remove`, which holds `t.cfg`) from `_ = removeHostJunction(l, slug)` to `_ = removeHostJunction(l, slug, filterHubReserved(t.cfg.Dirs()))`. Because the name-set now comes from `t.cfg` (always present in a `Topology`), there is NO config-load path in `removeHostJunction` and therefore NONE of the config-load-failure residual-junction-leak risk the earlier internal-load design had (that risk is dissolved by the hybrid seam). No `"path/filepath"` import is needed (the function no longer computes a weft base). Update the godoc to note the name-set is caller-supplied.
- **Commit:** `feat(fabricengine): thread names param into removeHostJunction from t.cfg`

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
- **Requirements:** In `PairInSync(l)` (`internal/fabricengine/drift.go:39`), before the junction loop load `names, err := junctionNames(filepath.Join(l.WeftWorktree(), l.RelPath))` (the weft base — durable and independent of the host `_lyx` junction whose health this function checks; the local `weftWorktree := l.WeftWorktree()` already computed at `:54` may be reused); on error return `false, fmt.Sprintf("host junction check unavailable: cannot load fabric.yaml: %v", err), nil` — a determinable out-of-sync verdict REASON with a `nil` Go error, NOT a hard `err`. Two constraints shape this exactly: (i) it must be a reason, not a hard `err`, because `internal/loomengine/preflight.go:120-123` propagates a non-nil `err` straight into `return Report{}, err` (an infra-escalation reserved for "could not determine an answer at all"), whereas a missing/corrupt `fabric.yaml` is a determinable "pair unhealthy: bad config" state — this mirrors card 9's `checkJunctionHealth`; (ii) the reason string MUST contain the substring `"junction"`. `preflight.go:125-148`'s check-3 classifier sets `check3BlocksSeed = true` only when `strings.Contains(reason, "junction")` (its own godoc at `:125-131` explicitly warns "Any future reword of those reasons must keep the substring 'junction' in them, or this classification silently reverts to CheckWeftSync"). A config-load failure means the junction set is undeterminable — a genuine junction-check failure that MUST block seed (check3BlocksSeed=true) so check 4 reports `CheckSeedUnreadable("unreadable, see check 3")` rather than misreporting `CheckSeedMissing`. The chosen wording `"host junction check unavailable: cannot load fabric.yaml: <err>"` satisfies both: it contains `"junction"` (correct preflight classification, no `preflight.go` edit — loomengine stays untouched) AND names the config-load fault (so an operator sees the real cause, not phantom junction-drift-repair). Replace `l.HostJunctionsHere()` at `:79` with `l.HostJunctionsHere(names)`. Keep the signature `PairInSync(l *hubgeometry.Layout) (ok bool, reason string, err error)` unchanged so `preflight.go:120` is untouched. Preserve the existing drift `reason` strings verbatim. Reading from the weft base (not through the host junction) is what keeps the existing `TestPairInSync_JunctionDriftShapes` drift reasons correct when the `_lyx` junction is the broken one. Relax the "stateless, consults no registry" godoc note to "loads `fabric.yaml` for the junction name-set; still consults no registry/status.md," and document that a config-load failure is reported as a junction-check-unavailable reason (not a hard error, and deliberately containing the `"junction"` substring the preflight classifier keys on).
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
- **Requirements:** In `checkJunctionHealth(hostLayout)` (`internal/fabricengine/reconcile.go:323`, returns `(bool, string)`), before the loop load `names, err := junctionNames(filepath.Join(hostLayout.WeftWorktree(), hostLayout.RelPath))` (the weft base — durable and independent of the `_lyx` junction whose health this function checks); on error return `false, fmt.Sprintf("host junction check unavailable: cannot load fabric.yaml: %v", err)` — identical wording to card 8's `PairInSync` config-load reason (the two are documented twins), naming the config-load fault so an operator is not misdirected toward junction-drift repair (this reason is surfaced through `status.go:149`→`pair.JunctionReason`; `status.go` has no `check3BlocksSeed`-equivalent classifier, but keeping the wording identical to `PairInSync`'s keeps the twins consistent). Replace `hostLayout.HostJunctionsHere()` at `:324` with `hostLayout.HostJunctionsHere(names)`. In `junctionRepointedDetail(hostLayout)` (`:364`, returns `string`), load the same names from the same weft base; on error return `"junction re-pointed: cannot load fabric.yaml: " + err.Error()` (this function only runs after `checkJunctionHealth` found the pair unhealthy AND `WireJunctions` succeeded, so config is present in practice — the error branch is defensive). Replace `hostLayout.HostJunctionsHere()` at `:365` with `hostLayout.HostJunctionsHere(names)`. `hostLayout` is the per-host-worktree layout built by `hostLayoutFor`, so `hostLayout.WeftWorktree()` resolves that pair's weft sibling. Keep both function signatures unchanged.
- **Commit:** `feat(fabricengine): source junction-health names from fabric config with distinct load-failure reason`

### Card 10: Pass pathspec names into Add's reserved-name check

- **Context:**
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/config.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At `internal/fabricengine/add.go:119`, change `hubgeometry.IsReservedHubName(slug)` to `hubgeometry.IsReservedHubName(slug, t.cfg.Dirs())` — the acting worktree's already-loaded config (the new slug's own config does not exist yet at add time), passing the RAW unfiltered `Dirs()` (the reserved union must include every `pathspec` junction name, so appending a name to `pathspec` also reserves it as a slug). Do not call the `junctionNames` helper here — its wiring-guard filter would wrongly drop hub-reserved names from the reserved union. Update the reserved-name list in the step-0 comment (`add.go:51`, currently `_lyx, _raddle, _board, _portals, _launchers`) and the reserved-name prose comment (`add.go:111-118`) to state that the junction/weft-backed names now come from `fabric.yaml` `pathspec` (unioned with the hub-structural `hubgeometry.HubReservedNames()` set), rather than a fixed five-name list. **Fix the existing test in the same card:** `TestAdd_RejectsReservedHubNameSlug` (`add_test.go:124`) constructs `fabricengine.NewTopology(fabricengine.Config{})` (empty `Config`, so `t.cfg.Dirs()` is empty); after card 1 removes `_lyx`/`_pattern` from `HubReservedNames()`, its `{"LyxDir", "_lyx"}` sub-case would get `IsReservedHubName("_lyx", [])` == false and fail. Change that `Topology` construction to `fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _pattern"})` so `_lyx`/`_pattern` stay rejected via the injected `pathspec` union while `_board`/`_portals`/`_launchers`/`_raddle` stay rejected via `HubReservedNames()`. This MUST land in batch 2 (not batch 3's card 14) because batch 2's own `verify:` runs `./internal/fabricengine/` and would otherwise fail on this pre-existing test.
- **Commit:** `feat(fabricengine): union pathspec junction names into Add slug-reservation check`

### Card 11: Migrate initengine callers (init genesis + undo) to supply names

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junction.go`
  - `internal/configsync/configsync.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/initengine/init.go`
  - `internal/initengine/undo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Both `initengine` callers of the now-`names`-taking mutating primitives must supply the wired name-set via the exported `fabricengine.WiredNames(baseDir)`. **`init.go`:** `Init` (`:51`) calls `fabricengine.WireJunctions(l, slug)` at `:91` and writes config via `configsync.ReconcileAll(cwd, true)` at `:131`. Since `Init` must now obtain `names` to pass to `WireJunctions`, and on a genesis first-ever init no `fabric.yaml` exists yet, reorder so config is materialised on the weft base first: after the weft-pairing existence check (`:62-65`) and before wiring, compute `weftBase := filepath.Join(l.WeftWorktree(), l.RelPath)`, `os.MkdirAll(hubgeometry.ConfigDir(weftBase), 0o755)`, `results, err := configsync.ReconcileAll(weftBase, true)`, then `names, err := fabricengine.WiredNames(weftBase)`, then `fabricengine.WireJunctions(l, slug, names)`. Remove the now-redundant post-wire `configsync.ReconcileAll(cwd, true)` at `:131` and feed `InitResult.Modules` from the pre-wire `results` (the `result.Modules` loop at `:136-146` stays). Note there is NO chicken-and-egg any more — `WireJunctions` itself never reads config; the pre-seed exists only so `Init` has a `pathspec` to read for `names`. Verified rationale for weft-base (not `cwd`) write: after wiring, `cwd/_lyx` and `weftBase/_lyx` are the same physical dir (junction), so the reconcile result is identical; writing to `cwd` before wiring would trip `seedLyxJunction`'s host-pristine guard; `ReconcileAll` preserves the legacy warp/weft→fabric migration. Leave the host-side `os.MkdirAll(lyxDir/patternDir)` (`:103-108`), `ConfigDir(cwd)` mkdir (`:110-114`), and `.gitignore` handling in place. Do not change `Init`'s signature or `InitResult` shape. **`undo.go`:** at `:81`, change `fabricengine.UnwireJunctions(l, slug)` to load names first — `names, err := fabricengine.WiredNames(filepath.Join(l.WeftWorktree(), l.RelPath))` (undo runs on an initialised worktree, so the weft config is present; propagate the error) — then `fabricengine.UnwireJunctions(l, slug, names)`.
- **Commit:** `fix(initengine): supply wired names to Wire/Unwire; materialise config before genesis wiring`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/ ./internal/initengine/ ./internal/configcli/ ./internal/loomengine/` covers every package with a `WireJunctions`/`UnwireJunctions` call site, so the compiler-forced signature migration (card 6 + card 11) is fully verified — a missed call site fails the build. The signature change means each of those packages does not compile until its call sites gain the `names` argument, which is exactly why they are all migrated in this batch and all appear in the verify scope. Existing `fabricengine` integration tests (`junction_pattern_integration_test.go`, `junction_repoint_test.go`, `reconcile_stale_registration_test.go`, etc.) then validate that caller-supplied `["_lyx","_pattern"]` reproduces today's wiring with no behavior change; because `WireJunctions` no longer reads config, the previously-breaking `MaterialisesMissingWeftTarget` (deletes the weft `_lyx` target) and `TestRemove_TearsDownNestedJunction` (`RelPath == "sub"`) tests now pass simply by supplying names — no config seeding, and self-heal of the absent target is preserved. `initengine`'s init/undo tests validate the reorder + name-load. `-tags integration` is required for the git-spawning fixtures. New proof tests (extensibility, wiring-guard filter, no-fallback error) are added in batch 3.
