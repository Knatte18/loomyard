# Batch: dereference-consumers

```yaml
task: "Reconsider whether lyx mux needs anchor:top at all"
batch: dereference-consumers
number: 1
cards: 8
verify: go build ./... && go test ./internal/muxengine/... ./internal/muxcli/... ./internal/shuttleengine/... ./internal/shuttlecli/... && go test -tags smoke -run '^$' ./internal/muxcli/
depends-on: []
```

## Batch Scope

This batch removes every *reference* to the doomed symbols — `render.AnchorTop`,
`render.Display.TopBandRows`, `render.Params.TopBandRows`, and `muxengine.Config.TopBandRows`
— from all consumers and tests, while the symbol *definitions* remain in place (batch 2
deletes those). After this batch: no file outside `render/{types,policy,rules}.go`,
`config.go`, `template.go`, and the two template yamls references any top-band symbol, and the
CLI/engine reject `top`. The tree still compiles and every non-smoke test passes because the
retained definitions are self-consistent (an `AnchorTop` strand would still render as a top
band — nothing exercises that path anymore). The external contract the next batch consumes:
the render top-band symbols and `Config.TopBandRows` are now referenced only at their own
definition sites, so batch 2 can delete them without touching any file in this batch.

Batch-local note (differs from nothing in Shared Decisions): incidental fixtures that used
`render.AnchorTop` purely as "some anchor value" are swapped to `render.AnchorBelowParent`; the
`Display` round-trip and unbound-strand filters they assert are anchor-agnostic, so the swap
does not change what those tests prove.

## Cards

### Card 1: strip top-band cases from render's rules tests

- **Context:**
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/policy.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/render/height_test.go`
- **Edits:**
  - `internal/muxengine/render/rules_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `rules_test.go`, remove all top-band cases that exercise
  `render.AnchorTop` or `render.Display.TopBandRows`: the top-band golden cases in
  `TestRulesGolden` (top-band placement, the `>=2 top / 0 stack` last-band stretch, every
  top+stack mix) and the per-strand `Display.TopBandRows` override cases. **Do not delete** the
  paneOrder tests (`TestRulesPaneOrderResequencesCellsToPhysicalOrder` and
  `TestRulesPaneOrderUnknownIDsKeepIntendedTailOrder`) even though they are built on
  `AnchorTop` fixtures — `resequenceByPaneOrder` is retained unchanged by batch 2 card 11, so
  re-express both against a `below-parent` parent+child stack with an inverted `paneOrder`
  (preserving the same positional-reorder and unknown-id-tail assertions) rather than losing
  the coverage. After this card **no `render.AnchorTop` or `TopBandRows` reference may remain in
  `rules_test.go`** (so batch 2 can delete the symbols). Keep — and if absent, add — a
  `TestRulesGolden` (or sibling) case proving the loom shape at the `Rules` level: (a) a single
  lone `below-parent` strand fills the full box; (b) a `below-parent` root parent + one child
  collapses the parent to `CollapsedStripRows` with the child taking the remainder. Case (a) is
  the childless-full-height behavior the task explicitly endorses (discussion Decision
  `childless-full-height-is-acceptable`); `height_test.go`'s
  `TestStackHeightsActiveStrictlyTallestWithSingleAncestor` already covers the height-layer form
  of (b) — do not duplicate it, only ensure the `Rules`-level golden exists. `policy_test.go` is
  **not** touched here — its `partitionByAnchor` two-return call sites are migrated in batch 2
  card 10 alongside the signature change, so it must keep compiling unedited through this batch.
- **Commit:** `test(render): drop anchor:top golden and override cases`

### Card 2: swap incidental AnchorTop fixtures in engine tests

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/state.go`
- **Edits:**
  - `internal/muxengine/state_test.go`
  - `internal/muxengine/io_test.go`
  - `internal/muxengine/contract_integration_test.go`
  - `internal/muxengine/lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace every `render.AnchorTop` with `render.AnchorBelowParent` in these
  four files (`state_test.go:56,111`, `io_test.go:29`, `contract_integration_test.go:377`,
  `lifecycle_test.go:31`). These fixtures use the anchor value incidentally (persistence
  round-trip, unbound/hidden filters, lifecycle strand records) and assert nothing top-band
  specific, so the swap must not change any assertion. If any assertion depended on top-band
  placement geometry after the swap, that is a plan defect — surface it rather than hand-editing
  expected geometry.
