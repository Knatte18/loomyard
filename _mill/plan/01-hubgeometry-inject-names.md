# Batch: hubgeometry-inject-names

```yaml
task: 'fabric: config-driven junction list'
batch: hubgeometry-inject-names
number: 1
cards: 4
verify: go test -tags integration ./internal/hubgeometry/
depends-on: []
```

## Batch Scope

This batch makes `internal/hubgeometry` config-blind about the junction name-set. It adds the single exported `HubReservedNames()` accessor, changes `IsReservedHubName`, `HostJunctions`, and `HostJunctionsHere` to take the weft-backed name-set as an explicit `[]string` parameter, removes the hardcoded `_lyx`/`_pattern` junction-record literals, updates the affected godoc, and amends the `CONSTRAINTS.md` Hub Geometry Invariant. It is one batch because it is a single pure package with no config dependency, TDD-friendly, and self-contained (compiles and tests in isolation).

The external interface batch 2 consumes: `HubReservedNames() []string`, `IsReservedHubName(name string, junctionNames []string) bool`, `HostJunctions(slug string, names []string) []HostJunction`, `HostJunctionsHere(names []string) []HostJunction`.

Batch-local note: changing these three method signatures breaks `internal/fabricengine` and `internal/initengine` compilation until batch 2 — expected and covered by the Shared Decision "Go verify commands, no repo-wide compile between signature batches". This batch's `verify:` compiles only `./internal/hubgeometry/`, which stands alone.

## Cards

### Card 1: Add `HubReservedNames()` and re-signature `IsReservedHubName`

- **Context:**
  - `internal/fabricengine/add.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add exported `func HubReservedNames() []string` returning `[]string{BoardDirName, "_portals", "_launchers", "_raddle"}` — today's `IsReservedHubName` set minus the config-migrated `LyxDirName`/`PatternDirName`. Its godoc must state it is the sole source of the hub-structural reserved set (both `IsReservedHubName` and `fabricengine`'s wiring-guard filter consume it) and that `_raddle` is a known-future junction name reserved ahead of wiring. Change `IsReservedHubName(name string) bool` (currently `internal/hubgeometry/hubgeometry.go:424`, a `switch` over `LyxDirName, "_raddle", BoardDirName, "_portals", "_launchers", PatternDirName`) to `IsReservedHubName(name string, junctionNames []string) bool`: return true if `name` is in `HubReservedNames()` OR in `junctionNames`. Update its godoc: the hub-structural set is hubgeometry-owned; the junction/weft-backed portion is injected from fabric config (`pathspec`), and the union over the default `junctionNames = ["_lyx","_pattern"]` reproduces exactly today's six reserved names. The four literal token strings remain only inside `hubgeometry`.
- **Commit:** `refactor(hubgeometry): add HubReservedNames, inject junctionNames into IsReservedHubName`

### Card 2: Inject `names` into `HostJunctions`/`HostJunctionsHere`, remove literals

- **Context:** none
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `HostJunctions(slug string) []HostJunction` (`internal/hubgeometry/hubgeometry.go:802`) to `HostJunctions(slug string, names []string) []HostJunction` and `HostJunctionsHere() []HostJunction` (`:834`) to `HostJunctionsHere(names []string) []HostJunction`. Replace the two hardcoded `{_lyx, _pattern}` record literals in each with a generic loop over `names` producing one `HostJunction{Name: n, Link: ..., Target: ...}` per name, in the input slice's order: for `HostJunctions`, `Link: filepath.Join(l.WorktreePath(slug), l.RelPath, n)` and `Target: filepath.Join(l.WeftWorktreePath(slug), l.RelPath, n)`; for `HostJunctionsHere`, `Link: filepath.Join(l.WorktreeRoot, l.RelPath, n)` and `Target: filepath.Join(l.WeftWorktree(), l.RelPath, n)`. These reproduce today's per-name accessors (`HostLyxLink`/`WeftLyxDirFor`/`HostPatternLink`/`WeftPatternDirFor` and their Here variants) exactly. An empty `names` slice yields no records. Update the godoc on both methods: order now follows the injected `names` slice (the default `pathspec: _lyx _pattern` keeps `_lyx` first as a property of the default, not an enforced invariant); drop the "_lyx stays first deliberately" wording. This includes the sentence in `HostJunctions`'s own godoc (~`hubgeometry.go:794-797`) that cites `UnwireResult.JunctionsRemoved` as the reason `_lyx` is first — reword it to "order follows the injected name list". Do NOT edit `UnwireResult.JunctionsRemoved`'s own field comment: that type lives in `internal/fabricengine/junction.go`, not in this card's `Edits:` target, and batch 1 has no dependency on `fabricengine` (its ordering godoc is covered by batch 2). Leave the per-name accessors (`HostLyxLink`, `WeftLyxDirFor`, `HostPatternLink`, `WeftPatternDirFor`, and Here variants) in place; other code may use them.
- **Commit:** `refactor(hubgeometry): inject junction name-set into HostJunctions/HostJunctionsHere`

### Card 3: Update hubgeometry unit tests to the new signatures + regression assertions

- **Context:** none
- **Edits:**
  - `internal/hubgeometry/weft_test.go`
  - `internal/hubgeometry/geometry_test.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update every direct call to the re-signatured methods: `layout.HostJunctions(slug)` → `layout.HostJunctions(slug, []string{"_lyx", "_pattern"})` at `weft_test.go:263` and `:310`; `layout.HostJunctions(slug)` at `hubgeometry_test.go:594` and `:727`; `layout.HostJunctionsHere()` → `layout.HostJunctionsHere([]string{"_lyx", "_pattern"})` at `hubgeometry_test.go:643`, `:688`, `:726`; `hubgeometry.IsReservedHubName(tt.input)` → add a `junctionNames` argument at `geometry_test.go:232` (pass `[]string{"_lyx", "_pattern"}` and keep the existing expectations, which already treat `_lyx`/`_pattern` as reserved) and `hubgeometry.IsReservedHubName("_pattern")` at `hubgeometry_test.go:747` → `hubgeometry.IsReservedHubName("_pattern", []string{"_lyx", "_pattern"})`. In `TestHostJunctions` (`weft_test.go:213`) and `TestHostJunctionsHere` (`hubgeometry_test.go:629`) add table rows asserting: an empty `names` slice yields zero records; a 3-name slice `["_lyx","_pattern","_extra"]` yields three records with correctly-composed Link/Target in input order; a reversed slice `["_pattern","_lyx"]` yields records in that given order (config order authoritative, no forced sort). In `TestIsReservedHubName` (`geometry_test.go:211`) add assertions: `_board`, `_portals`, `_launchers`, and `_raddle` are reserved for ANY `junctionNames` including the empty slice (assert `_raddle` stays reserved when `junctionNames` is empty — the r1-review regression); names present only in `junctionNames` are reserved; unrelated names are not; and the union over `junctionNames = ["_lyx","_pattern"]` reserves exactly the six names `_lyx, _pattern, _board, _portals, _launchers, _raddle`.
