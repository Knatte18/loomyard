# Batch: hubgeometry-pattern-surface

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
batch: hubgeometry-pattern-surface
number: 2
cards: 3
verify: go test -tags integration ./internal/hubgeometry/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch gives `internal/hubgeometry` its complete `_pattern` surface — the constant, the six path accessors, the reserved-name entry, the new `HostJunctionsHere()` detection accessor, and the enforcement-test token — **without yet adding `_pattern` to `HostJunctions`**. That split is the whole point of making this its own batch: the moment `HostJunctions` returns a second entry, `fabricengine`'s seeder starts creating a junction whose weft target nothing materialises, and every existing `fabricengine`/`initengine` integration test breaks. So this batch is purely additive — every new symbol compiles and is tested, nothing that already exists changes behaviour — and the flip waits until batch 5, after batch 3 has taught `fabricengine` to materialise, unwire, remove and health-check junctions generically.

The external interfaces later batches consume: `PatternFileHere()` (batch 6's active check), `HostJunctionsHere()` (batch 3's three health-check sites), and `HostPatternLink`/`WeftPatternDirFor` (batch 5's `HostJunctions` entry).

## Cards

### Card 3: add the `_pattern` geometry constant and its six path accessors

- **Context:**
  - `docs/shared-libs/hubgeometry.md`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/hubgeometry/pattern_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add exported const `PatternDirName = "_pattern"` to `internal/hubgeometry/hubgeometry.go`, then add six methods on `*Layout`, each mirroring its existing `_lyx` counterpart exactly — including the `RelPath` join, which is the trap this whole surface exists to avoid: `WeftPatternDir()` mirroring `WeftLyxDir()` (`filepath.Join(WeftWorktree(), RelPath, PatternDirName)`); `WeftPatternDirFor(slug string)` mirroring `WeftLyxDirFor(slug)` (`filepath.Join(WeftWorktreePath(slug), RelPath, PatternDirName)`); `HostPatternLink(slug string)` mirroring `HostLyxLink(slug)` (`filepath.Join(WorktreePath(slug), RelPath, PatternDirName)`); `HostPatternLinkHere()` mirroring `HostLyxLinkHere()` (`filepath.Join(WorktreeRoot, RelPath, PatternDirName)`); plus two accessors with no `_lyx` counterpart — `PatternDir(baseDir string) string` returning `filepath.Join(baseDir, PatternDirName)` and `PatternFile(baseDir string) string` returning `filepath.Join(baseDir, PatternDirName, "PATTERN.md")`, both free functions taking an explicit base in the shape of the existing `ConfigDir(base)`/`ConfigFile(base, module)` helpers — and finally `PatternFileHere() string` on `*Layout`, returning `PatternFile(filepath.Join(l.WorktreeRoot, l.RelPath))`. `PatternFileHere` is the one batch 6 calls, and its `WorktreeRoot`+`RelPath` anchor is load-bearing: a `WorktreeRoot`-only anchor would miss the file entirely in any nested-hub geometry and render PATTERN silently inactive in all five agents. Godoc each accessor stating the exact join it returns, matching the style of the surrounding `_lyx` accessors, and state on `PatternFileHere` why it is anchored at `WorktreeRoot+RelPath` rather than `WorktreeRoot` or `Cwd`. Create `internal/hubgeometry/pattern_test.go` (untagged; it spawns nothing and copies nothing) asserting each accessor's join for both `RelPath == "."` and a nested `RelPath` of at least two segments, and asserting that `PatternFileHere()` on a `Resolve`-built Layout equals `PatternFile(l.Cwd)` — the equality that holds because `Resolve` sets `RelPath = filepath.Rel(WorktreeRoot, Cwd)`.
- **Commit:** `hubgeometry: add _pattern geometry constant and path accessors`

### Card 4: add `HostJunctionsHere()` as the anchor-correct detection accessor

- **Context:**
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/status.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `HostJunctionsHere() []HostJunction` on `*Layout`, returning the same `HostJunction` records as `HostJunctions(slug)` but resolved against the *current* worktree rather than a named slug: each entry's `Link` comes from the `…Here()` accessor (`HostLyxLinkHere()`) and each `Target` from the un-slugged weft accessor (`WeftLyxDir()`). In this batch it returns exactly one entry, for `_lyx`, matching `HostJunctions`'s current single entry; batch 5 adds the `_pattern` entry to both in one card. Godoc must state why the pair exists: `HostJunctions(slug)` is `Hub`/slug-anchored and is what wiring, unwiring and `remove` use, whereas all three junction **health-check** sites (`fabricengine/reconcile.go`, `fabricengine/status.go`, `fabricengine/drift.go`) use the `Here`-anchored `HostLyxLinkHere()`/`WeftLyxDir()` pair and have no slug available — `PairInSync(l *hubgeometry.Layout)` in particular takes no slug at all and is documented as stateless, so threading one in would break its contract. Point the godoc at the existing `HostLyxLinkHere()`/`HostLyxLink(slug)` and `WeftLyxDir()`/`WeftLyxDirFor(slug)` pairs as the precedent this mirrors. Extend `internal/hubgeometry/hubgeometry_test.go` with a case asserting `HostJunctionsHere()` returns the expected `Name`/`Link`/`Target` for both `RelPath == "."` and a nested `RelPath`, and a case asserting it agrees entry-for-entry with `HostJunctions(slug)` when the layout's slug and current worktree coincide.
- **Commit:** `hubgeometry: add HostJunctionsHere for slug-free junction detection`

### Card 5: reserve `_pattern` and add it to the machine-enforced token list

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/hubgeometry/enforcement_test.go`
  - `internal/hubgeometry/hubgeometry_test.go`
  - `CONSTRAINTS.md`
  - `docs/shared-libs/hubgeometry.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three coupled changes plus their docs. (1) In `internal/hubgeometry/hubgeometry.go`, add `PatternDirName` to `IsReservedHubName`'s switch, so a worktree slug can never claim `_pattern` — the same collision `_lyx`, `_raddle`, `_board`, `_portals` and `_launchers` are already protected against. (2) In `internal/hubgeometry/enforcement_test.go`, add `"_pattern"` to the `geometryToken` switch inside `TestEnforcement_GeometryLiterals`. From this commit onward the literal is banned outside `internal/hubgeometry` in a `filepath.Join` argument, a `+` operand, or a string const value — which is precisely why card 3's accessors must exist before this lands, and why `internal/pattern` in batch 6 can never construct its own path. Note the guard's shape while making this change: it is whole-token equality on production files only, and it does **not** flag a literal in a comparison or in a git-pathspec slice literal, which is what will make `fabricengine/status.go`'s new pollution-scan entry legal in batch 5. (3) In `internal/hubgeometry/hubgeometry_test.go`, assert `IsReservedHubName("_pattern")` is true. Then update `CONSTRAINTS.md`'s Hub Geometry Invariant bullet to list `_pattern` among the owned geometry tokens, and update `docs/shared-libs/hubgeometry.md` to document `PatternDirName`, the six accessors from card 3, and `HostJunctionsHere()` from card 4. Both markdown edits use one unwrapped line per paragraph or list item.
- **Commit:** `hubgeometry: reserve _pattern and enforce it as a geometry token`

## Batch Tests

`verify: go test -tags integration ./internal/hubgeometry/... ./cmd/lyx/...` covers the two test files this batch edits (`internal/hubgeometry/hubgeometry_test.go`, `internal/hubgeometry/enforcement_test.go`) and the one it creates (`internal/hubgeometry/pattern_test.go`).

**`-tags integration` is mandatory here and is easy to get wrong**, because nothing this batch *creates* is integration-tagged. The reason is what it *edits*: `internal/hubgeometry/hubgeometry_test.go` already begins with `//go:build integration`, so under a plain `go test` that file is excluded from the build entirely and card 4's new `HostJunctionsHere()` assertions and card 5's new `IsReservedHubName("_pattern")` assertion would never compile, let alone run. The failure would be invisible — a green verify over code that was never built — and would not surface until batch 5's wider tagged run, three batches later, entangled with the two-junction flip's much larger diff. `cmd/lyx`'s guards do not compensate: `tierpurity_test.go` and `hermeticenv_test.go` text-scan file bytes and never compile them. Go's build tags are additive, so the tagged run still exercises the untagged tier in these packages.

The new assertions stay in `hubgeometry_test.go` beside their existing `HostLyxLinkHere`/`HostJunctions` siblings rather than being relocated into the untagged file, because batch 5's card 15 must edit that same file anyway and splitting one accessor's coverage across two files by build tag would be worse than the tag itself.

`./cmd/lyx/...` is in scope deliberately rather than by habit: card 5 changes what `TestEnforcement_GeometryLiterals` bans repo-wide, and `cmd/lyx` holds the three cross-cutting guards this plan can trip — `tierpurity_test.go`, `hermeticenv_test.go` and `sandbox_coverage_test.go` — so a new untagged test file with a banned token would otherwise not surface until a later batch. No file this batch creates spawns a process or copies a fixture tree; `internal/hubgeometry/pattern_test.go` is pure join arithmetic and stays untagged.
