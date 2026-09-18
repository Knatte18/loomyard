# Batch: render-bottom-band

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
batch: "render-bottom-band"
number: 1
cards: 6
verify: go test ./internal/reedengine/render/ ./internal/reedengine/
depends-on: []
```

## Batch Scope

This batch flips `internal/reedengine/render`'s fixed-height band from the top edge to the bottom edge and renames its vocabulary from Header to Selvage, then updates the single engine-side call site (`internal/reedengine/apply.go`'s `toRenderInputs`) so the tree still compiles and `internal/reedengine`'s own tests still pass.
It is one batch because `render` is a pure leaf whose four production files (`types.go`, `height.go`, `layout.go`, `rules.go`, plus one helper signature in `policy.go`) share a single geometry decision — where the band sits — and because `apply.go` is the only production consumer of the renamed type, so splitting it out would leave the repository uncompilable between batches.

The external interface the next batch consumes: `render.Selvage{PaneID, HeightRows}` and `render.Params.Selvage` replace `render.Header`/`Params.Header`, and `render.Rules` now emits the band cell **last** in the layout body with the strand stack starting at `box.Y`.

Batch-local decision beyond `## Shared Decisions`: `render`'s internal height helper is renamed `clampBandHeight` rather than `clampSelvageHeight` — the package models a generic fixed band and deliberately does not learn what its caller uses the band for.
`ReedState.HeaderPaneID` and `Config.Header` are **not** touched here; `apply.go` keeps reading both under their current names in this batch and both are renamed later (batch 2 for the config block, batch 3 for the state field).

## Cards

### Card 1: rename render's band vocabulary to Selvage

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/reedengine/render/types.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the exported type `Header` to `Selvage` in `internal/reedengine/render/types.go`, keeping both its fields (`PaneID string`, `HeightRows int`) unchanged. Rename the `Params.Header` field to `Params.Selvage` with type `Selvage`. Rewrite `Selvage`'s doc comment so it describes a fixed-height **bottom** band below the below-parent stack rather than a top band above it, keeps the "a zero-value Selvage (empty PaneID) means no band" contract verbatim, keeps the "never a Strand — injected at this Params seam" sentence, and names `clampBandHeight` instead of `clampHeaderHeight` as the floor-preserving adjuster. Update `Params.MinFullRows`'s own doc comment, which today reads "clampHeaderHeight also uses this as the strand-stack region's floor when a header pane is present" — it must name `clampBandHeight` and the Selvage band. Update `Params.Selvage`'s field comment the same way. Do not rename `Anchor`, `Display`, `Strand` or `Box`, and do not add a position enum — per the discussion's `selvage-is-a-bottom-band-not-a-strand` decision there is exactly one band and it is at the bottom.
- **Commit:** `refactor(render): rename the Header band type to Selvage`

### Card 2: rename clampHeaderHeight to clampBandHeight

- **Context:**
  - `internal/reedengine/render/types.go`
- **Edits:**
  - `internal/reedengine/render/height.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the unexported function `clampHeaderHeight` to `clampBandHeight` in `internal/reedengine/render/height.go`, renaming its first parameter `headerRows` to `bandRows` and keeping the parameters `windowRows` and `minStackRows` unchanged. Its body and semantics stay **verbatim** per the discussion's `band-clamp-and-degenerate-cases-carry-over-unchanged` decision: the band yields rows first so the strand stack keeps its `minStackRows` floor, and the `maxHeader < 1` branch (renamed to its `maxBand` local) still floors the band at 1 row so a zero-height band cell is never produced. Update the doc comment and the inline comment inside that branch to say "band" rather than "header" while keeping the same reasoning — a starved stack strand still renders because `clampToFit` floors every strand at 1 row, while a zero-height band cell is mishandled by the real multiplexer. Do not change `stackHeights` or `clampToFit`.
- **Commit:** `refactor(render): rename clampHeaderHeight to clampBandHeight`

### Card 3: splice the band cell last instead of first

- **Context:**
  - `internal/reedengine/render/types.go`
- **Edits:**
  - `internal/reedengine/render/layout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename `bandHeader` to `bandSelvage` in `internal/reedengine/render/layout.go` and change its parameter name `headerPaneID` to `selvagePaneID` and `headerHeight` to `bandHeight`. Change its behaviour from prepending to **appending**: the band cell must be emitted after `stackBody`'s inner cells rather than before them, and its `y` offset must be `fullBox.Y + fullBox.H - bandHeight` rather than `fullBox.Y`. Concretely, the emitted body is `<fullBox.W>x<fullBox.H>,<fullBox.X>,<fullBox.Y>[` followed by `stackBody`'s inner content (the substring between its `[` and its last `]`), then — when that inner content is non-empty — a `,` separator, then `<fullBox.W>x<bandHeight>,<fullBox.X>,<fullBox.Y+fullBox.H-bandHeight>,<selvagePaneID minus its leading '%'>`, then `]`. Keep the existing `strings.IndexByte(stackBody, '[')` / `strings.LastIndexByte(stackBody, ']')` extraction shape and the empty-inner guard. Update the doc comment to say it appends a fixed-height Selvage cell to stackBody's pane group. Do not change `buildStackBody`, `wrapLayout` or `placement`.
