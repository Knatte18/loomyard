# Batch: render

```yaml
task: 'Build internal/mux: the window to the world (overlay + strands + render)'
batch: 'render'
number: 3
cards: 7
verify: go test ./internal/muxengine/render/...
depends-on: []
```

## Batch Scope

Creates `internal/muxengine/render`, the pure leaf package that owns the closed display
vocabulary and the deterministic `rules(strands) -> window_layout` function. No I/O, no
psmux, no engine import — this is the golden-file test surface. The external interface
batches 4/5 consume: the value types (`Anchor`, `Display`, `Strand`, `Box`, `Params`) and
`func Rules(strands []Strand, box Box, p Params) (layout string, focus string, err error)`.
Two distinct layers live here and must stay separate (a legibility constraint from the
discussion): **layout policy** (which strand lands where + how tall) vs **layout mechanics**
(the `window_layout` string builder + tmux checksum). Adding an anchor must be a localized
change (a `policy.go` case + its test), never a mechanics rewrite. The checksum and string
format are ported **verbatim** from `internal/muxpoccli/cmd.go`; only the height policy
differs from muxpoc.

**Card order matters:** each card must build on its own without forward-referencing a
symbol a later card creates — types (3) -> checksum (4) -> layout mechanics + `placement`
(5) -> anchor policy (6) -> focus/ancestor helpers (7) -> derived height policy, which
consumes `placement` and `isAncestor` (8) -> `Rules` composition (9).

Batch-local decisions:
- `render.Strand` holds only what layout needs: `GUID`, `Parent` (parent guid or ""),
  `Display`, `PaneID`, `Live`. The engine's opaque `cmd`/`resumeCmd`/`sessionId`/`worktree`
  are **not** in this type (engine maps them out before calling `Rules`).
- `Params` carries the tunable knobs (`TopBandRows`, `CollapsedStripRows`, `MinFullRows`)
  so render stays config-agnostic (the engine passes values loaded from `mux.yaml`).
- `hidden` strands are excluded from the layout string **by construction** (a hidden strand
  never owns a pane). More generally, `partitionByAnchor` (card 6) is the **single filter**
  that drops any strand which is `hidden`, `Live == false`, or has an empty `PaneID`, so render
  only ever lays out strands that own a present window pane and can never emit an empty
  `paneNum` (GAP B).

## Cards

### Card 3: render vocabulary types + package doc

- **Context:**
  - `internal/muxpoccli/cmd.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/render/types.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/render/types.go` (package `render`, with a package
  doc comment describing the policy/mechanics split and the closed anchor vocabulary):
  define `type Anchor string` with the closed set `AnchorTop = "top"`, `AnchorBelowParent =
  "below-parent"`, `AnchorOwnWindow = "own-window"`, `AnchorHidden = "hidden"` (own-window is
  declared but rejected/unused in v1 — document it as deferred). Define `type Display struct
  { Anchor Anchor; Focus bool; ShrinkWhenWaitingOnChild bool }`. Define `type Strand struct
  { GUID string; Parent string; Display Display; PaneID string; Live bool }`. Define `type
  Box struct { X, Y, W, H int }`. Define `type Params struct { TopBandRows, CollapsedStripRows,
  MinFullRows int }`. These are plain data types with no methods.
- **Commit:** `feat(render): add display vocabulary types (Anchor, Display, Strand, Box, Params)`

### Card 4: layout checksum (ported verbatim from muxpoc)

