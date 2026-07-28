# Batch: pattern-junction-flip

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
batch: pattern-junction-flip
number: 5
cards: 6
verify: go test -tags integration ./internal/hubgeometry/... ./internal/fabricengine/... ./internal/initengine/... ./internal/initcli/... ./internal/loomengine/... ./cmd/lyx/...
depends-on: [4]
```

## Batch Scope

This is the flip. `HostJunctions` and `HostJunctionsHere` gain their `_pattern` entry, and from card 15 onward loomyard has two junctions instead of one. Everything that made this safe landed in batches 3 and 4: the seeder materialises weft targets, unwire and remove iterate, all three health checks are per-junction with junction-named reasons, loom's preflight matches on a substring, and `CommitWeft` tolerates a pathspec entry matching nothing. What remains here is the data change itself, `initengine`'s reporting, the host-pollution scan, the existing tests that pin a one-junction world, and the docs.

Two consequences are accepted deliberately and documented rather than suppressed. First, **`lyx init` reporting**: batch 3's materialisation means `Init`'s post-`WireJunctions` stat of `cwd/_lyx` now succeeds through the freshly-created junction, so `LyxDir` would report `"exists"` on a first-ever init — silently inverting an existing CLI observable and making the new `PatternDir: "created"` assertion unpassable. Card 16 fixes that by stating the weft-side targets *before* `WireJunctions` runs. Second, **every worktree wired before this change lacks the `_pattern` junction**, so the generalised health check correctly reports a fault until `lyx init` or `lyx fabric reconcile` is re-run — including this repo's own live worktrees, including whichever one lands this change. That is the correct behaviour, not a regression: the junction genuinely is missing, and a health check that lies about a missing junction is the exact fault batch 3 exists to remove.

## Cards

### Card 15: add `_pattern` to both junction accessors

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/weftwiring.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
  - `internal/hubgeometry/weft_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/hubgeometry/hubgeometry.go`, add a second `HostJunction` record to both `HostJunctions(slug)` and `HostJunctionsHere()`: `{Name: PatternDirName, Link: HostPatternLink(slug), Target: WeftPatternDirFor(slug)}` for the slug-anchored form, and the `HostPatternLinkHere()`/`WeftPatternDir()` pair for the `Here` form. Keep `_lyx` first in both slices — `UnwireResult.JunctionsRemoved` is documented as being in `HostJunctions` order, and the health check is first-unhealthy-wins, so the order is observable. Update both functions' godoc, which currently states that the slice has exactly one entry. Then fix the tests that pin the one-junction world, updating rather than deleting them: `internal/hubgeometry/hubgeometry_test.go`'s `HostJunctions` subtest asserts `len(junctions) != 1` and indexes `junctions[0]`; `internal/hubgeometry/weft_test.go`'s `TestHostJunctions` table carries a `wantJunctionCount: 1` field and a single `wantName`, and its doc comment states the function returns exactly one entry and that no entry's `Name` equals `_raddle` — that last assertion stays true and stays. Both should now assert two entries with the expected `Name`/`Link`/`Target` for each, at `RelPath == "."` and at a nested `RelPath`.
- **Commit:** `hubgeometry: add the _pattern host junction`

### Card 16: report `_pattern` from `lyx init` and fix the created/exists inversion

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/initengine/init.go`
  - `internal/initengine/init_test.go`
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `PatternDir string` to `initengine.InitResult` alongside `LyxDir string`, carrying the same `"created"`/`"exists"` vocabulary, and have `Init` create the `_pattern` directory through the junction exactly as it already creates `_lyx`. **The ordering fix is the load-bearing part of this card.** `Init` today calls `fabricengine.WireJunctions` and only *then* stats `cwd/_lyx` to decide `"created"` versus `"exists"`. Since batch 3, the seeder materialises each junction's weft-side target, so that stat now succeeds through the fresh junction and `LyxDir` reports `"exists"` on a first-ever init — silently inverting the existing `lyx_dir` CLI observable and making a `PatternDir: "created"` assertion impossible to satisfy. Restructure `Init` to stat both weft-side targets (`l.WeftLyxDirFor(slug)` and `l.WeftPatternDirFor(slug)`) **before** the `WireJunctions` call, and derive both `LyxDir` and `PatternDir` from that pre-wiring observation. The vocabulary then keeps meaning the only thing an operator can use it for — did this invocation create it? Keep the existing post-wiring `MkdirAll` of `cwd/_lyx` and add the matching one for the `_pattern` directory: both are now redundant with the seeder's materialisation but are harmless and keep `Init` self-contained. Preserve the existing hard error when the `_lyx` path exists but is not a directory, and add the equivalent for `_pattern`. In `internal/initcli/initcli.go`, add a `pattern_dir` key to `runInit`'s emitted map beside `lyx_dir`, and update `lyx init`'s `Short`/`Long` to say it now wires two junctions and creates both directories — help accuracy is a review-blocking obligation under the CLI/Cobra Invariant. Extend `internal/initengine/init_test.go` so `PatternDir` reports `"created"` on a first run and `"exists"` on a second, **and assert `LyxDir` does the same** — that second assertion is the regression guard for this card's whole reason for existing: if the stat is ever moved back after `WireJunctions`, both report `"exists"` on a first init and only an explicit `LyxDir` assertion catches it. Also assert that after `Init` both junctions resolve, both weft directories exist, and a second `Init` is idempotent. Extend `internal/initcli/initcli_test.go` to pin the new `pattern_dir` key.
- **Commit:** `initengine: create and report the _pattern directory from lyx init`