- **Commit:** `refactor(render): emit the fixed band cell at the bottom of the layout body`

### Card 4: rewrite planCells, Rules and FixedHeightPins for a bottom band

- **Context:**
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/render/height.go`
  - `internal/reedengine/render/layout.go`
  - `internal/reedengine/render/policy.go`
- **Edits:**
  - `internal/reedengine/render/rules.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/render/rules.go`, rename `cellPlan`'s fields `hasHeader` → `hasBand`, `soleHeader` → `soleBand`, `headerHeight` → `bandHeight`, updating each field's doc comment to name the Selvage band. In `planCells`: read `p.Selvage.PaneID`/`p.Selvage.HeightRows` in place of `p.Header.*`; pass `p.Selvage.PaneID` to `removeDuplicatePaneCells`; keep the sole-band early return (no strand placed → `cellPlan{hasBand: true, soleBand: true}`) exactly as it is today, since per the discussion's `band-clamp-and-degenerate-cases-carry-over-unchanged` decision reed must still emit a bracket-less single cell rather than a zero-height cell inside a group; and rewrite the stack-box math so the stack occupies the **top** of the box instead of the bottom — rename the `headerDivider` const to `bandDivider` (still 1), compute `bandHeight = clampBandHeight(p.Selvage.HeightRows, box.H-bandDivider, p.MinFullRows)`, and set `stackBox = Box{X: box.X, Y: box.Y, W: box.W, H: box.H - bandHeight - bandDivider}`. Update the long comment above that block so it still explains why the divider row is budgeted before the clamp, with "header" replaced by the Selvage band. In `Rules`: build the sole-band layout string from `p.Selvage.PaneID`, and call `bandSelvage(box, p.Selvage.PaneID, plan.bandHeight, body)` after `buildStackBody`. In `FixedHeightPins`: emit the band pin from `p.Selvage.PaneID` at `plan.bandHeight`, keeping it **first** in the returned slice ahead of every strip pin — per the discussion's gotcha that hook-array index is fire order, not screen position, so the band pin keeps index 0 even though the band is now at the bottom. Update `Rules`' and `FixedHeightPins`' doc comments to describe a fixed-height bottom band. Do not change `resequenceByPaneOrder` or `Pin`.
- **Commit:** `feat(render): lay the Selvage band out at the bottom of the window`

### Card 5: retarget removeDuplicatePaneCells' parameter name

- **Context:**
  - `internal/reedengine/render/rules.go`
- **Edits:**
  - `internal/reedengine/render/policy.go`
  - `internal/reedengine/render/policy_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/render/policy.go`, rename `removeDuplicatePaneCells`' second parameter from `headerPaneID` to `bandPaneID` and update the function's doc comment: the paragraph naming `bandHeader` as the reason a duplicate is reachable must name `bandSelvage` instead, and the "The header is the case that makes this reachable" sentence must name the Selvage band. The body's logic is unchanged. In `internal/reedengine/render/policy_test.go`, update any test that passes a band pane id to this helper so its local variable names and subtest names name Selvage rather than the header; `TestPartitionByAnchor`, `TestOrderStackSiblingInsertionOrder` and `TestBreakCyclesTerminatesAndKeepsEveryStrand` need no behavioural change.
- **Commit:** `refactor(render): name removeDuplicatePaneCells' band parameter for Selvage`

