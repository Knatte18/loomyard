# Batch: render-defaults

```yaml
task: Investigate the unexplained lyx mux server crash
batch: render-defaults
number: 1
cards: 2
verify: go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

Land the render-side legibility fixes: the already-designed-and-verified per-strand
`Display.TopBandRows` override (card 1 reproduces, verbatim, the reviewed uncommitted
diff from the `cluster-fork-spike` worktree), then the shipped `top_band_rows` config
default bump 1 → 3 (card 2). One batch because both cards live in the render/config
default neighbourhood and card 2 must reconcile prose card 1 introduces. External
interface consumed by later batches: none (later batches touch lifecycle/config, not
render policy). Batch-local note: card 1's edits are a faithful re-application of an
operator-verified change — do not redesign it, reproduce it.

## Cards

### Card 1: Per-strand TopBandRows override and --top-band-rows flag

- **Context:**
  - `internal/muxengine/strand.go`
  - `internal/muxengine/render/height.go`
  - `internal/muxengine/render/policy.go`
- **Edits:**
  - `internal/muxcli/add.go`
  - `internal/muxengine/render/types.go`
  - `internal/muxengine/render/rules.go`
  - `internal/muxengine/render/rules_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reproduce the reviewed, operator-verified change exactly (source of
  truth: `git -C /home/knatte/Code/loomyard/wts/cluster-fork-spike diff` — a read-only
  cross-worktree git query, allowed; if that worktree's diff is gone, the four items
  below are the complete specification):
  1. `internal/muxengine/render/types.go` — add field `TopBandRows int` with json tag
     `topBandRows,omitempty` to `Display`, after `ShrinkWhenWaitingOnChild`. Doc
     comment: overrides `Params.TopBandRows` for this AnchorTop strand only when > 0;
     zero inherits the config-wide default; exists because a single global top-band
     height cannot serve both one-line status commands and a full box-drawing TUI
     (which renders corrupted, overlapping frames when given too few rows); ignored
     for any strand not using AnchorTop.
  2. `internal/muxengine/render/rules.go` — in `Rules`, inside the top-band loop,
     after `height := p.TopBandRows` insert:
     `if s.Display.TopBandRows > 0 { height = s.Display.TopBandRows }` — ordered
     BEFORE the `isLastTop` stretch check so the last-top-band stretch still wins.
  3. `internal/muxcli/add.go` — add `topBandRows int` to the flag var block, register
     `cmd.Flags().IntVar(&topBandRows, "top-band-rows", 0, ...)` with help text
     explaining: overrides the config default band height for an `--anchor top` strand
     (0 inherits mux.yaml's `top_band_rows`; needed for a full TUI command sharing the
     top band, which renders corrupted at a too-small height). Wire
     `TopBandRows: topBandRows` into the `render.Display` literal inside the
     `muxengine.AddSpec` construction. Extend the `addCmd` doc comment's flag list with
     `--top-band-rows`.
  4. `internal/muxengine/render/rules_test.go` — add `TestRulesTopBandRowsOverridePerStrand`
     (three AnchorTop strands in `Box{0,0,100,30}`, `Params{TopBandRows: 3,
     CollapsedStripRows: 2, MinFullRows: 3}`; middle strand `Display.TopBandRows: 10`;
     expected layout body `100x30,0,0[100x3,0,0,1,100x10,0,4,2,100x15,0,15,3]` — default
     band, override band, last band stretches) and
     `TestRulesTopBandRowsOverrideIgnoredWhenZero` (two AnchorTop strands in
     `Box{0,0,100,20}`, first with explicit `TopBandRows: 0`; expected body
     `100x20,0,0[100x3,0,0,1,100x16,0,4,2]` — zero inherits the config default, never a
     zero-height pane). Both use `wrapLayout` like the neighbouring tests.
  Per Shared Decision no-root-cause-claims, keep all prose corruption-focused, not
  crash-causal.
- **Commit:** `Add per-strand TopBandRows override and --top-band-rows flag to mux`

### Card 2: Bump shipped top_band_rows default 1 -> 3

- **Context:**
  - `internal/muxengine/config.go`
- **Edits:**
  - `internal/muxengine/template_posix.yaml`
  - `internal/muxengine/template_windows.yaml`
  - `internal/muxengine/config_test.go`
  - `internal/muxcli/add.go`
  - `internal/muxengine/render/types.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `top_band_rows: 1` to `top_band_rows: 3` in BOTH template
  yamls, keeping the trailing comment but extending it to note that 3 rows keeps a
  status command legible and that a per-strand `--top-band-rows` override exists for
  taller needs (a 1-row band shows effectively nothing). Update `config_test.go`'s
  default assertion (`cfg.TopBandRows != 1` at ~line 70) to expect 3, including its
  failure-message literal. Then reconcile prose introduced by card 1 that pins the old
  default: in `add.go`'s `--top-band-rows` flag help and in `types.go`'s
  `Display.TopBandRows` doc comment, refer to "a too-small band height" or "the config
  default" rather than a literal "1-row default"/"1-2 rows", so the text stays true at
  default 3. Run `grep -rn "top_band_rows" internal/ cmd/` and confirm no other
  production or test site pins the old value (the `Params{TopBandRows: 3}` literals in
  render tests are explicit per-test params, not the shipped default — leave them).
- **Commit:** `Bump shipped mux top_band_rows default from 1 to 3`

## Batch Tests

`verify:` runs the untagged unit tests for the whole mux module plus the `cmd/lyx`
guard tests (drift/helptree/registration and the hubgeometry enforcement guard run via
their own packages). Card 1 carries its two new render policy tests; card 2 adjusts the
config default assertion. The `cmd/lyx` scope is included because card 1 changes a
cobra flag surface and the help-tree/drift guards pin command metadata. No smoke run in
this batch — no live psmux behavior changes (layout math is pure).
