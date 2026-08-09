# Batch: spec-repair-acceptance

```yaml
task: 'builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference'
batch: spec-repair-acceptance
number: 5
cards: 2
verify: go build ./... && go test ./... && go test -tags integration ./...
depends-on: [4]
```

## Batch Scope

Two closing jobs.
Card 17 repairs `manifest/designs/shed-followups.md`, which is a **live specification five other tasks execute against**, not documentation: this task overrode four of its ownership assignments and invalidated two of its instructions, and leaving that unrecorded means the next task in the chain works from a stale spec.
Card 18 is the acceptance gate — the repo-wide zero-hit grep that turns "did we sweep everything" from an opinion into a test, plus the three build/test commands the discussion pins as the verification bar.

It is the last batch because both cards depend on every preceding one: the overrides can only be described accurately once they are made, and the grep is only meaningful against the finished tree.

## Cards

### Card 17: Record this task's overrides in shed-followups.md

- **Context:**
  - `docs/reference/status-schema.md`
  - `docs/reference/webster-contract.md`
  - `internal/loomengine/coherence.go`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/shed-followups.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Record four overrides of this spec by task A, each at the site it overrides, so no downstream task works from a stale ownership claim.
  1. **The phase enum.** The "What this task does not own" section defers `internal/loomengine/coherence.go`'s `validPhases` map and its `docs/reference/status-schema.md` twin to the `Shed` build task, on the grounds that editing them now means inventing an interim phase set `Shed` would discard.
     Task A renamed both `builder` → `webster` instead.
     Record that the deferral no longer holds and why: an enum naming a deleted module is not a neutral interim state, and `Shed` replacing the phase enum with a flat producer list later is not a reason to leave a wrong value in live validation code now.
  2. **`builder-contract.md`.** The spec treats it as retired-as-a-reference;
     task A deleted it outright and created `docs/reference/webster-contract.md` as webster's own consumer-facing contract.
     Record the new file, and that the deleted file's chain-rollback and recovery-ladder material is recoverable only from git history.
  3. **The roadmap phase slot.** The spec assigns `manifest/roadmap.md`'s "deferred phase slot between Builder and Finalize" to task E as its remaining roadmap obligation.
     Task A's phase rename necessarily touched the word `Builder` on that line.
     Record the split explicitly: A renamed the phase **word**, E's remaining obligation on that line is the slot **semantics** — so E does not find the line already changed and conclude its obligation lapsed.
  4. **`loom.md`'s naming note.** The spec assigns `manifest/designs/loom.md`'s naming-note lines and the module-decomposition table row to task E.
     Task A rewrote them in full, because they literally name the two deleted builder packages and task A's package-name zero-hit criterion reaches them regardless of ownership.
     Record that everything else in `loom.md` remains E's and the B → C → E chain-order owner list is otherwise unchanged.
  Then repair two instructions this task invalidated:
  the task-B file inventory lists the deleted builder-contract doc's path, which no longer exists — remove it, and add `docs/reference/webster-contract.md` if that file is in B's sweep scope;
  and task C's step 5 grounds itself in "`builder-contract.md`'s digest contract" — re-ground it in a live source, or state that the grounding document is gone and what replaces it.
  Also update the v2-coexistence-prose ownership list so it no longer names sites inside deleted files.
  Do **not** record an override for the `plan-format.md` dangling window: this task adopted the spec's own A→B window as written rather than diverging from it.
  This file is a named exclusion in card 18's acceptance grep, so it may keep the word `builder` where it is describing history or another task's scope.
- **Commit:** `docs(shed-followups): record task A's four overrides and repair the B/C instructions`

### Card 18: Acceptance gate — the repo-wide zero-hit sweep