- **Commit:** `test(hubgeometry): cover injected junction name-set and reserved-name union`

### Card 4: Amend the Hub Geometry Invariant in CONSTRAINTS.md and the hubgeometry reference doc

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/shared-libs/hubgeometry.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `## Hub Geometry Invariant` section of `CONSTRAINTS.md`, add prose recording that the weft-backed junction *name-set* is injected from fabric config (`pathspec`) rather than enumerated in `hubgeometry`: `hubgeometry` still owns all path *construction* and the hub-structural tokens (`_board`, `_portals`, `_launchers`, `_raddle`), exposes the hub-structural reserved set via `HubReservedNames()`, and its junction methods (`HostJunctions`/`HostJunctionsHere`/`IsReservedHubName`) take the name-set as an explicit `[]string` parameter; the `enforcement_test.go` machine check stays green because no new geometry-token literal is introduced anywhere. Note explicitly (for future readers) that a `[]string{"_lyx","_pattern"}` slice literal in non-hubgeometry Go is NOT one of the three contexts `TestEnforcement_GeometryLiterals` catches (`filepath.Join` arg, binary-`+` operand, `const`-value `BasicLit`), so the "no config-migrated names hardcoded in fabric" rule rests on review discipline, not the machine check. Also update the durable reference doc `docs/shared-libs/hubgeometry.md` (the "Junction detection methods" bullets at ~`:124-125`): change the documented signatures to `HostJunctions(slug string, names []string) []HostJunction` and `HostJunctionsHere(names []string) []HostJunction`, and reframe the "two entries, `_lyx` first ... then `_pattern`" prose as "one record per injected name, in the given order (the default `pathspec: _lyx _pattern` yields `_lyx` then `_pattern`)"; add a bullet for the new `HubReservedNames() []string` accessor and update the `IsReservedHubName` description if it appears there. Keep the one-line-per-paragraph markdown style (no hard-wrap) in both files.
- **Commit:** `docs(hubgeometry): record injected junction name-set in Invariant and reference doc`

## Batch Tests

`verify: go test -tags integration ./internal/hubgeometry/` runs the full hubgeometry package (both the untagged unit tests in `weft_test.go`/`geometry_test.go` and the `//go:build integration` tests in `hubgeometry_test.go`, which hold `TestHostJunctionsHere` and `TestIsReservedHubName_Pattern`). `-tags integration` is required so those tagged tests compile and run; it is a superset that also runs every untagged test. The package is pure (no git spawn from the junction/reserved methods themselves) and stands alone, so it compiles and passes independently of the still-unmodified `fabricengine`/`initengine` packages. Card 3 adds the empty-list, 3-name, reversed-order, and reserved-union regression assertions; `enforcement_test.go` (`TestEnforcement_GeometryLiterals`) is run by the same command and must stay green after the literal removal — no new assertion is needed for it.
