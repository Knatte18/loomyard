# Discussion: fabric: config-driven junction list

```yaml
task: 'fabric: config-driven junction list'
slug: fabric-junction-config
status: discussing
parent: main
```

## Problem

`fabric` wires host↔weft directory junctions (`_lyx`, `_pattern`) and enumerates them in Go source. Today the wired junction set is **hardcoded** in `internal/hubgeometry` (`HostJunctions`/`HostJunctionsHere` return literal `_lyx` + `_pattern` records), and the slug-collision predicate `IsReservedHubName` hardcodes six names. Adding a new weft-backed module that wants its own junction (the planned `_raddle`, or a future per-worktree `_board`) therefore means **editing `hubgeometry` and/or `fabricengine` Go code** — a code change fanning across the wiring path, the unwiring path, the health checks, and the reserved-name guard.

This is **slice 1 of the `fabric` V2 campaign** (`manifest/designs/fabric-unified-view.md`, "Build order", slice 1): make the junction name-set **config-driven**. The goal, stated by the operator: when raddle and a future board junction are built, they should be able to **add their junction name to a config file** and get the junction wired — with **no rewrite of `fabric` or `hubgeometry`**. `hubgeometry` keeps owning path *construction*; only the *name set* is injected from fabric config. The design flags this slice as "small, near-mechanical, independent." Sequenced after the already-landed `board: move storage to weft:main` and `native clients` items; it does not touch clone (that is slice 4).

## Scope

**In:**