- **Context:**
  - `CONSTRAINTS.md`
  - `crucible/README.md`
  - `crucible/review-prompt-template.md`
  - `docs/benchmarks/fixture-copy.md`
  - `docs/benchmarks/scout-vs-grep.md`
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/reference/plan-format-v3.md`
  - `docs/reference/webster-contract.md`
  - `docs/research/scout-agent-usage-findings.md`
  - `docs/research/scout-multilang.md`
  - `docs/research/scout-spike.md`
  - `internal/fabricengine/refscanner_test.go`
  - `internal/webstercli/sync_integration_test.go`
  - `internal/websterengine/audit_test.go`
  - `manifest/designs/fabric-unified-view.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/shed-followups.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run the acceptance sweep and confirm it is clean.
  Write every exclusion into the grep invocation explicitly rather than filtering by eye, so the check is auditable and a reviewer sees exactly what was deliberately left behind.
  Six patterns, all of which must return zero hits across the whole repo — `plugins/`, `tools/` and `sandbox/` included — minus the git metadata directory, this task's own `_mill/` directory, `manifest/designs/shed-followups.md`, and the dated historical records (`docs/benchmarks/test-suite-timing.md`, `docs/benchmarks/fixture-copy.md`, `docs/benchmarks/scout-vs-grep.md`, `docs/research/scout-agent-usage-findings.md`, `docs/research/scout-spike.md`, `docs/research/scout-multilang.md`, `crucible/README.md`, `crucible/review-prompt-template.md`), which are timestamped records of what was measured and are falsified by editing:
  1. **Package names** — `builderengine`, `buildercli`.
  2. **Phase and gate token** — `"builder"`, `phase: builder`, `builder-review`, and the `→ Builder →` phase-list form.
  3. **Module word** — `lyx builder`, `builder.yaml`, `builder-suite`, `builder suite`, `SANDBOX-BUILDER-SUITE`, `_lyx/builder`, `.lyx/builder`.
  4. **Deleted filename** — `builder-contract` in any form.
     `plan-format.md` is deliberately **not** in this pattern: its links dangle by design for the window between this task and the next, so a zero-hit criterion on it would fail on a state this task intends to produce.
  5. **Commit subject and path fragment** — the `builder:` commit-subject prefix and the `/builder/` path fragment.
  6. **Bare word** — a case-insensitive, word-boundary `builder` scan, minus the enumerated exclusion tokens below.
     This pattern is load-bearing, not belt-and-braces: the five qualified patterns cannot see the provenance doc-comments that write plain "builder".
  The only permitted bare-word exclusions are these enumerated tokens: `strings.Builder` (stdlib type);
  `master-builder` / `master-builder-weft` (arbitrary worktree-name fixtures in `internal/fabricengine/refscanner_test.go` and `internal/websterengine/audit_test.go`, unrelated to the builder module — leave both files untouched);
  "content builder";
  "fixture builder" and "fixture builders";
  "the same builder produces";
  "a builder that died";
  "`xCmd()` builder";
  "fluent builder method";
  "Hub builder:";
  and the task slug `builder-retire` where a document names this task by name.
  The `builder-retire` token is an addition to the discussion's derived list and was known at plan time, not discovered by the scan;
  it is recorded here rather than applied silently.
  The discussion's `"internal/builder"` synthetic path-prefix fixture exclusion no longer applies — batch 1 renamed that fixture to `internal/webster`.
  If the scan turns up any other ordinary-English or unrelated-fixture site, stop and report it as a finding rather than adding a token to this list — the list's provenance is the point.
  Then confirm three checks recorded during planning and left deliberately untouched, and report if any is no longer true:
  `manifest/designs/fabric-unified-view.md` is a record of a completed migration — no `BuilderDir` symbol exists in the tree today — so it is left alone;
  `manifest/designs/loom.md`'s Plan-producer paragraph about the target format changing is not this task's in any respect, and its only builder-era link targets `plan-format.md`;
  and `docs/reference/plan-format-v3.md`'s remaining `plan-format.md` link is left dangling on purpose.
  Finally run the three build/test commands from `verify:` and confirm all three are clean — the integration-tagged run is required, not optional, because `internal/webstercli/sync_integration_test.go` is invisible to an untagged `go test ./...`.
  This card writes nothing.
  If the acceptance scan or any of the three commands is not clean, that is a plan defect to report, not a site to patch here.
- **Commit:** none

## Batch Tests

`verify:` is the discussion's full verification bar, run here rather than per-batch because this is where the whole task is accepted: `go build ./...`, the untagged `go test ./...`, and `go test -tags integration ./...`.
The integration-tagged run is required — `internal/webstercli/sync_integration_test.go` is the one real cross-package compile blocker and an untagged run never compiles it.
The same two suites also run at the repo-wide done gate configured in `mill-config.yaml`, which is the backstop for any package outside a batch's own verify scope.
Card 18's grep sweep is the completeness criterion the test suite cannot express;
it is run by hand as part of that card and its exclusions are written into the invocation.
No new tests are written anywhere in this task — the existing suite is the test, and four guards (`cmd/lyx/helptree_test.go`, `cmd/lyx/notransients_test.go`, `internal/configreg/configreg_test.go`, `cmd/lyx/sandbox_coverage_test.go`) fail loudly on a half-removal.
