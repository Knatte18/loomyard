# Batch: registration-docs

```yaml
task: "Build builder - the batch-implementation loop"
batch: "registration-docs"
number: 8
cards: 3
verify: go test ./cmd/lyx/... ./internal/buildercli/...
depends-on: [7]
```

## Batch Scope

Make the module visible and documented: `cmd/lyx` registration with every pinned-set
test updated, the sandbox scenario satisfying the Sandbox Suite Coverage invariant, and
the Documentation Lifecycle obligations — the new module doc, the overview table row,
loom.md's holistic-review correction, and the roadmap milestone flip.

## Cards

### Card 30: cmd/lyx registration and pinned sets

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Wire `buildercli` into `newRoot()` per the CLI/Cobra Invariant:
  import, `root.AddCommand(buildercli.Command())`, append `builder` to the root
  `Long` module list. `helptree_test.go` is the one guard with a hardcoded pin —
  extend it with builder's six subcommands; `registration_test.go` (AST-based) and
  `longlist_test.go` (derived from `newRoot().Commands()`) are dynamic and need no
  edit — they simply must pass. Run the full `go test ./cmd/lyx/...`
  and fix any drift the guards name — including `drift_test.go`'s Short check and
  `sandbox_coverage_test.go`, which will fail until card 31 lands (cards 30 and 31
  are one commit-pair within the batch; order the commits so the suite is green at
  the batch boundary, not necessarily per card — note this explicitly: card 30's
  commit may leave sandbox coverage red until card 31's commit follows immediately).
- **Commit:** `feat(builder): register builder module in cmd/lyx`

### Card 31: sandbox scenario

- **Context:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one scenario in the existing suite style (bold-label
  `**Goal:**`/`**Watch:**`/`**Verdict:**` lines) with `**Covers:** builder`: in an
  initialized sandbox worktree, copy the `plan-valid` fixture under the worktree's
  plan dir, run `lyx builder validate` (expect the valid envelope), `lyx builder
  status` (expect `initialized: false`), then break the fixture (flip `approved:` to
  false) and re-run validate expecting the findings envelope. No agent spawns — the
  real end-to-end orchestrator run is deliberately NOT a CI/sandbox scenario
  (discussion: slow, subscription-burning, flaky); note that exclusion inline in the
  scenario's prose.
- **Commit:** `test(builder): sandbox scenario covering the builder module`

### Card 32: module doc, overview, loom correction, roadmap

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/README.md`
  - `docs/modules/plan-format.md`
- **Edits:**
  - `docs/overview.md`
  - `docs/modules/loom.md`
  - `docs/roadmap.md`
  - `CONSTRAINTS.md`
- **Creates:**
  - `docs/modules/builder.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `docs/modules/builder.md`: the module doc per the Documentation
  Lifecycle — the as-built design: verb surface (all six), the digest contract as the
  pinned field table (batch, status, tests, stuck_reason, out_of_scope,
  drift_unreported, files_changed, dirty, dead_reason, elapsed_s), poll's four-branch
  terminal classification, role selection + recovery ladder, chain rollback, pause,
  outcome contract + archiving, fingerprint + `--fresh`, run.lock, the weft commit
  boundaries, and the co-versioning rule (templates ↔ digest/outcome parsers change
  together, same commit). `docs/overview.md`: add builder's row to the module table
  (LLM orchestrator + Go verbs; input contract plan-format.md). `docs/modules/loom.md`:
  correct the Builder section sentence "…and runs a holistic builder-review at the
  end…" to state the holistic review is the Builder-review GATE — perch's job, driven
  by loom (or the operator) after `builder run` returns done; builder ends at
  batches-built. Update loom.md's module-decomposition table row for builder to name
  the six as-built verbs. `docs/roadmap.md`: mark the builder milestone ✅ Done linking
  `docs/modules/builder.md` (milestone completion only — no fix-notes). `CONSTRAINTS.md`:
  no new invariant is expected from this task (geometry, CLI, weft, provider-seam, and
  sandbox invariants all pre-exist and were followed); make no edit unless the
  implementation genuinely introduced a new cross-cutting invariant, in which case
  record it minimally.
- **Commit:** `docs(builder): module doc, overview row, loom correction, roadmap`

## Batch Tests

`verify:` runs `cmd/lyx` (drift, helptree, registration, longlist, sandbox-coverage
guards — the machine half of the invariants this batch satisfies) plus buildercli as
the regression guard.