### Card 17: leave weft `_pattern` content untouched on `lyx init --undo`

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/initengine/testmain_test.go`
- **Edits:**
  - `internal/initengine/undo.go`
  - `internal/initengine/undo_test.go`
  - `internal/initcli/initcli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Undo` removes both junctions and both git-exclude entries — that already happens via batch 3's generalised `UnwireJunctions` — but must **not** touch weft `_pattern/` content. Only `_lyx` content is cleared: the `os.RemoveAll` target stays `l.WeftLyxDirFor(slug)`, the commit pathspec stays `ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})`, and the commit message stays `_lyx`-scoped. So this card adds no new deletion logic; its work is to make the omission **deliberate and pinned** rather than incidental. State the rule in `Undo`'s godoc and in the `UndoResult.WeftContent` field comment: `WeftContent` continues to describe `_lyx` only, and `_pattern` content is preserved by design. The reasoning belongs in the godoc because the code will otherwise read as an oversight: `Undo` does `RemoveAll`, commits, and **pushes** the deletion, which is right for `_lyx` — lyx's own runtime state — and badly wrong for `_pattern`, which is the host repo's hand-authored invariants. Deactivating lyx must not destroy the repo's own constraints and push that deletion to the remote, where it cannot be casually undone. Also update `lyx init --undo`'s `Long` in `internal/initcli/initcli.go` if it describes weft clearing, so the help says both junctions are removed while `_pattern` content is preserved. Add the destructive-behaviour guard to `internal/initengine/undo_test.go` — it must exist, not be optional: seed a `PATTERN.md` under the weft `_pattern` directory, run `Undo`, and assert the file survives on disk **and** that no deletion of it was committed. Also assert both junctions and both exclude entries are gone and that `UndoResult.JunctionsRemoved` names both.
- **Commit:** `initengine: preserve weft _pattern content across lyx init --undo`

### Card 18: treat tracked `_pattern` paths as restorable host pollution

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/enforcement_test.go`
- **Edits:**
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `detectHostPollution` in `internal/fabricengine/status.go` scans the host index with `git ls-files -- _lyx _raddle` and classifies matches two ways: `_lyx` matches offer an automated `git rm --cached` restore remedy plus a reminder to restore the junction and exclude entry, while `_raddle` matches are report-only *because* no junction is wired for it. Add `_pattern` to the pathspec and classify its matches like `_lyx`, not like `_raddle` — from card 15 it has a junction, so the same automated restore applies. Update the function's doc comment, which today explains the two-way split in terms of `_lyx` and `_raddle` only. Note for the implementer: both new uses of the literal here are legal under the Hub Geometry Invariant despite `_pattern` being an enforced token, because the invariant explicitly carves out comparisons and git-pathspec slice literals as not path construction — the guard flags a matching literal only in a `filepath.Join` argument, a `+` operand, or a string const value. Do not route these through a `hubgeometry` accessor; a pathspec entry and a prefix comparison are exactly the shapes the carve-out exists for. Add a case to `internal/fabricengine/junction_pattern_integration_test.go` asserting a tracked path under `_pattern` in the host index is reported as pollution with the same restore remedy `_lyx` gets, and is not report-only.
- **Commit:** `fabricengine: report tracked _pattern paths as restorable host pollution`

### Card 19: cover the two-junction world end to end

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/remove.go`
  - `internal/initengine/init.go`
  - `internal/loomengine/preflight.go`
- **Edits:**
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** With two junctions live, extend the integration suites so every generalisation from batch 3 is exercised against a genuinely second junction rather than against a loop of length one. Health check, **one case per site** — all three must be covered separately, because `drift.go` does not share `checkJunctionHealth`'s code path: with `_lyx` healthy and `_pattern` missing or mis-pointed, `reconcile` must repair rather than report `ReconcileActionAlreadyHealthy`; `status` must report `JunctionHealthy` false with a `JunctionReason` naming `_pattern` and the pair not in sync; and `drift` must report drifted with the reason matching `checkJunctionHealth`'s wording for that fault. Per-junction refusal behaviours still hold: a real, non-link directory at *either* host path is refused, and a dangling or wrong-target link at either is re-pointed. Wiring is idempotent: wiring twice is a no-op, and wiring a worktree with `_lyx` already wired and `_pattern` not yet — the upgrade path — completes without error and adds only the missing junction. `loomengine` preflight classification must hold for all three `_pattern` drift shapes: missing, non-link and mis-pointed each classify as `CheckJunction` and set `check3BlocksSeed`, never `CheckWeftSync`. The **legacy-worktree upgrade** case, which is the one operators will actually meet: a worktree with `_lyx` wired and no `_pattern` junction reports not-in-sync from `status`, is repaired by `reconcile`, and blocks loom's preflight — then passes after one `lyx init`. And `remove` at a nested `RelPath` with both junctions wired leaves neither behind. Sweep `internal/fabricengine/reconcile_stale_registration_test.go` and `internal/fabricengine/junction_repoint_test.go` for any remaining assumption that a worktree has exactly one junction and update it — never delete it.
- **Commit:** `fabricengine: cover the two-junction world in the integration suites`

### Card 20: document the second junction and the upgrade step

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/status.go`
  - `internal/initengine/init.go`
