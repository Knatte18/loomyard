# Batch: proofs-and-docs

```yaml
task: 'fabric: config-driven junction list'
batch: proofs-and-docs
number: 3
cards: 4
verify: go test -tags integration ./internal/fabricengine/
depends-on: [2]
```

## Batch Scope

This batch adds the tests that prove the operator's goal — a future module appends its junction name to `pathspec` and is wired with no fabric/hubgeometry code change — plus the no-fallback and wiring-guard proofs, and lands the fabricengine/overview documentation. It is one batch because every card validates or documents batch-2 behavior and shares the same fabricengine test-fixture context (`lyxtest.CopyPairedLocal` / `lyxtest.SeedConfig`).

Batch-local decisions: proof tests seed the fabric config on the WEFT side (`lyxtest.SeedConfig(t, fixture.WeftPrime, ...)`, matching the existing integration tests and the weft-base load decision); the extensibility proof uses an `_extra` name that is neither a hub-reserved token nor part of the default pathspec.

## Cards

### Card 12: Extensibility and narrow-pathspec health integration tests

- **Context:**
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/template.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create a `//go:build integration` test file (package `fabricengine_test`, mirroring `junction_pattern_integration_test.go`'s imports and its `lyxtest.CopyPairedLocal(t)` fixture pattern). Add two tests. (1) `TestWireJunctions_WiresEveryPassedName` — the extensibility proof under the hybrid seam: `fixture := lyxtest.CopyPairedLocal(t)`, then `WireJunctions(l, slug, []string{"_lyx", "_pattern", "_extra"})` — passing the name-set directly, exactly as a caller sourcing an extended `pathspec` would (NO `SeedConfig` needed, because `WireJunctions` no longer reads config). Assert the host link, weft target, and `.git/info/exclude` entry (via the existing `readExcludeLines` helper) exist for all three names `_lyx`, `_pattern`, `_extra` — the proof that a future raddle/board append (one extra `pathspec` token) is wired with no `fabric`/`hubgeometry` code change; then `UnwireJunctions(l, slug, []string{"_lyx", "_pattern", "_extra"})` removes all three (assert `UnwireResult.JunctionsRemoved` contains all three and each host link is gone). Compose the `_extra` host link / weft target via `l.HostJunctions(slug, []string{"_extra"})[0].Link`/`.Target` (or `filepath.Join(l.WorktreePath(slug), l.RelPath, "_extra")`). (2) `TestPairInSync_NarrowPathspecIsHealthy` — `PairInSync` still loads config internally, so this test seeds it: `lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{"fabric": <template with pathspec: _lyx>})`, wire only `_lyx` (`WireJunctions(l, slug, []string{"_lyx"})`), and assert `PairInSync(l)` reports the pair healthy (`ok == true`) when only `_lyx` is wired (narrow-pathspec reality). CRITICAL fixture setup for test (2): `PairInSync` checks weft-branch correspondence (`weftBranch == WeftBranchName(hostBranch)`, `drift.go:69-72`) BEFORE the junction loop, and raw `CopyPairedLocal` leaves the weft prime on `main` (not `main-weft`), so without extra setup `PairInSync` returns `(false, "host on main, weft on main (want main-weft)")` and the assertion fails — before wiring, run `lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))` (the `TestPairInSync_JunctionDriftShapes` pattern, `junction_pattern_integration_test.go:~400`). Test (1) calls only `WireJunctions`/`UnwireJunctions` (no branch check, no config load) and needs neither `SeedConfig` nor the branch checkout. Assert health only through EXPORTED surfaces (`PairInSync`, or `Topology.Status`/`Reconcile`); do NOT call the unexported `checkJunctionHealth` from this external `fabricengine_test` package. (The wiring-guard is now a caller-side filter, proven by `filterHubReserved`'s unit test in card 13, not at the `WireJunctions` level — `WireJunctions` wires exactly the names it is given.)
- **Commit:** `test(fabricengine): prove caller-supplied name-set wiring and narrow pathspec health`

