# Batch: render-fixed-height-pins

```yaml
task: "reed: attach's layout computation scales header pane height with terminal height"
batch: "render-fixed-height-pins"
number: 1
cards: 2
verify: go test ./internal/reedengine/render/...
depends-on: []
```

## Batch Scope

This batch adds the second pure entry point to `internal/reedengine/render`: `FixedHeightPins`, which reports the panes whose heights are absolute row budgets (the header band and every collapsed strip) at the heights `Rules` actually placed them at.
It is one batch because it is one pure leaf package with no I/O and no engine dependency, and because the extraction that makes `Rules` and `FixedHeightPins` share a single policy composition has to land together with the entry point it exists to serve.
`Rules`' signature is unchanged — `(layout, focus, err)` — and both existing call sites (`internal/reedengine/apply.go`'s `planLayout` and the package's own tests) are untouched by this batch.

The external interface batch 2 consumes is `render.Pin` and `render.FixedHeightPins(strands []Strand, box Box, p Params) []Pin`.

Batch-local decision, differing from nothing in the overview: `FixedHeightPins` takes no `paneOrder` argument.
A pin names its pane by tmux pane id, so emission order carries no geometry; `paneOrder` only resequences layout-string cells.

## Cards

### Card 1: Share Rules' policy composition and add FixedHeightPins

- **Context:**
  - `internal/reedengine/render/policy.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/render/focus.go`
  - `internal/reedengine/render/checksum.go`
  - `internal/reedengine/render/rules_test.go`
  - `internal/reedengine/render/height_test.go`