- Reuse the **existing `fabric.yaml` `pathspec` key** as the single, canonical list of weft-backed folder names. It already exists (`pathspec: _lyx _pattern` in `internal/fabricengine/template.yaml`) and already drives weft-sync staging. This slice makes the **same list also drive junction wiring** and the reserved junction-name portion of slug validation.
- Change `internal/hubgeometry` `HostJunctions`/`HostJunctionsHere`/`IsReservedHubName` to take the junction name-set as an **injected `[]string` parameter** and compute paths generically (`filepath.Join(base, RelPath, name)` per name). Remove the hardcoded `_lyx`/`_pattern` junction-record literals from `hubgeometry`.
- Add a `fabricengine` helper that sources the names from `LoadConfig(baseDir).Dirs()` and pass them into the hubgeometry methods at every fabric call site. All `fabricengine` consumer signatures (`WireJunctions`, `UnwireJunctions`, `removeHostJunction`, `PairInSync`, `checkJunctionHealth`, `junctionRepointedDetail`) **keep their current signatures**; the helper is called internally.
- Make `IsReservedHubName` = (config junction names) ∪ (hardcoded hub-reserved names `{_board, _portals, _launchers, _raddle}` = today's set minus the config-migrated `_lyx`/`_pattern`). The union equals exactly today's six reserved names — no reservation lost. The hardcoded names stay owned by `hubgeometry` for slug-collision; only the junction/weft-backed portion comes from config.
- Guard the wiring loop: the `fabricengine` junction-name helper filters out the hardcoded hub-reserved names before wiring, so a hub-level name mis-added to `pathspec` can never wire a colliding per-worktree junction.
- Update tests, godoc (`hubgeometry`, `fabricengine/doc.go`), `docs/overview.md` (the junction-wiring section), and the **Hub Geometry Invariant** in `CONSTRAINTS.md` (amend to record that the junction name-set is injected from fabric config while path construction and the hub-structural tokens stay hubgeometry-owned) — all in the same commit(s) per the Documentation Lifecycle rule.

**Out:**

- **`Fabric.Commit`, snapshot-as-trailer, clone-does-everything, subpath-in-weft, warp-rebase/reconcile** — slices 2–5 of the campaign. Not touched.
- **Adding a `_raddle` junction** — the raddle module does not exist yet. This slice adds no new junction; it only makes the *existing* `_lyx`+`_pattern` set config-driven so raddle can append later with zero code change.
- **A new per-worktree `_board` junction** — board storage currently lives at the hub level (`<hub>/_board`, on `weft:main`), not as a per-worktree junction. If a future board rework wants a per-worktree junction, it appends to the same `pathspec` list like raddle will. Not in scope now.
- **A new config key** — deliberately not introduced; we reuse `pathspec` (see Decisions → config-key). This avoids any migration break.
- **`_portals` / `_launchers` / `_board` / `_raddle`** — `_portals`/`_launchers` are **hub-level mirror directories** (`<hub>/_portals/<...>`, `<hub>/_launchers/<...>`); `_board` is the hub-level board dir; `_raddle` is a known-future junction with no wiring yet. None are per-worktree weft junctions to be wired in this slice, and none go in `pathspec` now. All four stay hardcoded in `hubgeometry`'s hub-reserved set (slug-collision only). Not migrated to config.
- **Migration / backfill of narrow-`pathspec` worktrees** — no automated detection or backfill (see Decisions → migration).
- **Changing weft-sync staging semantics** — `pathspec` continues to drive staging exactly as today; we only additionally read it for wiring.

## Decisions

### config-key: reuse `pathspec`, do not add a new key

- **Decision:** The junction name-set **is** the `fabric.yaml` `pathspec` list. One list drives weft-sync staging (existing behavior), junction wiring (new), and the reserved junction-name set (new). No new config key is added. A new weft-backed module extends the set by appending its dir name to the `pathspec` default in `internal/fabricengine/template.yaml` (one line) — after which it is automatically wired, weft-synced, and slug-reserved with no `fabric`/`hubgeometry` code change. **This appendable template default is the "template new weft-backed modules append to" the design refers to.**
- **Rationale:** Zero migration friction — no new key means no missing-key `Load` error and no forced `lyx config reconcile` on existing worktrees (contrast a new `junctions:` key, which `configengine.Load`'s `MissingKeys` check would reject until reconcile runs on every worktree). One source of truth. Matches the design's "grows out of today's `fabric.yaml` (`pathspec` key already there)." It also makes today's *accidental* reality principled: a pre-`_pattern` worktree with `pathspec: _lyx` already lacks the `_pattern` junction (documented in `fabricengine/doc.go`); reading the same list for wiring makes "you wire what you sync" the intended rule rather than a coincidence.
- **Rejected:** A dedicated `junctions:` key separate from `pathspec` — cleaner "wire vs stage" separation, but forces a config-reconcile migration across every existing worktree and creates two near-identical lists to keep in sync. YAGNI: nothing today needs the junction set to diverge from the sync set.

### injection: thread `names []string` into hubgeometry; hubgeometry stays config-blind

- **Decision:** `hubgeometry` gains **no** config dependency. Its junction methods take the name-set as a parameter:
  - `HostJunctions(slug string, names []string) []HostJunction`
  - `HostJunctionsHere(names []string) []HostJunction`
  - `IsReservedHubName(name string, junctionNames []string) bool`
  Each builds records generically: `Name: n`, `Link: filepath.Join(WorktreePath(slug), RelPath, n)` (or `WorktreeRoot` for the Here variant), `Target: filepath.Join(WeftWorktreePath(slug), RelPath, n)` (or `WeftWorktree()` for Here) — the exact shapes the current per-name accessors already produce (`HostLyxLink`/`WeftLyxDirFor`/`HostPatternLink`/`WeftPatternDirFor`, verified uniform). The hardcoded `_lyx`/`_pattern` junction-record literals are **removed** from `HostJunctions`/`HostJunctionsHere`.
- **Rationale:** Preserves the Hub Geometry Invariant's spirit — `hubgeometry` remains the sole owner of *path composition* and still owns the hub-structural tokens, while the *name set* is injected. Keeps `hubgeometry` a near-leaf (imports only `gitexec` today). Keeps the change mechanical and testable (methods are pure functions of their inputs).
- **Rejected:** `hubgeometry` reads `fabric.yaml` itself — couples geometry to `configengine` and the fabric config location, and `Layout` (built by `Resolve`, no config I/O) has no clean config seam. A mutable `Layout.JunctionNames` field set post-`Resolve` — introduces mutable geometry state and a nil-default ambiguity; rejected in favor of explicit params.

### name-sourcing: fabric loads config internally; consumer signatures stay stable

- **Decision:** A single `fabricengine` helper (e.g. `junctionNames(baseDir string) ([]string, error)`) returns `LoadConfig(baseDir).Dirs()`. Every fabric consumer that needs the name-set calls this helper (baseDir from the `Layout`) and passes the result to the hubgeometry methods. `WireJunctions(l, slug)`, `UnwireJunctions(l, slug)`, `removeHostJunction(l, slug)`, `PairInSync(l)`, `checkJunctionHealth(l)`, `junctionRepointedDetail(l)` **keep their current signatures**; `internal/loomengine/preflight.go`, `internal/fabricengine/status.go`, and `internal/fabricengine/reconcile.go` callers are untouched. The slug-validation caller `internal/fabricengine/add.go` loads config for its `IsReservedHubName` call.
- **Rationale:** Most contained. `hubgeometry` stays pure (params), fabric owns loading its own config, and no signature churn propagates up into `loomengine`. `PairInSync`'s "stateless" godoc note relaxes to "loads `fabric.yaml` for the junction name-set; still consults no registry/status.md."
- **Rejected:** Threading `names []string` all the way up through `PairInSync(l, names)` into `loomengine` preflight — more explicit but spreads fabric-config-loading into `loomengine` and changes a consumed signature/string-format contract for no real gain.

### no-fallback: a config-load failure is a surfaced error, never a silent default

- **Decision:** When a site that needs the junction names cannot load `fabric.yaml`, that is an **error** — propagated/surfaced, never silently substituted with a hardcoded default. The helper returns `([]string, error)`; consumers propagate it (`PairInSync` returns its existing `err`; a `(bool, string)` health function surfaces it as an explicit unhealthy reason rather than swallowing it; wiring/unwiring return the error as they already do). The `pathspec` **template default** (`_lyx _pattern`) applies **only at config-scaffold time** (new-worktree creation via `configengine`), exactly as today — it is not a runtime fallback. An initialized worktree always has a `pathspec` value on disk, so no runtime default is ever needed in normal operation.
- **Rationale:** Operator's explicit direction: a missing/unloadable `fabric.yaml` at a name-needing site is a genuine broken state and must be caught, not papered over with a guessed default that could wire the wrong set. A defensive default is a crutch that hides real breakage.
- **Rejected:** Fabric helper falls back to the template default on load failure; `hubgeometry` keeps a hardcoded default when `names` is nil. Both re-introduce a silent wrong-answer path and (for the latter) the very literals this slice removes.
- **Note for mill-plan:** `hubgeometry`'s own unit tests build a `Layout` and pass `names` explicitly — no config load, no error path there. The no-fallback rule concerns only the `fabricengine` helper and its consumers, which run in initialized-worktree contexts (real fixtures in integration-tagged tests).

### reserved-names: config junctions ∪ hardcoded hub-reserved (incl. `_raddle`)

- **Decision:** `IsReservedHubName(name, junctionNames)` returns true if `name` is in `junctionNames` (from config) **or** in a hardcoded hub-reserved set still owned by `hubgeometry`. That hardcoded set is defined as **today's `IsReservedHubName` set minus the names now sourced from config** (`_lyx`, `_pattern`) — i.e. `{_board, _portals, _launchers, _raddle}`. The union therefore equals **exactly today's six reserved names** — no reservation is lost. Appending a name to `pathspec` automatically reserves it as a slug too (no separate edit); the union is idempotent, so a name that is both hardcoded and in config is harmless.
- **`_raddle` specifically:** kept in the hardcoded set as a **known-future junction name reserved ahead of wiring** (it already has `hubgeometry.WeftRaddleDir` and is the design's next junction consumer; `hubgeometry.go:426` reserves it today). No `_raddle` junction is wired now (honors Q4 — raddle does not exist). When raddle is built, it appends `_raddle` to the `pathspec` default (getting it wired + synced), and the hardcoded `_raddle` entry can be dropped at that point (the union already keeps it reserved either way).
- **Rationale:** Preserving every current reservation is a strict requirement — dropping `_raddle` would let a worktree be named `_raddle` and collide when raddle later appends to `pathspec` (the exact regression the r1 review caught). The dangerous hub-level collisions (`_portals`, `_launchers`, `_board`) stay geometry-intrinsic; `_raddle` rides the same hardcoded set until it graduates to config.
- **Rejected:** (a) Making the entire reserved set config-driven — would force `_board`/`_portals`/`_launchers` into config even though no module appends them and their geometry is hub-level. (b) Adding `_raddle` to `pathspec` now — would wire an empty `_raddle` junction on every worktree, contradicting Q4. (c) Accepting `_raddle`'s de-reservation — a real (if low-probability) collision risk for no benefit.

### wiring-guard: never wire a junction for a hub-reserved name

- **Decision:** The `fabricengine` junction-name helper **filters out the hardcoded hub-reserved names** (`_board`, `_portals`, `_launchers`, `_raddle`) from the list it feeds to `WireJunctions`/`HostJunctions`, even if an operator mis-adds one to `pathspec`. Junction wiring only ever acts on genuine per-worktree weft-backed names.
- **Rationale:** `_board`/`_portals`/`_launchers` have **hub-level** geometry (`<hub>/<name>/…`), not per-worktree (`<worktree>/<RelPath>/<name>`). Wiring a per-worktree junction for one of them would create a directory colliding with the hub-level dir — a silent operator-error footfault the reserved-set union does not itself prevent (it stops the name being a *slug*, not being a *pathspec entry*). The filter closes that hazard cheaply and keeps "the correct path is the easy path, mistakes are inert" (per `docs/overview.md`). `_raddle` is filtered too until it graduates to a real junction, so a stray `_raddle` in `pathspec` stages content (harmless) but does not wire a premature junction.
- **Rejected:** Consciously accepting the mis-wire as pure operator error and only documenting it — the guard is a one-line filter and removes a whole class of confusing breakage.
- **Note for mill-plan:** the filter set is exactly the hardcoded hub-reserved set from the reserved-names decision — keep the two definitions sharing one source (a single `hubgeometry` accessor or `fabricengine` const), not two drifting lists.

### junction-order: `pathspec` token order is authoritative

- **Decision:** Junction order (observable via `UnwireResult.JunctionsRemoved` and first-unhealthy-wins in the health checks) follows **`pathspec` token order**. The default `pathspec: _lyx _pattern` keeps `_lyx` first; the helper does **not** force a sort or pin `_lyx`. `_lyx`-first is a property of the default value, not an enforced invariant.
- **Rationale:** No code depends on `_lyx` being *first* specifically — the order is documented as "observable," not correctness-critical (`_lyx`'s primacy today is just its position in the default list). Making config order authoritative is the simplest honest rule and matches "you wire what you list, in the order you list it." YAGNI: pinning `_lyx` first would add sort logic guarding a case nothing needs.
- **Rejected:** Forcing `_lyx` to sort first regardless of `pathspec` order — unnecessary machinery; if a future need to pin `_lyx` first emerges, add it then.
- **Note for mill-plan:** update the `hubgeometry` godoc on `HostJunctions`/`HostJunctionsHere`/`UnwireResult` that currently asserts "`_lyx` first deliberately" to state that order follows the injected name list (default keeps `_lyx` first).

### migration: none

- **Decision:** No migration step, no automated backfill, no new diagnostic. Because `pathspec` is reused (no new key), existing worktrees keep working unchanged. A pre-`_pattern` worktree with `pathspec: _lyx` wires only `_lyx` and reserves only `_lyx` among junction names — which already matches its current on-disk reality (it lacks the `_pattern` junction today). Widening remains: edit `pathspec` + `lyx fabric reconcile`, as today. Document the behavior in `fabricengine/doc.go`/godoc.
- **Rationale:** Reusing `pathspec` makes migration a non-event; the existing `doc.go` asymmetry note already covers the "narrow pathspec ⇒ missing junction, reconcile is the remedy" story.
- **Rejected:** Active detection/backfill for narrow-`pathspec` worktrees — a new diagnostic class in `fabric status`, out of scope and unnecessary here.

## Technical context

**The three overlapping lists today (important — the design doc conflates them):**

1. `internal/hubgeometry/hubgeometry.go` `HostJunctions(slug)` / `HostJunctionsHere()` — the **actual wired junction set**: two records, `_lyx` then `_pattern` (order is observable — `UnwireResult.JunctionsRemoved` documents it and health checks are first-unhealthy-wins). Each record is `{Name, Link, Target}`.
2. `internal/fabricengine/template.yaml` `pathspec: _lyx _pattern` — the **weft-sync staging list**, already config-driven, read via `fabricengine.LoadConfig(baseDir)` → `Config.Dirs()` (splits on whitespace). This is the key we reuse.
3. `internal/hubgeometry/hubgeometry.go` `IsReservedHubName(name)` — **slug-collision predicate**: `_lyx, _raddle, _board, _portals, _launchers, _pattern`. Mixes per-worktree junctions and hub-structural dirs. Note `_raddle` is listed here but has **no junction wired** (see `internal/fabricengine/status.go`: "no junction is wired for _raddle in this release", report-only in host-pollution scans).

**Uniform junction geometry (verified):** every host link is `filepath.Join(WorktreePath(slug), RelPath, <name>)` (Here variant: `WorktreeRoot`), and every weft target is `filepath.Join(WeftWorktreePath(slug), RelPath, <name>)` (Here variant: `WeftWorktree()`). So a generic name→record loop reproduces the current records exactly. The existing per-name accessors (`HostLyxLink`, `HostLyxLinkHere`, `WeftLyxDir`, `WeftLyxDirFor`, `HostPatternLink`, `HostPatternLinkHere`, `WeftPatternDir`, `WeftPatternDirFor`) may remain (other callers may use them) or be trimmed at mill-plan's discretion; `WeftRaddleDir` and the `_raddle` string in `status.go` are unrelated and stay.

**Call sites consuming the junction name-set (all in `internal/fabricengine`, plus one hubgeometry predicate caller):**

- `WireJunctions(l, slug)` → `seedLyxJunction` + `seedGitExclude`, both loop `l.HostJunctions(slug)`. Callers: `internal/initengine/init.go`, `internal/fabricengine/checkout.go`, `internal/fabricengine/reconcile.go`.
- `UnwireJunctions(l, slug)` → `unseedLyxJunction`/`unseedGitExclude`, loop `l.HostJunctions(slug)`. Caller: `internal/initengine/undo.go`.
- `removeHostJunction(l, slug)` → `l.HostJunctions(slug)` (`internal/fabricengine/weftwiring.go`). Caller: `internal/fabricengine/remove.go`.
- `PairInSync(l)` → `l.HostJunctionsHere()` (`internal/fabricengine/drift.go`). Caller: `internal/loomengine/preflight.go:120` (consumes the reason strings — keep wording).
- `checkJunctionHealth(l)` + `junctionRepointedDetail(l)` → `l.HostJunctionsHere()` (`internal/fabricengine/reconcile.go`). Callers: `reconcile.go` and `internal/fabricengine/status.go:149`.
- `IsReservedHubName(slug)` (`internal/fabricengine/add.go:119`) — load config here for `junctionNames`.

**Config plumbing:** `fabricengine.LoadConfig(baseDir)` (`internal/fabricengine/config.go`) calls `configengine.Load(baseDir, "fabric", ConfigTemplate())` (strict; errors "not initialized here; run lyx init" when `<baseDir>/_lyx/` is absent), unmarshals to `Config{BranchPrefix, Pathspec}`, and `Config.Dirs()` returns `strings.Fields(Pathspec)`. `configengine.Load` → `yamlengine.MissingKeys` rejects a file missing any template key (hence: no new key). `yamlengine.Reconcile` adds absent template keys and preserves existing values.

**Enforcement interaction:** `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) bans the tokens `_board -weft -HUB _portals _launchers _raddle _lyx _pattern` as **literals in path-construction context** (a `filepath.Join` arg, a `+` operand, or a `const` string) in production Go **outside `hubgeometry`**. Design implications: (a) `hubgeometry` still legally holds the hub-structural token consts and now composes junction paths from a runtime `name` variable — fine; (b) `fabricengine` must source names from **runtime config values** / the parsed template — never introduce a `filepath.Join(x, "_lyx")`-style literal or a `[]string{"_lyx","_pattern"}` const path in fabric Go. The `pathspec: _lyx _pattern` default lives in `template.yaml` (YAML, not scanned). The comment reference `add.go:51` listing the reserved names is a comment (stripped before scanning) but should be updated for accuracy.

## Constraints

From `CONSTRAINTS.md` (authoritative):

- **Hub Geometry Invariant** — `internal/hubgeometry` owns all cwd/geometry and path construction; geometry tokens (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, `_pattern`) may be used in path-construction context **only** inside `hubgeometry`. Enforced by `enforcement_test.go`. **This slice must amend this invariant's prose** (same commit) to record: `hubgeometry` still owns path construction and the hub-structural tokens, but the **weft-backed junction name-set is injected from fabric config (`pathspec`)** rather than enumerated in `hubgeometry`. The machine check stays green (no new literals introduced anywhere).
- **CLI/Cobra Invariant** — no CLI surface changes expected in this slice (no new command/flag). If any `Short`/`Long` help text references the junction pair, re-read and correct it (review obligation). `fabricengine`/`fabriccli` split unchanged.
- **Weft Git Invariant** — untouched; this slice does not change weft git operations, only which folder names get wired/reserved.
- **Documentation Lifecycle** — docs land in the same commit: `hubgeometry` godoc (the three methods), `fabricengine/doc.go` (junction-set now config-driven; extension contract), `docs/overview.md` (the "Junctions" section, lines ~95–97, currently hardcodes `_lyx`/`_pattern` — reframe as the `pathspec`-driven set), and `CONSTRAINTS.md` Hub Geometry Invariant amendment. `manifest/roadmap.md` does **not** move for a single slice — the whole `fabric-unified-view` item stays Planned until the campaign completes; the design doc's Build-order is the slice tracker.

Discovered during discussion:

- **No silent defaults** (Decisions → no-fallback): config-load failure at a name-needing site is a surfaced error.
- **Reserved-set semantics** (Decisions → reserved-names): hub-structural stays hardcoded; junction names come from config; the two are unioned.

## Testing

Follow `golang:golang-testing` conventions; respect the **Test Tier Purity** and **Hermetic Git Test Environment** invariants (junction-wiring tests spawn git ⇒ integration-tagged with a `TestMain` calling `lyxtest.HermeticGitEnv()`).

**`internal/hubgeometry` (pure, Tier-1, TDD candidates):**

- `HostJunctions(slug, names)` / `HostJunctionsHere(names)` — table test: given `names`, produces the correct `{Name, Link, Target}` records in input order; `["_lyx","_pattern"]` reproduces today's exact two records (regression lock against the pre-change literals); an empty list yields no records; a 3-name list (`["_lyx","_pattern","_extra"]`) yields three correctly-composed records. Update the existing `TestIsReservedHubName` and `TestIsReservedHubName_Pattern` to the new signature.
- `IsReservedHubName(name, junctionNames)` — hardcoded hub-reserved names (`_board`, `_portals`, `_launchers`, **`_raddle`**) reserved for **any** `junctionNames` (including empty — assert `_raddle` stays reserved even when it is not in `junctionNames`, the exact r1-review regression); names in `junctionNames` reserved; unrelated names not reserved. Include a regression assertion that the union over the default `junctionNames = ["_lyx","_pattern"]` reserves **exactly today's six names** (`_lyx`, `_pattern`, `_board`, `_portals`, `_launchers`, `_raddle`).
- `enforcement_test.go` still passes (no new literals) — implicitly covered by the whole `go test` run; no new assertion needed, but confirm it stays green after removing the `HostJunctions` literals.

**`internal/fabricengine` (integration-tagged where git spawns):**

- Name-sourcing helper — loads `pathspec` and returns `Dirs()`; **returns an error (no default) when `fabric.yaml` cannot be loaded** (the no-fallback rule — assert the error path explicitly).
- **Extensibility test (the key proof of the operator's goal):** in a real host+weft fixture, set `pathspec: _lyx _pattern _extra` and assert `WireJunctions` wires **all three** junctions (host links + weft targets + `.git/info/exclude` entries) with **no code change** — this is what demonstrates a future raddle/board can append and be wired. Correspondingly, `UnwireJunctions` removes all three.
- Health/drift respect the config list — `PairInSync` / `checkJunctionHealth` verify exactly the `pathspec`-listed junctions; a worktree with `pathspec: _lyx` (only) is reported healthy when only `_lyx` is wired (matching current narrow-pathspec reality), and unhealthy if a listed junction is missing/mispointed. Reason-string wording consumed by `loomengine` preflight is preserved.
- `add.go` slug validation rejects a slug equal to a hub-structural name and to any current `pathspec` junction name.

**Edge cases to cover or consciously accept:**

- Empty `pathspec` ⇒ no junctions wired (degenerate but not an error at the wiring loop; reserved set = hardcoded hub-reserved only). Accept.
- Duplicate/whitespace-noisy `pathspec` ⇒ `strings.Fields` normalizes whitespace; decide in mill-plan whether the helper de-dups (wiring is idempotent, so low-risk either way).
- **A hub-reserved name in `pathspec` (e.g. `_board`, `_raddle`)** — the wiring-guard filter (Decisions → wiring-guard) must drop it so **no junction is wired** for it; assert `WireJunctions` with `pathspec: _lyx _pattern _board` wires only `_lyx`+`_pattern`, never a `_board` junction. This is the r1-review NOTE made a hard test.
- **Junction order follows `pathspec`** (Decisions → junction-order) — assert `HostJunctions(["_pattern","_lyx"])` yields records in that given order (config order authoritative), and the default `_lyx _pattern` keeps `_lyx` first. No forced-sort assertion.

## Q&A log

- **Q:** The design says "replace the hardcoded reserved-name set `IsReservedHubName`", but the code has three overlapping lists and `IsReservedHubName` is slug-validation, not the wired set. Which list(s) does this slice config-drive? **A:** Config-drive the junction-wiring set (`HostJunctions`/`HostJunctionsHere`) and derive `IsReservedHubName`'s junction portion from it; keep `_board`/`_portals`/`_launchers` hardcoded as hub-structural.
- **Q:** Where does the config-driven junction list live — reuse `pathspec` or a new `junctions:` key? **A:** Reuse `pathspec` (single list drives staging + wiring + reserved junction-names); no new key, no migration break.
- **Q:** How is the name-set injected into `hubgeometry`? **A:** Thread `names []string` params into `HostJunctions`/`HostJunctionsHere`/`IsReservedHubName`; `hubgeometry` stays config-blind and holds no junction-name literals.
- **Q:** Is `_raddle` (and a new `_board` junction) in scope? **A:** No — raddle does not exist yet and board's per-worktree junction does not exist. The whole point is that when raddle and board are built, they append their junction name to the config file and get wired **without** rewriting fabric. The `pathspec` template default is that append point.
- **Q:** How do the read-only health functions (`PairInSync` in loom preflight, `checkJunctionHealth`) source the names? **A:** Fabric loads its own config internally via a helper; consumer signatures (incl. `PairInSync(l)`) stay stable; `loomengine` untouched.
- **Q:** What is the fallback when `fabric.yaml` can't be loaded at a name-needing site? **A:** There is none — that is a real error, surfaced/propagated, never silently defaulted. The template default applies only at scaffold time, as today.
- **Q:** Any migration for existing worktrees? **A:** None — reusing `pathspec` means no forced reconcile; a narrow-`pathspec` worktree already matches its on-disk junction reality. Document, don't backfill.
- **Q:** (r1 review gap) The new `IsReservedHubName` drops `_raddle` from the reserved set — a worktree could be named `_raddle` and collide when raddle later appends to `pathspec`. Keep it reserved or accept removal? **A:** Keep it. The hardcoded hub-reserved set is defined as "today's set minus the config-migrated `_lyx`/`_pattern`" = `{_board, _portals, _launchers, _raddle}`, so the union over the default `pathspec` reserves exactly today's six names. No `_raddle` junction is wired (honors Q4); `_raddle` is reserved as a known-future junction name until raddle graduates it to `pathspec`.
