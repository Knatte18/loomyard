# Batch: status-interrupt-policy

```yaml
task: "lyx loom step + external supervisor skill"
batch: "status-interrupt-policy"
number: 5
cards: 2
verify: go test ./internal/loomcli/... ./internal/lyxcwd/...
depends-on: [4]
```

## Batch Scope

This batch adds one additive key — `interrupt_policy` — to `lyx loom status`'s envelope, reporting the policy for its own `current_producer`, and documents it.
It is one batch because the key and its guard against a silent rename are inseparable, and it is separate from batch 4 because it changes a different verb with a different wiring tier.
It is what closes the supervisor skill's first-step case: a first step that is interrupted leaves no prior envelope to read a policy from, so `status` must carry it.

Batch-local decision: `status` keeps its lightweight wiring (`verbUsesLightweightWiring` returns true for it). The lookup is a map read keyed by a name the verb already has, needing no config and no engine, so nothing about this key justifies moving `status` onto the full `wire()` path — and doing so would reintroduce the exact defect `wireLightweight` exists to prevent, where a fault in an unrelated module's config takes away the operator's read-out and the documented emergency brake.

The `depends-on: [4]` edge is for doc serialisation, not code: this batch's only genuine code dependency is batch 2's table. See `## Shared Decisions`' loom-md-edits-are-serialised-through-the-batch-chain in the overview.

## Cards

### Card 14: add interrupt_policy to the status envelope

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/loomengine/status.go`
  - `internal/shedengine/status.go`
- **Edits:**
  - `internal/loomcli/status.go`
  - `internal/loomcli/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `statusCmd`'s `RunE`, add exactly one key to the `output.Ok` map: `"interrupt_policy": loomshed.InterruptPolicyFor(st.CurrentProducer)`. Change no existing key's name, value, or type. The eight existing keys — `current_producer`, `state`, `error`, `pause_requested`, `activity`, `history_length`, `slug`, `parent` — stay exactly as they are.

  Add the `internal/loomshed` import. Comment at the call site that the policy comes from the same `internal/loomshed` table `step`'s `next_interrupt_policy` reads, that it is what lets the supervisor skill branch on an interrupted **first** step where no prior `step` envelope exists, and that the lookup is a map read keyed by a name this verb already has — so `status` keeps its lightweight wiring and needs no config and no engine for it. Note that the value is the empty string when `current_producer` names no row, which is the caller's signal to omit rather than a third policy value.

  Update `statusCmd`'s `Long` prose, which today enumerates what the one-shot envelope carries ("the current producer, state, error text, pause flag, composed activity, history length, and the task's slug/parent"), to include the interrupt policy.

  In `internal/loomcli/status_test.go`, add a test asserting the envelope's exact key set. Drive `statusCmd()`'s `RunE` in-process against a hand-populated receiver whose `shedPaths` point at a seeded status file under `t.TempDir()`, capture the emitted JSON, and assert the decoded top-level key set equals exactly the nine payload keys plus `ok`. Assert both directions, so the addition cannot quietly become a rename of an existing key and a later removal fails here too. Assert `interrupt_policy` reads `"handback"` for a status file whose `current_producer` is `loomshed.NameWebster`, `"reinvoke"` for at least one other row, and the empty string for a `current_producer` naming no row. Leave the existing `TestRenderStatusLine` and `printStatusLinesOnChange` tables untouched — neither renders this key.
- **Commit:** `feat(loomcli): report interrupt_policy on the status envelope`

### Card 15: document the additive status key

- **Context:**
  - `internal/loomcli/status.go`
  - `internal/loomshed/interruptpolicy.go`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `manifest/designs/loom.md` describes the status **file**'s fields but carries no description of the `lyx loom status` **envelope**'s key set, so there is no key list to append to. The edit site is the Module decomposition table's existing `` `lyx loom status` `` row — the same `| Piece | Form | Notes |` table batch 4 card 13 added a `` `lyx loom step` `` row to. Extend that row's Notes cell to record that the one-shot envelope also carries `interrupt_policy` for its own `current_producer`, drawn from `internal/loomshed`'s table, empty when `current_producer` names no row. Keep the cell on one line, per the repo's table convention, and keep the row's existing strand description intact. State in the cell that the key is additive — no existing key changes — and that `status` keeps its lightweight wiring.

  Do not touch the `/ly-*` skills table row: batch 6 owns that edit.

  Write in semantic line breaks per `CLAUDE.md`: one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, using plain newlines. Every inline link added must resolve, file part and `#anchor` alike — `internal/lyxcwd`'s `docslink_test.go` enforces it and is in this batch's verify scope.
- **Commit:** `docs(loom): document the additive interrupt_policy status key`

## Batch Tests

`verify: go test ./internal/loomcli/... ./internal/lyxcwd/...` runs the two packages this batch touches. `internal/loomcli` carries the edited `status_test.go` with the new exact-key-set assertion; `internal/lyxcwd` carries `docslink_test.go`, the Markdown Link Integrity guard covering card 15's doc edit.

Running the whole `loomcli` package rather than named test functions is correct here because card 14 edits a shipped verb body, and the package's other suites — `cli_test.go`'s command-tree walk in particular — must stay green against it.

The exact-key-set test is the load-bearing one: it asserts both directions, so neither a silent rename of an existing key nor an untested new key can ship. The tagged `smoke` and `integration` suites are not in scope and are not edited; `pipeline.done_gate` covers the integration tier before the task is marked done.