- **Edits:**
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/height.go`
  - `internal/reedengine/render/layout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedengine/render/layout.go`, add a third field to the unexported `placement` struct: `strip bool`, documented as reporting whether this cell's height came from the collapsed-strip budget (an absolute row budget) rather than from the equal-split of whatever rows were left.
  `buildStackBody` must not read it.

  In `internal/reedengine/render/height.go`, set that field inside `stackHeights` when it builds its `placements` slice, from the `isStrip[i]` value it has already computed: `placements[i] = placement{id: s.PaneID, height: heights[i], strip: isStrip[i]}`.
  Do not change `stackHeights`' signature, its parameters, or `clampToFit` in any way — `height_test.go` calls `stackHeights` directly at six sites and must keep compiling and passing unchanged.

  In `internal/reedengine/render/rules.go`, extract the policy half of `Rules` into a new unexported `planCells(strands []Strand, box Box, p Params) (cellPlan, error)` and a new unexported struct `cellPlan` carrying exactly: `hasHeader bool`, `soleHeader bool`, `headerHeight int`, `stackBox Box`, `ordered []Strand`, `placements []placement`.
  `planCells` performs, in this order and with no behavioural change to any of them, exactly what `Rules` performs today up to and including `stackHeights`: the `AnchorOwnWindow` rejection loop (returning the same `fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)` error), `breakCycles`, `removeDuplicatePaneCells(partitionByAnchor(fixed), p.Header.PaneID)`, `orderStack`, the `hasHeader := p.Header.PaneID != ""` test, the `hasHeader && len(ordered) == 0` sole-header branch (which sets `soleHeader: true` and returns before any height math), the `const headerDivider = 1` / `clampHeaderHeight(p.Header.HeightRows, box.H-headerDivider, p.MinFullRows)` header budget, the `stackBox` derivation, and `stackHeights(ordered, stackBox, p)`.
  Move the existing explanatory comment blocks with the code they explain rather than rewriting them.

  Rewrite `Rules` to call `planCells` and then perform only the mechanics it performs today: on `soleHeader`, build and `wrapLayout` the same single-cell string it builds today and return `("", nil)` for focus and error; otherwise `resequenceByPaneOrder(plan.placements, paneOrder)`, `buildStackBody(plan.stackBox, …)`, `bandHeader(box, p.Header.PaneID, plan.headerHeight, …)` when `plan.hasHeader`, `focusTarget(plan.ordered)`, and `wrapLayout`.
  `Rules`' exported signature, its returned values for every input, and its doc comment's claims must all be unchanged; `rules_test.go` must keep passing with no edit.

  Add an exported struct `Pin` with exported fields `PaneID string` and `Height int`, documented as one pane whose height is an absolute row budget rather than "whatever is left" — the header band and every collapsed strip — and stating that `Height` is the height `Rules` actually placed the cell at, after `clampHeaderHeight`/`clampToFit`, never the raw configured budget.

  Add the exported `FixedHeightPins(strands []Strand, box Box, p Params) []Pin`.
  It calls `planCells` with the same three arguments and returns `nil` on any error from it, on `soleHeader`, and whenever it would otherwise produce an empty slice.
  Otherwise it returns, in this order: the header pin `Pin{PaneID: p.Header.PaneID, Height: plan.headerHeight}` when `plan.hasHeader`, followed by one `Pin{PaneID: pl.id, Height: pl.height}` for every `pl` in `plan.placements` whose `strip` field is true, in `plan.placements` order.
  Document that it is pure and total like `Rules`, that it takes no `paneOrder` because a pin names its pane by id, that the sole-header branch yields no pin because there the header claims the whole box and has no absolute budget, and that a caller must treat a nil return as "nothing is pinned", never as "no opinion".
- **Commit:** `feat(reedengine/render): add FixedHeightPins reporting the header and strip row budgets`

### Card 2: Unit-test FixedHeightPins against Rules' own placed heights

- **Context:**
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/height.go`
  - `internal/reedengine/render/layout.go`
  - `internal/reedengine/render/policy.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/render/rules_test.go`
  - `internal/reedengine/render/height_test.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/render/pins_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create an untagged test file with a file-level comment stating that it pins `FixedHeightPins` against the heights `Rules` places for the same inputs, so the two can never drift.
  Reuse the fixture shapes already established in `rules_test.go` and `height_test.go` rather than inventing new ones.

  Write one table-driven test whose every case calls both `FixedHeightPins` and `Rules` on the identical `(strands, box, params)` triple and asserts (a) the returned pin list equals the case's expected `[]Pin` exactly — length, order, `PaneID` and `Height` per entry — and (b) that each returned pin's `Height` is the height that pane's cell actually carries in `Rules`' returned layout string, parsed out of that string rather than restated from the expectation.
  Assertion (b) is what makes the test a drift guard rather than a second copy of the policy.

  Cover exactly these cases:
  a header plus two full strands, expecting one pin, the header's;
  a header plus a `ShrinkWhenWaitingOnChild` ancestor with a present descendant, expecting two pins, header first then the strip;
  no header configured (`Params.Header.PaneID == ""`) with a strip present, expecting only the strip pin;
  no header and no strip, expecting a nil or empty pin list;
  a box short enough that `clampHeaderHeight` clamps an oversized `Header.HeightRows`, asserting the header pin carries the clamped value and not the configured one;
  a box short enough that `clampToFit`'s priority-1 pass reclaims strip rows below `CollapsedStripRows`, asserting the strip pin carries the reclaimed value and not `CollapsedStripRows`;
  a header configured with no strand placed (the sole-header branch), asserting no pin at all is returned so a stale one-row pin can never be emitted for a header that owns the whole box;
  and a strand carrying `AnchorOwnWindow`, asserting a nil return rather than a panic, matching `Rules`' error on the same input.

  Add a separate test asserting pin ordering directly: with a header and two distinct strips present, the header pin is index 0 and both strip pins follow.
- **Commit:** `test(reedengine/render): pin FixedHeightPins against Rules' placed cell heights`

## Batch Tests

`verify: go test ./internal/reedengine/render/...` runs the whole `render` leaf package: the new `pins_test.go`, plus `rules_test.go`, `height_test.go`, `policy_test.go` and `checksum_test.go`, all of which must keep passing unchanged.
That whole-package scope is the correct one here rather than an over-broad choice — card 1 edits three of the package's six production files, including the shared `placement` type and the `Rules` composition every other test in the package exercises, so a narrower scope would not see the regressions this batch can cause.
The package is a pure leaf with no I/O, so the run is fast and needs no build tag.