- **Edits:**
  - `docs/overview.md`
  - `docs/shared-libs/hubgeometry.md`
  - `internal/fabricengine/doc.go`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Four documentation obligations, all due in this batch because this is the commit that makes them true. (0) `docs/shared-libs/hubgeometry.md`'s "Junction detection methods" section still describes `HostJunctions`/`HostJunctionsHere` as returning a single `_lyx` entry; update both bullets to state two entries, `_lyx` then `_pattern`, matching the godoc `HostJunctions`/`HostJunctionsHere` carry as of this batch. (1) `docs/overview.md`'s "Junction model" section lists the junctions; add the `<host>/_pattern` → `<hub>/<slug>-weft/_pattern` entry, **and fix the pre-existing error in the same passage while in it**: the list also claims a `<host>/_raddle` → `<hub>/<slug>-weft/_raddle` junction, which does not exist — `internal/fabricengine/status.go` states plainly that no junction is wired for `_raddle` in this release, and `HostJunctions` has never returned one. Leaving that line while adding `_pattern` beside it would produce a list of three junctions where two exist. (2) In `internal/fabricengine/doc.go`, document the legacy-worktree upgrade consequence with its concrete blast radius, so nobody meets it as a surprise: every worktree wired before this change lacks the `_pattern` junction, so `lyx fabric status` reports the pair not in sync with the reason naming `_pattern`, `lyx fabric reconcile` reports `ReconcileActionJunctionRepointed` rather than `AlreadyHealthy` — and repairs it, so reconcile *is* the remedy — and loom's preflight fails `CheckJunction`, sets `check3BlocksSeed` and blocks the run. This repo's own live worktrees are affected, including whichever one lands this change. State the remedy — one `lyx init` (idempotent; wires the missing junction and materialises the weft directory) or one `lyx fabric reconcile` — and state why it is not suppressed: the junction genuinely is missing, and a health check that lies about a missing junction is the exact fault the generalisation exists to remove. Note the deliberate asymmetry with the pathspec migration documented in batch 4 — the junction side self-heals on the next `init`/`reconcile` and reports loudly until it does, while the pathspec side never self-heals and reports nothing — and why: `WireJunctions` owns junction state outright, whereas `pathspec` is an operator-editable config value `configsync` must not overwrite. (3) Re-read and update the affected cobra `Short`/`Long` strings in `internal/fabriccli/fabric.go` for `lyx fabric reconcile`, `lyx fabric status`, `lyx fabric remove` and `lyx fabric checkout`, whose observable behaviour all changed — reconcile and status now detect and repair a second junction, remove tears one down, and checkout is the third `WireJunctions` caller so it now wires and materialises both. `fabric checkout`'s existing help already speaks generically of "junctions" in the plural, so it is likely a re-read-and-confirm rather than a rewording, but confirming it is still the obligation. Write every markdown paragraph and list item as one unwrapped line.
- **Commit:** `docs: document the _pattern junction and the worktree upgrade step`

## Batch Tests

`verify: go test -tags integration ./internal/hubgeometry/... ./internal/fabricengine/... ./internal/initengine/... ./internal/initcli/... ./internal/loomengine/... ./cmd/lyx/...` runs the widest scope in this plan short of batch 7's repo-wide gate, and the breadth is justified rather than defensive: card 15 changes the return value of a `hubgeometry` accessor that `fabricengine`, `initengine` and `loomengine` all consume at runtime, so a one-junction assumption anywhere in those four packages surfaces here or not at all. `-tags integration` is required — the three integration-tagged files carrying this batch's central assertions (`junction_pattern_integration_test.go`, `remove_junctions_integration_test.go`, `preflight_integration_test.go`) do not compile without it. Existing tests updated rather than deleted: `internal/hubgeometry/hubgeometry_test.go`, `internal/hubgeometry/weft_test.go`, `internal/initengine/init_test.go`, `internal/initengine/undo_test.go`, `internal/initcli/initcli_test.go`, `internal/fabricengine/reconcile_stale_registration_test.go`, `internal/fabricengine/junction_repoint_test.go`, `internal/loomengine/preflight_integration_test.go`. `./cmd/lyx/...` also carries the CLI/Cobra guards (`drift_test.go`, `helptree_test.go`, `longlist_test.go`) that card 20's help-text edits must keep passing. No new test file is created in this batch, so no new build-tag or hermetic-env decision arises.
