# Batch: cross-product-driver

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'cross-product-driver'
number: 7
cards: 2
verify: go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/
depends-on: [6]
```

## Batch Scope

This batch delivers the cross-product driver — the one uniform loop over states, verbs and anchors that is the actual deliverable of the task — and the first full run of it.
It is one batch because the driver and the run are inseparable: the driver's correctness is only observable by running all 168 cells, and the wall-clock number the run produces is a card of its own only because it must be recorded rather than asserted.

Batch-local decision: the driver contains **no** per-verb or per-state special cases.
Every restriction is expressed as data on the case (`States`) or as a row in the omission table.
A conditional in the driver would be the point at which the cross-product property quietly stops holding for the next verb someone appends.

## Cards

### Card 18: the cross-product driver

- **Context:**
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/fabrictest/states.go`
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fabricengine/fabrictest/refusal.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/matrix_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `//go:build integration`, `package fabrictest`.
  One test function driving `states × verbs × anchors` as nested `t.Run(anchor + "/" + state.Name + "/" + verb.Name)` subtests, each calling `t.Parallel()` on its **own** hub built by `NewHub`.
  Every cell already owns its hub for correctness — independent bare pairs so pushes never race — so `t.Parallel()` is free.
  No hand-rolled worker cap: Go's `-parallel` flag already bounds concurrency, and a cap here would duplicate the toolchain.
  The loop skips a `(state, verb)` pair when the verb's `States` restriction excludes it, or when the pair appears in the omission table;
  a skipped cell calls `t.Skip` with the omission table's recorded reason, so a green run states out loud what it did not run rather than silently omitting it.
  Each cell runs the five phases in the fixed order — build, arrange, state, capture-before, run-then-capture-after — and then asserts against its `Expectation`:
  `KindRefusedByGate` asserts `RefusedByGate(err, check)` **and** that the error does not match the other two checks;
  `KindRefusedBefore` asserts `RefusedBefore(err, substring)`, which by construction also asserts the error does not carry `"check failed"`;
  `KindProceeds` asserts `err == nil`, runs the cell's `Effect`, and asserts the state's planted content still survives.
  Every kind then runs `AssertNoUnpermittedChange` against the cell's permitted roots.
  Clean-state cells run their `Effect` under every kind, since that is where the tautology guard lives.
  Add the `CloneHub{Reset: true}` column as a separate, explicitly-named test function rather than a row in the loop, because it is scoped to the ownership axis and not to the ten states — folding it into the loop would either reintroduce the vacuous dirtiness rows or require a per-verb conditional in the driver, and the plan rejects both.
  Add a **count assertion**: the driver tallies the cells it ran and the cells it skipped, and asserts the total equals the enumeration this plan owns — 168 cells, of which 130 are the ordinary product after the recorded structural omissions, 34 are hostile-input cases and 4 are the `Reset` column, minus any dirtiness omission batch 6 recorded.
  Assert the tally against a constant derived from the tables in code, not against a hardcoded literal, so appending a verb updates the expected count automatically instead of failing;
  what the assertion pins is that the driver ran every cell the tables describe and skipped only what the omission table names.
  A silent cap or a swallowed skip is the one failure mode a green matrix cannot reveal on its own.
- **Commit:** `fabrictest: drive the state x verb x anchor cross product`

### Card 19: full-matrix run and recorded wall-clock

- **Context:**
  - `internal/fabricengine/fabrictest/matrix_test.go`
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/fabrictest/states.go`
  - `docs/benchmarks/running-tests.md`
- **Edits:**
  - `internal/fabricengine/fabrictest/doc.go`
  - `internal/fabricengine/fabrictest/verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the full matrix (`go test -tags integration ./internal/fabricengine/fabrictest/`) to completion and fix every failing cell.
  A cell failing here is one of three things and each is resolved differently, so classify before fixing:
  a harness bug (fix the harness);
  a mis-derived expectation (re-derive it from the scope table in `verbs.go`, never from what the run happened to produce — a cell rewritten to match observed behaviour asserts nothing and is the exact failure this whole task exists to correct);
  or a genuine defect in `internal/fabricengine` (this is instance nine — do **not** fix production code in this task, which is scoped as additive with no production behaviour change beyond `CloneAndWire`;
  record it in `doc.go` as a discovered defect with the cell that found it and leave the cell failing only if it cannot be honestly expressed otherwise, surfacing it to the operator instead of quietly weakening the assertion).
  Then measure the wall-clock of a full run and record the number in `doc.go`'s measured-wall-clock section, together with the machine and core count it was measured on and the `-parallel` value in effect.
  Recording the number makes a future regression visible to a reader;
  **do not** add a timing assertion of any kind — a failing wall-clock guard fails on a loaded CI box rather than on a real regression, which is why the repo rejects timing assertions elsewhere.
  For reference, the discussion's measured baseline is a 24 ms concurrent full fixture (about 2 ms bare copy plus about 22 ms clone), with clone scaling 5.2x on 14 cores because it is `fork`/`fsync`-bound.
- **Commit:** `fabrictest: record the measured full-matrix wall-clock`

## Batch Tests

`verify:` is `go build ./... && go test -tags integration ./internal/fabricengine/fabrictest/`.
This is the one batch where the unbounded package suite is the right scope rather than a shortcut: the deliverable **is** the full cross product, so a narrower `-run` filter would verify something other than what the batch ships.
The suite is still bounded to one package — it does not reach for the whole-tree `go test ./...`, which is the configured `pipeline.done_gate` and runs once at the end.
No enforcement guard is re-run here because this batch adds one integration-tagged `_test.go` file and edits only `doc.go`'s prose, neither of which can move a guard that batches 2-6 already passed;
the whole-tree done gate is the backstop if that judgement is wrong.
