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

### Card 12: Extensibility, wiring-guard, and narrow-pathspec health integration tests

- **Context:**
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/drift.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create a `//go:build integration` test file (package `fabricengine_test`, mirroring `junction_pattern_integration_test.go`'s imports and its `lyxtest.CopyPairedLocal(t)` + `lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{"fabric": <content>})` fixture pattern). Add three tests: (1) `TestWireJunctions_WiresEveryPathspecName` — seed a fabric config equal to `fabricengine.ConfigTemplate()` but with `pathspec: _lyx _pattern _extra`; call `WireJunctions(l, slug)`; assert the host link, weft target, and `.git/info/exclude` entry (via the existing `readExcludeLines` helper) exist for all three names `_lyx`, `_pattern`, `_extra` — the proof that a future raddle/board append is wired with no code change; then `UnwireJunctions(l, slug)` removes all three (assert `UnwireResult.JunctionsRemoved` contains all three and each host link is gone). Compose the `_extra` host link / weft target via `l.HostJunctions(slug, []string{"_extra"})[0].Link`/`.Target` (or `filepath.Join(l.WorktreePath(slug), l.RelPath, "_extra")`). (2) `TestWireJunctions_SkipsHubReservedNameInPathspec` — seed `pathspec: _lyx _pattern _board`; call `WireJunctions(l, slug)`; assert only `_lyx` and `_pattern` are wired and NO `_board` host junction is created (the wiring-guard filter drops `hubgeometry.HubReservedNames()` members). (3) `TestPairInSync_NarrowPathspecIsHealthy` — seed `pathspec: _lyx` (only), wire, and assert `PairInSync(l)` / `checkJunctionHealth` report the pair healthy when only `_lyx` is wired (narrow-pathspec reality: you check exactly the listed junctions).
- **Commit:** `test(fabricengine): prove pathspec-driven wiring, wiring-guard, narrow pathspec`

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
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/add_test.go`, extend the reserved-slug coverage (near `TestAdd_RejectsReservedHubNameSlug`, `:124`) so it also proves that a slug equal to a current `pathspec` junction name is rejected: with a `Topology` whose `cfg.Pathspec` includes a name such as `_pattern` (or an added `_extra`), `Add` rejects that slug with the "reserved for lyx hub geometry" error via the new `IsReservedHubName(slug, t.cfg.Dirs())` union — not only the hub-structural names. Keep the test at the tier the existing `TestAdd_RejectsReservedHubNameSlug` uses (it rejects at step 0 before any git operation, so no new fixture is needed).
- **Commit:** `test(fabricengine): reject Add slug equal to a pathspec junction name`

### Card 15: Document the config-driven junction set (doc.go + overview.md)

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/config.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`, add prose stating the wired junction set is now sourced from `fabric.yaml`'s `pathspec` key (the same list that drives weft-sync staging), filtered against `hubgeometry.HubReservedNames()`, so a new weft-backed module is wired by appending its dir name to the `pathspec` template default with no `fabric`/`hubgeometry` code change; keep the existing narrow-pathspec asymmetry note accurate. In `docs/overview.md`: update the `Layout` method list at line 40 (`HostJunctions(slug)` → `HostJunctions(slug, names)`); reframe the "Junction model" section (lines ~95–101) so the wired set is described as the `pathspec`-driven list rather than a hardcoded `_lyx`/`_pattern` pair, retaining the concrete `<host>/_lyx`→`<weft>/_lyx` and `_pattern` examples as the default-pathspec instances and the existing "no `_raddle` junction is wired in this release" note. Preserve the repo's one-line-per-paragraph markdown style (no hard-wrap).
- **Commit:** `docs(fabric): describe config-driven junction set in doc.go and overview`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/` runs the new `//go:build integration` proof file (card 12), the untagged helper unit tests (card 13), the extended `add_test.go` (card 14), and the whole existing fabricengine suite — confirming the documentation edits (card 15, `doc.go` comments + markdown) did not break compilation. `-tags integration` is required for the git-spawning proof tests in card 12; card 13's helper tests are hermetic (Tier-1) and run under the same command as a superset. No module-wide `verify:` is set; the configured `pipeline.done_gate` (`go test ./...`) gates the fully-assembled tree when mill-go marks the task done.