### Card 6: rewrite the render tests for the bottom band and retarget apply.go

- **Context:**
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/layout.go`
  - `internal/reedengine/render/height.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/config.go`
- **Edits:**
  - `internal/reedengine/render/rules_test.go`
  - `internal/reedengine/render/height_test.go`
  - `internal/reedengine/render/pins_test.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/apply_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the existing top-band fixtures rather than merely renaming them — they encode the old geometry byte for byte in their expected layout strings. In `internal/reedengine/render/rules_test.go`: rename `TestRulesHeaderBandEnumeratesHeaderPlusEveryStrandCell` to `TestRulesSelvageBandEnumeratesEveryStrandCellPlusSelvage` and assert the band cell is emitted **last** and at `y = box.Y + box.H - bandHeight`, with the first strand cell at `y = box.Y`; rename `TestRulesHeaderWithNoPlacedStrandClaimsWholeBoxAsSoleCell` to name Selvage, keeping its assertion that the sole-band body is a bracket-less single cell claiming the whole box; rename `TestRulesNoHeaderPreservesPreHeaderBehavior` to name Selvage, keeping its assertion that a zero-value `Params.Selvage` lays out exactly as before the band existed; update every other test in the file that constructs `render.Params{Header: ...}` to `render.Params{Selvage: ...}`, and update `TestRulesGolden`'s expected strings for the new cell order and offsets. In `height_test.go`: rename `TestClampHeaderHeight` to `TestClampBandHeight`, call `clampBandHeight`, and keep its cases' semantics verbatim — the band yields rows to the stack's floor first, and is never starved below 1 row. In `pins_test.go`: rename `TestFixedHeightPinsOrdersTheHeaderPinFirstThenEveryStripPin` to name the Selvage band pin, and keep its assertion that the band pin is **first** in the returned slice; update `TestFixedHeightPinsMatchesRulesPlacedHeights` for the new `Params.Selvage` field and the new placed offsets. Add one new test asserting that cell offsets and heights sum to the box with the one-row divider accounted for: for a box of height H with a band of height B and n placed strands, the strand cells occupy rows `box.Y .. box.Y+H-B-2` and the band cell occupies rows `box.Y+H-B .. box.Y+H-1`. In `internal/reedengine/apply.go`'s `toRenderInputs`, change the `render.Params` literal's `Header: render.Header{PaneID: headerPaneID, HeightRows: e.cfg.Header.HeightRows}` to `Selvage: render.Selvage{PaneID: headerPaneID, HeightRows: e.cfg.Header.HeightRows}` — the local `headerPaneID`, `st.HeaderPaneID` and `e.cfg.Header` all keep their current names in this batch and are renamed in batches 2 and 3. Update `renderInputs`' and `toRenderInputs`' doc comments to name the Selvage band. In `internal/reedengine/apply_test.go`, update every `render.Params{Header: ...}` construction and every assertion naming the header band's layout position to the new bottom-band geometry.
- **Commit:** `test(render): rewrite the band fixtures for the bottom-band geometry`

## Batch Tests

`verify: go test ./internal/reedengine/render/ ./internal/reedengine/` covers both the rewritten pure-leaf suite (`rules_test.go`, `height_test.go`, `pins_test.go`, `policy_test.go`, `checksum_test.go`) and the engine package that consumes the renamed type (`apply_test.go` in particular, plus every other untagged `internal/reedengine` test, which must stay green because this batch changes no engine behaviour beyond the `render.Params` field name).
The overview's module-wide `verify: go build ./...` catches any other package that fails to compile against the renamed `render` surface at this batch's boundary.
`render` is a pure leaf with no I/O, no tmux and no engine import, so every assertion here is a table-driven comparison of layout strings and pin slices — no tagged tier is involved and none of this batch's tests spawn a process.