### Card 13: Name-helper unit tests — no-fallback error and hub-reserved filter

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/junctionnames_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/junctionnames_test.go` (package `fabricengine`, so it can call the unexported `junctionNames` and `filterHubReserved`; untagged unit test). Add: (1) a test that `junctionNames(baseDir)` returns a non-nil error and a nil slice when `baseDir` has no loadable `fabric.yaml` (point it at an empty temp dir with no `_lyx/`) — the no-fallback rule: a config-load failure is surfaced, never defaulted to `_lyx _pattern`. (2) a table test for `filterHubReserved`: an input containing each of `hubgeometry.HubReservedNames()` (`_board`, `_portals`, `_launchers`, `_raddle`) has those dropped while non-reserved names (`_lyx`, `_pattern`, `_extra`) survive in input order; an all-reserved input yields an empty result; an input with no reserved names is returned unchanged. Keep this test hermetic (no git spawn) so it stays a Tier-1 unit test — do not use the paired-worktree fixture here.
- **Commit:** `test(fabricengine): cover junctionNames no-fallback error and hub-reserved filter`

### Card 14: Add-slug reservation test for a pathspec junction name

- **Context:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/config.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/add_test.go`, ADD new reserved-slug coverage (near `TestAdd_RejectsReservedHubNameSlug`, `:124`) proving that a slug equal to a current `pathspec` junction name is rejected: with a `Topology` whose `cfg.Pathspec` includes a name such as `_extra`, `Add` rejects the slug `_extra` with the "reserved for lyx hub geometry" error via the new `IsReservedHubName(slug, t.cfg.Dirs())` union (a name that is NOT in `hubgeometry.HubReservedNames()` and is reserved only because it is in `pathspec`) — proving the union's config-driven arm, not only the hub-structural arm. Note: card 10 (batch 2) already updated the existing `TestAdd_RejectsReservedHubNameSlug` `Topology` construction to `Config{Pathspec: "_lyx _pattern"}`; this card only adds the new `pathspec`-only case, it does not re-touch the `LyxDir` sub-case. Keep the test at the tier the existing test uses (rejection at step 0 before any git operation, so no fixture is needed).
- **Commit:** `test(fabricengine): reject Add slug equal to a pathspec junction name`

### Card 15: Document the config-driven junction set (doc.go + overview.md + design doc)

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `docs/overview.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`, add prose stating the wired junction set is now sourced from `fabric.yaml`'s `pathspec` key (the same list that drives weft-sync staging), filtered against `hubgeometry.HubReservedNames()`, so a new weft-backed module is wired by appending its dir name to the `pathspec` template default with no `fabric`/`hubgeometry` code change; keep the existing narrow-pathspec asymmetry note accurate. In `docs/overview.md`: update the `Layout` method list at line 40 (`HostJunctions(slug)` → `HostJunctions(slug, names)`); reframe the "Junction model" section (lines ~95–101) so the wired set is described as the `pathspec`-driven list rather than a hardcoded `_lyx`/`_pattern` pair, retaining the concrete `<host>/_lyx`→`<weft>/_lyx` and `_pattern` examples as the default-pathspec instances and the existing "no `_raddle` junction is wired in this release" note. In `manifest/designs/fabric-unified-view.md` (the campaign design that names this task as its build-order slice 1): mark slice 1 "Config-driven junction list" complete in the "Build order" section (`:104`, annotate item 1 as done / landed, since the design doc's Build-order is this campaign's slice tracker per the discussion's Constraints note) and resolve the "Home of the junction-name config" open question (`:116`) to the decided answer — `fabricengine` reads `fabric.yaml` `pathspec` and injects the name-set into `hubgeometry`, which stays the owner of path construction. IMPORTANT — when annotating item 1 done, correct its overclaim: the slice-1 text (`:104`) says this slice migrates `_lyx`/`_raddle`/`_board`'s hardcoded junction wiring into config, but the delivered scope migrates ONLY the weft-backed junction name-set (`_lyx`/`_pattern`, via `pathspec`); `_board`/`_portals`/`_launchers`/`_raddle` stay hardcoded and hubgeometry-owned via `HubReservedNames()` (they are hub-structural, not per-worktree weft junctions — `_raddle` is reserved-only, wired later when it graduates to `pathspec`). Note this scope split in the annotation so the design doc does not record an inaccuracy. Do NOT touch `manifest/roadmap.md` (the whole `fabric-unified-view` item stays Planned until the campaign completes, per the discussion). Preserve the repo's one-line-per-paragraph markdown style (no hard-wrap) in all three files.
- **Commit:** `docs(fabric): describe config-driven junction set and mark design slice 1 done`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/` runs the new `//go:build integration` proof file (card 12), the untagged helper unit tests (card 13), the extended `add_test.go` (card 14), and the whole existing fabricengine suite — confirming the documentation edits (card 15, `doc.go` comments + markdown) did not break compilation. `-tags integration` is required for the git-spawning proof tests in card 12; card 13's helper tests are hermetic (Tier-1) and run under the same command as a superset. No module-wide `verify:` is set; the configured `pipeline.done_gate` (`go test ./...`) gates the fully-assembled tree when mill-go marks the task done.