- **Commit:** `test(muxengine): swap incidental anchor:top fixtures to below-parent`

### Card 3: validateAnchor drops top + strand_test updates

- **Context:**
  - `internal/muxengine/render/types.go`
- **Edits:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `strand.go`'s `validateAnchor`, remove `render.AnchorTop` from the
  accepted `case` (so an incoming `top` anchor is rejected) and change the error string
  `"invalid anchor %q; want top|below-parent|hidden"` to `"invalid anchor %q; want
  below-parent|hidden"`. In `strand_test.go`: delete the `{"Top_Launches", render.AnchorTop,
  true}` table row (line 151) since `top` no longer exists as a launching anchor — the
  `below-parent` row already covers the `true` case; swap the incidental `render.AnchorTop`
  fixture at line 308 to `render.AnchorBelowParent`; if a `validateAnchor` test asserts `top`
  is *accepted*, retarget it to assert `render.Anchor("top")` is now *rejected* with the new
  error string. After this card `strand.go`/`strand_test.go` carry no `render.AnchorTop`
  reference.
- **Commit:** `feat(muxengine): reject anchor:top in validateAnchor`

### Card 4: stop wiring TopBandRows through apply + apply_test

- **Context:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/rules.go`
- **Edits:**
  - `internal/muxengine/apply.go`
  - `internal/muxengine/apply_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `apply.go`, remove the `TopBandRows: e.cfg.TopBandRows,` line from the
  `render.Params{...}` literal (line 75) — after this, `e.cfg.TopBandRows` is read by nothing
  (batch 2 removes the field). In `apply_test.go`: remove `TopBandRows` from every `e.cfg`
  assignment and `render.Params{...}` literal (lines 18, 34, 54, 69); swap the `render.AnchorTop`
  in the `UnboundStrand` case (line 137) to `render.AnchorBelowParent` (that case tests the
  empty-`PaneID` exclusion filter — anchor value is incidental). The layout-asserting cases at
  lines 34/69 use `below-parent` strands, so `TopBandRows` was already unused there and its
  removal must not change the expected `window_layout` output; if a golden changes, it means a
  case actually depended on a top band — surface it, do not silently re-baseline.
- **Commit:** `refactor(muxengine): stop passing TopBandRows into render.Params`

### Card 5: drop TopBandRows from config_test and lock_test

- **Context:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/template_posix.yaml`
- **Edits:**
  - `internal/muxengine/config_test.go`
  - `internal/muxengine/lock_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `config_test.go`, remove the `cfg.TopBandRows != 3` assertion block
  (lines 70-72, i.e. the `if` line, the `t.Errorf` line, and the closing brace) — do not
  replace it; the remaining `collapsed_strip_rows`/`min_full_rows` assertions still prove config
  loading. In `lock_test.go`, remove the `TopBandRows: 3,` entry
  from its config/Params literal (line 36). After this card neither file references
  `TopBandRows`.
- **Commit:** `test(muxengine): drop TopBandRows assertions from config and lock tests`

### Card 6: remove anchor:top surface from the add CLI

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/strand.go`
- **Edits:**
  - `internal/muxcli/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `addCmd`: remove `render.AnchorTop` from the anchor-vocabulary `switch`
  (so `--anchor top` falls to the invalid-anchor branch) and change that branch's error string
  `"invalid --anchor %q; want top|below-parent|hidden"` to `"...; want below-parent|hidden"`
  (line 66); delete the `--focus`-with-`--anchor top` rejection guard block (lines 73-76);
  delete the `--top-band-rows` flag registration (line 115), the `topBandRows` var declaration,
  and the `TopBandRows: topBandRows` field in the `render.Display{...}` literal; change the
  `--anchor` flag usage string `"placement: top|below-parent|hidden"` to `"placement:
  below-parent|hidden"` (line 113); if the command `Long` text names `top` as a placement,
  update it to the two-value vocabulary. After this card `add.go` has no `render.AnchorTop` or
  `TopBandRows` reference. Keep `Short` intact (CLI/Cobra invariant).
- **Commit:** `feat(muxcli): remove --anchor top and --top-band-rows from add`

### Card 7: rewrite the smoke lifecycle two-top-band test

