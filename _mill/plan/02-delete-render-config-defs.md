# Batch: delete-render-config-defs

```yaml
task: "Reconsider whether lyx mux needs anchor:top at all"
batch: delete-render-config-defs
number: 2
cards: 4
verify: go build ./... && go test ./internal/muxengine/...
depends-on: [1]
```

## Batch Scope

With batch 1 having removed every external reference, this batch deletes the definitions
themselves: the `render.AnchorTop` const, the `Display.TopBandRows` and `Params.TopBandRows`
fields, the top-band placement logic in `policy.go`/`rules.go`, the `muxengine.Config.TopBandRows`
field, the `top_band_rows` template lines, and the residual `top`/`top-band` prose in the doc
comments of these files. It touches **only** `render/{types,policy,rules}.go`, `config.go`,
`template.go`, and the two template yamls — a file set disjoint from batch 1, so both batches
compile and test green independently. After this batch the `Anchor` vocabulary is
`below-parent | own-window(deferred) | hidden` and no top-band code, config, or comment remains.

Batch-local decision: `partitionByAnchor` is simplified to return only the below-parent stack
(discussion Decision `keep-partitionByAnchor-simplified`) — a `(top, stack)` signature with a
permanently-empty `top` return is dead structure. `Rules` correspondingly loses its top-band
`y`-cursor loop, the last-band stretch special case, and the `focus == "" && len(top) > 0`
fallback. The `AnchorOwnWindow` rejection at the top of `Rules`, the `AnchorHidden`/non-live/
empty-`PaneID` filter, `height.go`, `focus.go`, and the mechanics layer (`layout.go`,
`checksum.go`) are untouched — this stays a policy-layer change (render two-layer split rule).

## Cards

### Card 9: delete AnchorTop const and TopBandRows fields from render types

- **Context:**
  - `internal/muxengine/render/policy.go`
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/height.go`
- **Edits:**
  - `internal/muxengine/render/types.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the `AnchorTop` const from the `Anchor` block; remove the
  `TopBandRows` field from `Display`; remove the `TopBandRows` field from `Params`. Update the
  doc comments so they no longer describe a top band: the `Anchor` type comment ("Render
  recognizes exactly these four values" → three) and the package doc's two-layer paragraph if it
  cites `top_band_rows`; delete the `AnchorTop` godoc block and the `Display.TopBandRows` /
  `Params.TopBandRows` field comments. Keep `AnchorBelowParent`, `AnchorOwnWindow`,
  `AnchorHidden`, and the other `Display`/`Params` fields intact. This card and cards 10-11 must
  land together in this batch — `policy.go`/`rules.go` reference these symbols until their own
  cards remove those uses.
- **Commit:** `refactor(render): remove AnchorTop and TopBandRows from the display vocabulary`

### Card 10: simplify partitionByAnchor to stack-only

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/height.go`
- **Edits:**
  - `internal/muxengine/render/policy.go`
  - `internal/muxengine/render/policy_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `partitionByAnchor` to return only the below-parent stack (drop the
  `top []Strand` return and the `case AnchorTop:` arm). Keep the exclusion filter (`AnchorHidden
  || !Live || PaneID == ""`) and the `AnchorOwnWindow`/default no-op arm. Update the function's
  and file's doc comments to remove "the fixed top-band set" language and describe a single
  stack partition. `breakCycles`, `orderStack`, `chainDepth`, `severParent` are unchanged.
  Coordinate the new signature with card 11's `Rules` call site. In `policy_test.go`, migrate
  `TestPartitionByAnchor` to the single-return signature (`stack := partitionByAnchor(...)` at
  the call sites currently written as `top, stack := ...`): remove the top-partition assertion
  cases entirely and keep the exclusion-filter (non-live / empty-`PaneID` / `AnchorHidden`) and
  `AnchorOwnWindow`-excluded cases, asserting only the returned stack. After this card
  `policy_test.go` carries no `render.AnchorTop` reference and calls the new one-value signature.
  This file is edited only in this batch — batch 1 leaves it compiling against the old two-value
  signature, so the signature change and its test migration land together here (per the
  round-1 review BLOCKING fix).
- **Commit:** `refactor(render): partitionByAnchor returns only the below-parent stack`

### Card 11: strip the top-band placement block from Rules

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/policy.go`
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/render/focus.go`
- **Edits:**
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/layout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `Rules` to the below-parent-only flow: keep the `AnchorOwnWindow`
  rejection loop and `breakCycles`, take the single stack from the simplified
  `partitionByAnchor` (card 10), `orderStack` it, run `stackHeights` over the full `box`
  (`stackBox` = `box`), `resequenceByPaneOrder`, `buildStackBody`/`wrapLayout`, and
  `focusTarget`. Delete the top-band reservation `for` loop over `top` (the `y`-cursor,
  `TopBandRows` height lookup, per-band divider, and the `isLastTop && len(ordered) == 0`
  stretch special case) and the `focus == "" && len(top) > 0` fallback (with no top bands there
  is always either a stack or nothing to focus). Update the `Rules` doc comment and the
  `paneOrder` contract comment to drop "top bands first"/top-band language while preserving the
  positional-`select-layout` explanation that still governs the stack. `resequenceByPaneOrder`
  and `buildStackBody` are unchanged. Also reword `layout.go`'s file-header doc comment (round-1
  review NIT fix) to drop the "top-band region and the below-parent stack region can each be
  rendered independently" clause — it is mechanics-layer prose only, no logic in `layout.go`
  changes.
- **Commit:** `refactor(render): remove top-band placement from Rules`

### Card 12: delete Config.TopBandRows and the top_band_rows template lines

- **Context:**
  - `internal/muxengine/apply.go`
- **Edits:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/template.go`
  - `internal/muxengine/template_posix.yaml`
  - `internal/muxengine/template_windows.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the `TopBandRows int \`yaml:"top_band_rows"\`` field from the config
  struct in `config.go` (line 27). Delete the `top_band_rows: 3 ...` line from both
  `template_posix.yaml` and `template_windows.yaml` (line 6 in each). In `template.go`, remove
  `top_band_rows` from the config-key-list comment (line 15). `CollapsedStripRows` and
  `MinFullRows` stay. `apply.go` no longer reads `e.cfg.TopBandRows` (batch 1), so nothing
  references the removed field.
- **Commit:** `refactor(muxengine): drop top_band_rows config field and template lines`

## Batch Tests

`verify: go build ./... && go test ./internal/muxengine/...`

- `go build ./...` proves the whole module compiles after the symbol deletions — the critical
  cross-package regression catch for this batch (nothing outside the edited files may still
  reference the removed symbols; batch 1 guaranteed that).
- `go test ./internal/muxengine/...` runs the `render` unit tests (cards 9-11: the retained
  goldens from batch-1 card 1 now exercise the below-parent-only `Rules`, `partitionByAnchor`,
  and `stackHeights`) and the `muxengine` engine tests (card 12: config loading with
  `top_band_rows` gone). No smoke recompile is needed — the smoke tests reference `top` only as
  a CLI string (rewritten in batch 1), never the deleted `render.AnchorTop` symbol.
