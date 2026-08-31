# Batch: collapsed-strip-default

```yaml
task: "Reconsider the collapsed strand strip default size"
batch: "collapsed-strip-default"
number: 1
cards: 1
verify: go test ./internal/reedengine/...
depends-on: []
```

## Batch Scope

This batch delivers the whole task: `collapsed_strip_rows` moves from `3` to `6` in both platform templates, both template inline comments gain the readability rationale plus the reconcile adoption caveat the sibling `mouse`/`watchdog` keys already carry, the two `config_test.go` template-default assertions move to `6`, and `doc.go`'s silent-layout-rescale anecdote marks its `3` as the then-default.
It is one batch because it is one value change plus the assertions and prose that pin it — there is no second subsystem to sequence against, and no external interface for a later batch to consume.
The batch-local decisions are exactly the three in `## Shared Decisions` in `00-overview.md`;
this batch adds none of its own.

## Cards

### Card 1: Raise collapsed_strip_rows to 6 across both templates, their comments, the default assertions, and doc.go

- **Context:**
  - `internal/reedengine/config.go`
  - `internal/reedengine/attachgeometry_integration_test.go`
- **Edits:**
  - `internal/reedengine/template_posix.yaml`
  - `internal/reedengine/template_windows.yaml`
  - `internal/reedengine/config_test.go`
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Work in this order so the change is test-first, and land all of it in the single commit named below.
  (1) In `internal/reedengine/config_test.go`, change both `CollapsedStripRows` default assertions from `3` to `6` — the one at line 61 in the explicit-template test (`if cfg.CollapsedStripRows != 3 {` and its `t.Errorf("CollapsedStripRows = %d, want 3", cfg.CollapsedStripRows)`) and the one at line 138 in the degrade-to-embedded-template test (identical two lines).
  Change the `want 3` text inside both `t.Errorf` format strings to `want 6` as well.
  Do not alter any sibling assertion in either test — `Tmux`, `Shell`, `Width != 220`, `Height != 50`, `MinFullRows != 3`, `StrandName`, `DebugLog`, `Mouse`, and `Header.HeightRows != 1` all stay exactly as they are, and `MinFullRows` in particular stays `3`.
  Run `go test ./internal/reedengine/...` at this point and confirm both assertions fail against the still-unchanged templates.
  (2) In `internal/reedengine/template_posix.yaml`, replace the whole of line 5 with this exact line: `collapsed_strip_rows: 6  # height a shrink:true ancestor collapses to once a descendant is present (6 rather than 3 so the strip carries several consecutive lines of recent output and a still-producing strand is visibly moving — at 3 a TUI's trailing status and padding lines leave effectively one frozen line, which at a glance is indistinguishable from a dead strand; an already-materialized reed.yaml keeps whatever value it holds, since reconcile is key-based and never rewrites a value)`
  (3) In `internal/reedengine/template_windows.yaml`, replace the whole of line 5 with the byte-identical line from step (2) — same value, same comment text, no platform-specific wording.
  The two templates carry a byte-identical `collapsed_strip_rows` line today and must still do so afterwards.
  Do not change `min_full_rows`, `width`, `height`, `strand_name`, `debug_log`, `mouse`, `watchdog`, or the `header` block in either template.
  (4) In `internal/reedengine/doc.go`, in the "Silent layout rescale" entry of the `# Multiplexer contract surface` list (around lines 370-372), reword the measurement so its `3` reads as the then-default rather than the current one: the phrase `turned a 3-row collapsed strip into 1 row` becomes `turned a 3-row collapsed strip (3 was the then-default; it is 6 today) into 1 row`.
  Keep the tmux 3.6 measurement itself and the `"220x50"` / `"100x30"` figures verbatim — they record a real live observation and must not be restated as though taken at `6`.
  Re-wrap only the lines of that one bullet, preserving the file's existing `//     ` continuation prefix and its surrounding godoc wrap width;
  do not reflow any other bullet in the list, do not touch the `CollapsedStripRows` mention at line 373, and do not reword any other occurrence of this same measurement elsewhere in the package.
  (5) Run `go test ./internal/reedengine/...` again and confirm it is green.
  Do not edit `internal/reedengine/apply_test.go`, `internal/reedengine/lock_test.go`, `internal/reedengine/render/rules_test.go`, `internal/reedengine/render/pins_test.go`, or `internal/reedengine/render/height_test.go` — their `CollapsedStripRows: 2` values are deliberately-chosen unit inputs, not the template default.
  Do not edit `internal/reedengine/attachgeometry_integration_test.go` — it asserts against `e.cfg.CollapsedStripRows` rather than a literal, so it follows the template automatically;
  it is listed as Context only so its value-agnostic shape can be confirmed by reading, and it is exercised by the task's `pipeline.done_gate` tagged run, not by this batch's `verify:`.
  Do not add a spinner, badge, or any other synthetic liveness marker, do not add a strip floor to `clampToFit`, and do not add a value migration to `internal/configsync`.
- **Commit:** `fix(reedengine): default collapsed_strip_rows to 6 for readable liveness`

## Batch Tests

`verify: go test ./internal/reedengine/...` covers both packages this batch can affect: `internal/reedengine` (whose `config_test.go` holds the only two assertions on the template default's value — the explicit-template path and the degrade-to-embedded-template path, the latter being what proves both `template_posix.yaml` and `template_windows.yaml` moved, since it reads whichever template the build constraint selects) and `internal/reedengine/render` (unchanged by this batch, run to confirm nothing regressed;
every test there supplies its own `Params` and is independent of the template default by design).
This is a Go project, so no `PYTHONPATH= ` prefix applies.
No new tests are added: the only new fact is the default's value, which both existing assertions already pin, and a new `render` test would assert arithmetic `stackHeights` already has coverage for.
The untagged run stays tmux-free, satisfying the Test Tier Purity Invariant.
The tagged tier — `go test -tags integration ./...`, which runs `attachgeometry_integration_test.go` against a real multiplexer at a 100x30 client and again after a resize to 100x90 — is a landing gate for this task and is already covered by `pipeline.done_gate` in `mill-config.yaml`, so it is deliberately not duplicated into this batch's per-round `verify:` (it needs a live tmux and would run on every implementer and fixer round).