- **Context:**
  - `internal/muxcli/smoke_test.go`
  - `internal/muxcli/add.go`
  - `internal/muxengine/render/rules.go`
- **Edits:**
  - `internal/muxcli/smoke_lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The lifecycle smoke test (around lines 91-121) builds two `--anchor top`
  strands (`band1`, `band2`) to reproduce a split-path defect where `>=2 top / 0 stack`
  panes must still tile the window. With `anchor:top` removed, re-express the same multi-pane
  split concern using `below-parent` strands (e.g. a parent strand plus a below-parent child,
  or two below-parent siblings) so the test still exercises a multi-pane `select-layout` apply
  and its stray-state assertions. Replace the `"--anchor", "top"` argument pairs accordingly and
  update the comment at line 91. The file is `//go:build smoke` tagged and is not run here — it
  must only **compile** under `-tags smoke` (the batch verify runs `go test -tags smoke -run
  '^$'`). If the defect was intrinsically about top-band tiling and has no below-parent analog,
  instead delete the two `band` strands and their assertions and add a one-line comment stating
  the top-band split-path scenario retired with `anchor:top`; flag this choice for the reviewer.
- **Commit:** `test(muxcli): re-express two-top-band smoke case without anchor:top`

### Card 8: swap shuttleengine fixtures and fix shuttlecli anchor help

- **Context:**
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/strand.go`
- **Edits:**
  - `internal/shuttleengine/spec_test.go`
  - `internal/shuttleengine/run_test.go`
  - `internal/shuttlecli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace every `render.AnchorTop` with `render.AnchorBelowParent` in
  `spec_test.go` (lines 183, 187, 188) and `run_test.go` (lines 44, 66). These assert that
  `Spec.Display` round-trips unchanged; the anchor value is incidental, so the swap must not
  change what the tests prove. In `shuttlecli/run.go`, change the `--anchor` flag usage string
  (line 149) from `"placement: top|below-parent|hidden"` to `"placement: below-parent|hidden"`.
  `shuttlecli` does not vet the anchor vocabulary itself — it passes `render.Anchor(anchor)`
  straight to the engine (line 115), whose `validateAnchor` (card 3) now rejects `top` at
  runtime — so only the stale help string is dead surface here; do **not** add a CLI-level
  vocabulary switch (out of scope, and the engine already gates it). `shuttlecli/run.go` never
  references the `render.AnchorTop` symbol (only `render.Anchor(...)` and
  `render.AnchorBelowParent`), so batch 2's symbol deletion does not touch it. After this card
  no shuttle file references `render.AnchorTop` or advertises `top`.
- **Commit:** `refactor(shuttle): drop anchor:top from run.go help and test fixtures`

## Batch Tests

`verify: go build ./... && go test ./internal/muxengine/... ./internal/muxcli/...
./internal/shuttleengine/... ./internal/shuttlecli/... && go test -tags smoke -run '^$'
./internal/muxcli/`

- `go build ./...` proves the whole module still compiles with every consumer de-referenced
  (the retained top-band definitions are now referenced only at their own sites).
- `go test ./internal/muxengine/...` covers the render unit tests (cards 1, 4) and the engine
  tests (cards 2, 3, 4, 5) — `render`, plus `muxengine` (`apply_test`, `state_test`,
  `strand_test`, `io_test`, `contract_integration_test`, `lifecycle_test`, `lock_test`,
  `config_test`).
- `go test ./internal/muxcli/...` runs the non-smoke CLI tests (card 6's `add.go` change is
  exercised by the vocabulary/help surface; smoke tests are excluded by the missing `smoke`
  tag).
- `go test ./internal/shuttleengine/... ./internal/shuttlecli/...` covers card 8 — the
  shuttleengine `Display` round-trip fixtures and the `shuttlecli/run.go` `--anchor` help-string
  change (the non-smoke `shuttlecli/cli_test.go` exercises the CLI surface; `shuttlecli` smoke
  files are excluded by the missing `smoke` tag and reference `top` only as a string, never the
  deleted symbol, so no smoke recompile is needed there).
- `go test -tags smoke -run '^$' ./internal/muxcli/` compiles the `//go:build smoke` files —
  including card 7's rewritten `smoke_lifecycle_test.go` — and runs zero tests, so it needs no
  live psmux. This is the only way the default build reaches the smoke sources; without it a
  compile break in the rewrite would go unnoticed here.
