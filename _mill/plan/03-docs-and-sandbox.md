# Batch: docs-and-sandbox

```yaml
task: "Reconsider whether lyx mux needs anchor:top at all"
batch: docs-and-sandbox
number: 3
cards: 3
verify: go test ./cmd/lyx/ -run TestSandboxCoverage_AllModulesCoveredOrExcluded
depends-on: [2]
```

## Batch Scope

Docs and the operator-run sandbox suite are brought in line with the removed `anchor:top`:
`loom.md`'s planned status strand is migrated to `below-parent + ShrinkWhenWaitingOnChild`, the
mux review-prompt's TOP-BAND items are dropped, and the mux sandbox suite retires its top-band
scenario (M6), rewrites the mixed-adds scenario (M12) to below-parent only, and gains the new
operator-run M18 (below-parent root mother + child) that stands in for the proposal's empirical
test. This batch changes no compiled behavior; its one executable guard is the sandbox coverage
test, which requires M2's `**Covers:** mux` tag to survive the edits.

## Cards

### Card 13: migrate loom.md status strand to below-parent

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/height.go`
- **Edits:**
  - `docs/modules/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the `lyx loom status` strand's documented display from `anchor:top`
  to `below-parent` + `ShrinkWhenWaitingOnChild` in both places: the module table row (line 237,
  `runs as a strand (see internal/muxengine; anchor:top)`) and the `lyx loom run` bootstrap
  pseudo-code (around line 259, `display: anchor:top, height:fixed(1)`). Replace the stale
  `height:fixed(1)` with the dynamic shrink behavior — full height while it has no live child,
  collapsing to `collapsed_strip_rows` once a forked child exists. Add a one-clause note that a
  childless status strand rendering full-height is intended (discussion Decision
  `childless-full-height-is-acceptable`), so a future reader does not re-file the redundancy
  question. Do not otherwise restructure loom.md.
- **Commit:** `docs(loom): status strand uses below-parent shrink, not anchor:top`

### Card 14: drop TOP-BAND items from the mux review prompt

- **Context:** none
- **Edits:**
  - `docs/reviews/mux-review-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the two `TOP-BAND LEGIBILITY` bullets (the config-default check at
  lines 109-111 and the `add --anchor top --top-band-rows N` inspection at lines 243-245).
  In the anchor-vocabulary line (256), change `--anchor top|below-parent|hidden` to `--anchor
  below-parent|hidden` (keep the `own-window`/unknown-parent/non-leaf rejection paths). In the
  psmux-normalization bullet (279), drop `top_band_rows` from the `collapsed_strip_rows` /
  `top_band_rows` pair. Leave every non-top-band scenario untouched.
- **Commit:** `docs(reviews): remove anchor:top items from mux review prompt`

### Card 15: retire M6, rewrite M12, add M18 in the mux sandbox suite

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/render/focus.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-MUX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** (a) **Retire M6** ("Layout sanity (>=2 top, 0 stack)") — the top-band tiling
  scenario is meaningless with `anchor:top` gone. Replace its body with a one-line tombstone
  ("Retired: top-band tiling removed with anchor:top") and keep the `### M6` heading and number
  so M7-M17 refs and the session-log list stay stable (do not renumber). (b) **Rewrite M12**
  ("Layout survival under mixed adds") — drop the two top-anchored strands; make it a
  below-parent-only busy session (e.g. a parent strand, a child under it, and a second child)
  and assert every strand keeps a live pane, that `list-panes` shows the expected pane count
  with the parent shrunk to a strip once a child exists, and the deepest child dominant.
  (c) **Add M18** ("Below-parent mother/child shrink — operator-assisted visual") after M17: a
  below-parent *root* mother running a simple status-line placeholder (a plain, non-TUI
  command) with a Claude Code child added under it via `--parent`, no `--anchor top` anywhere;
  assert the mother is full height while alone, collapses to `collapsed_strip_rows` once the
  child exists, the child gets the window bulk, and the plain-text status line is legible at the
  collapsed height (contrast the box-drawing-TUI corruption that motivated the removed
  `TopBandRows`). Add the `M18` row to the session-log format list at the file end. Do **not**
  touch M2 or its `**Covers:** mux` tag — the sandbox coverage guard depends on it.
- **Commit:** `docs(sandbox): retire M6, rewrite M12, add M18 for below-parent mother/child`

## Batch Tests

`verify: go test ./cmd/lyx/ -run TestSandboxCoverage_AllModulesCoveredOrExcluded`

- The sandbox coverage guard globs `tools/sandbox/*SUITE.md` and asserts every registered module
  keeps a `**Covers:** <module>` tag. It is the one executable check for this batch: it confirms
  card 15's edits to `SANDBOX-MUX-SUITE.md` preserve M2's `**Covers:** mux` tag (retiring M6,
  rewriting M12, and adding M18 must not disturb it).
- `loom.md` (card 13) and `mux-review-prompt.md` (card 14) have no executable guard — they are
  prose whose accuracy the plan reviewer validates against the removed-anchor:top end state.