- **Context:**
  - `internal/muxpoccli/cmd.go`
  - `internal/muxpoccli/cmd_test.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/render/checksum.go`
  - `internal/muxengine/render/checksum_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Port `layoutChecksum` from `internal/muxpoccli/cmd.go:279` **verbatim**
  into `internal/muxengine/render/checksum.go` as `func layoutChecksum(s string) string`
  (16-bit rotate-right-1 accumulate over the body bytes, `fmt.Sprintf("%04x", csum)`). In
  `checksum_test.go`, pin the fixture from `muxpoccli/cmd_test.go`: body
  `"220x50,0,0[220x15,0,0,1,220x15,0,16,4,220x18,0,32,3]"` must checksum to `"acd7"`; add a
  shape assertion (any input -> exactly 4 lowercase hex chars).
- **Commit:** `feat(render): port tmux layout checksum verbatim from muxpoc`

### Card 5: layout mechanics — region-relative window_layout string builder

- **Context:**
  - `internal/muxpoccli/cmd.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/checksum.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/render/layout.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/render/layout.go`, add the mechanics layer that
  turns a resolved, ordered list of `(paneID, height)` placements within a `Box` into the
  tmux layout body and full string, mirroring `buildColumnLayout` (`muxpoccli/cmd.go:247`)
  but **region-relative**: `func buildStackBody(box Box, panes []placement) string` emits
  `"<box.W>x<box.H>,<box.X>,<box.Y>[<w>x<h>,<x>,<y>,<paneNum>,...]"` where `paneNum =
  strings.TrimPrefix(id, "%")`, panes ordered top->bottom, cumulative y advancing by paneH+1
  (a 1-row divider) starting at `box.Y`, each pane's width = `box.W` at `box.X`. Add `func
  wrapLayout(body string) string { return layoutChecksum(body) + "," + body }`. Define an
  unexported `type placement struct { id string; height int }`. This card does **not** decide
  heights or ordering (that is the policy cards) — it only builds the string from given
  placements. No test file here; it is exercised via `rules_test.go` (card 9). State that
  choice in Batch Tests.
- **Commit:** `feat(render): add region-relative window_layout string mechanics`

### Card 6: anchor placement policy (dispatch table)

- **Context:**
  - `internal/muxengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/render/policy.go`
  - `internal/muxengine/render/policy_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/render/policy.go`, implement the **legible**
  anchor->placement dispatch that is easy to extend. Provide `func partitionByAnchor(strands
  []Strand) (top []Strand, stack []Strand)` that routes `AnchorTop` strands to a pinned band
  set and `AnchorBelowParent` strands to the stack, and **excludes** (routes to neither set) a
  strand that is `AnchorHidden`, **or has `Live == false`, or has an empty `PaneID`** — render
  is total and only ever lays out strands that actually own a present window pane, so it can
  never emit a `paneNum` from an empty id (this is the single place not-live/pane-less strands
  are filtered out of the layout — see GAP B). It **rejects** `AnchorOwnWindow` (return it in
  neither set — own-window is deferred; also have `Rules` in card 9 surface an error if an
  own-window strand is passed). Provide `func
  orderStack(stack []Strand) []Strand` implementing deterministic ordering: strands ordered by
  parent chain depth (roots first, then children), and **siblings sharing a parent ordered by
  insertion order** (their index in the input slice). Provide cycle-safe traversal: `func
  breakCycles(stack []Strand) []Strand` that walks the parent chain with a visited-set and
  treats a repeat as a root, so a corrupt cyclic table still produces a total ordering (no
  infinite loop). In `policy_test.go`: assert hidden strands are dropped; a `Live==false` **or**
  empty-`PaneID` strand is dropped from both partitions; own-window is not placed; sibling
  insertion order; a cyclic parent table terminates and renders every strand once.
- **Commit:** `feat(render): add legible anchor->placement policy with cycle-safe ordering`

### Card 7: focus + shrinkWhenWaitingOnChild resolution

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/policy.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/render/focus.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/render/focus.go`: `func focusTarget(ordered
  []Strand) string` returns the pane id of the focused strand — exactly one per session: if
  one or more strands have `Display.Focus == true`, pick the **bottom-most** such strand
  (ties resolve to bottom-most); otherwise default to the **bottom-most/active** strand
  (muxpoc's "always select the bottom pane"). Also `func isAncestor(s Strand, ordered
  []Strand) bool` — true when `s` has a visible child below it in the ordered stack — used by
  the height policy (card 8) to decide whether a `shrink:true` strand collapses (a
  `shrink:false` ancestor stays a co-equal full pane). No standalone test file; covered by
  `height_test.go` (card 8) and `rules_test.go` (card 9). State this in Batch Tests.
- **Commit:** `feat(render): focus target selection and ancestor/shrink resolution`

### Card 8: derived height policy + clamp rule

- **Context:**
  - `internal/muxpoccli/cmd.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/layout.go`
  - `internal/muxengine/render/focus.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/render/height_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/render/height.go`, implement the derived height
  policy replacing muxpoc's fixed `activePaneShare`. `func stackHeights(stack []Strand, box
  Box, p Params) []placement` (returning the `placement` type from card 5's `layout.go`, and
  using `isAncestor` from card 7's `focus.go`) computes: usable height `Hu = box.H - dividers`
  (one row per gap between panes); each `top` band is a fixed `p.TopBandRows` (handled by the
  caller/card 9 which reserves top bands before calling); within the below-parent stack, a
  **shrink:true ancestor** (a strand that has a visible child below it in the chain, via
  `isAncestor`) collapses to `p.CollapsedStripRows`; the **active/bottom strand plus every
  shrink:false strand** are "full" panes splitting the remaining rows **equally**, with the
  **integer-division remainder assigned to the active/bottom pane** (others get the floor) so
  heights sum exactly (the determinism rule). Implement the **clamp** in strict priority when
  fixed demand exceeds the window: (1) shrink strips toward 1 row, (2) reduce full panes
  equally toward `p.MinFullRows`, (3) last resort keep the active pane at the remainder and
  clamp earlier panes to 1 row. `stackHeights` must **never** return a non-positive height.
  In `height_test.go`, parameterize over `CollapsedStripRows` and assert: heights + 1-row
  dividers exactly fill `box.H`; each collapsed ancestor == `CollapsedStripRows`; active pane
  is strictly tallest with a single ancestor; remainder-with->=2-full-panes goes to the
  active/bottom pane deterministically; a too-short window still yields only positive heights
  via the clamp order.
- **Commit:** `feat(render): derived height policy with deterministic remainder and clamp rule`

### Card 9: Rules() composition + golden tests

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/checksum.go`
  - `internal/muxengine/render/layout.go`
  - `internal/muxengine/render/policy.go`
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/render/focus.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/rules_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/muxengine/render/rules.go`, implement the public entry
  `func Rules(strands []Strand, box Box, p Params) (layout string, focus string, err error)`
  — name the second return `focus` (NOT `focusTarget`, which would shadow the `focusTarget`
  helper from card 7). Body: reject any `AnchorOwnWindow` strand with a non-nil error
  (deferred anchor); call `breakCycles` then `partitionByAnchor`; reserve
  `len(top)*p.TopBandRows` rows (+dividers) as fixed top bands at the top of `box`; run
  `orderStack` + `stackHeights` for the below-parent region below the bands; assemble all
  placements (top bands then stack) via `buildStackBody` + `wrapLayout`; set `focus =
  focusTarget(ordered)`. The function is pure and total for any non-own-window input. In
  `rules_test.go`, add golden / table tests over strand sets: `top` pinned as a fixed band
  above the stack; `below-parent` forms the bottom-dominant stack ordered by parent chain;
  `hidden` strands excluded from the string; mixed set (top + stack + hidden); empty and
  single-strand edge cases; the emitted checksum prefix always equals `layoutChecksum(body)`;
  own-window input returns an error. Assert full layout strings for at least the canonical
  multi-strand cases (golden).
- **Commit:** `feat(render): compose Rules() over policy+mechanics with golden tests`

## Batch Tests

`verify: go test ./internal/muxengine/render/...` covers the entire pure package. Cards 5
(`layout.go`) and 7 (`focus.go`) intentionally ship no standalone `_test.go` — they are
exercised through `height_test.go` (card 8) and the golden `rules_test.go` (card 9), which
assert the end-to-end layout strings and thus their outputs. Everything is pure (no psmux, no
agents), so the whole batch runs under the default `go test` with no build tag.
