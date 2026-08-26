# Batch: planvalidate-two-mode

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
batch: 'planvalidate-two-mode'
number: 3
cards: 7
verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...
depends-on: [1]
```

## Batch Scope

This batch turns the one `PlanValidate` engine into a two-mode engine and moves the `Plan-Revalidate` row into the approval-enforcing mode, which is the half of the deadlock fix that stops `Plan-Validate` demanding a flag only the review segment can produce.
It is one batch rather than several because `loomshed.NewPlanValidate` gains a parameter, and a Go signature change is atomic across the build: the production call site in `internal/shedrecipe/entries_simple.go` and every test construction in `internal/loomshed`, `internal/loomrecipe`, and `internal/loomcli` have to move with it or the tree does not compile.
The batch's cards therefore span four packages by necessity, and its `verify:` covers all four.

The `require_approved: true` recipe key on `Plan-Revalidate` lands here, in the same batch, deliberately: without it that row would drop to format-only mode the moment the signature change lands, and `internal/loomrecipe/revalidate_test.go` — which scripts a fixer round that clears the approval flag and asserts the row bounces — would fail with no code defect to point at.
The `approve_seam: plan` key on `Plan-Bouncer` is *not* here;
it lands in batch 6, once batch 4 has taught `shedrecipe` to resolve it.

Batch-local decision: the mode parameter is a plain `bool`, not a named mode type.
The producer has exactly two modes, no third is foreseeable, and the recipe key that supplies it is already a bool.

## Cards

### Card 7: Give the PlanValidate producer its two modes

- **Context:**
  - `internal/planparser/validate.go`
- **Edits:**
  - `internal/loomshed/planvalidate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a trailing `requireApproved bool` parameter to `NewPlanValidate`, giving the signature `NewPlanValidate(name, anchorPath, worktreeRoot string, requireApproved bool) shedengine.ShedProducer`, and store it as a `requireApproved bool` field on the unexported `planValidate` struct.
  In `Call`, keep `planparser.ParsePlan(planparser.PlanDir(p.anchorPath))` and its error disposition exactly as they are, then select between the two `planparser` entry points on `p.requireApproved`: true calls `planparser.Validate`, false calls `planparser.ValidateFormat`.
  Everything downstream of that selection is unchanged — the same non-empty-findings-to-`shedengine.Stuck` mapping, the same `formatPlanFindings` rendering, the same `logger.Warn` line, and the same `shedengine.Done` with the plan directory as the pointer.
  Do not add a second warn line or change the existing one: it already distinguishes the two rows by producer name.
  Update both doc comments in the file that currently pin the old single-mode contract by naming `planparser.Validate` specifically — the package doc at the top of the file and `Call`'s own doc — so each describes the two-mode wrap and says which mode each of the two rows sharing this engine runs in.
  Keep `Call`'s existing paragraph explaining why a `ParsePlan` error maps to a returned error rather than to `Stuck`, unchanged;
  it is mode-independent.
- **Commit:** `7: loomshed: give PlanValidate a requireApproved mode`

### Card 8: Read the require_approved config key in the PlanValidate registry entry

- **Context:**
  - `internal/shedrecipe/config.go`
  - `internal/loomshed/planvalidate.go`
- **Edits:**
  - `internal/shedrecipe/entries_simple.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `planValidateEntry`, read a new optional bool config key named `require_approved` via the package's existing `configBool(cfg, key, required)` helper with `required` false, so an absent key yields `false` — the mode `Plan-Validate` needs and every existing recipe row already implies.
  Add `"require_approved"` to this entry's `configRejectUnknown` call, which is currently invoked with an empty allowlist;
  without that addition the new key would be rejected as unrecognised the moment the recipe carries it.
  Pass the read value as `NewPlanValidate`'s new fourth argument.
  Order the calls so `configBool` runs before the `requireAbsRoot` checks, matching how the other entries in this file read their config keys first.
  Update the entry's doc comment to name the new key, state that absent means false, and say which of the two rows sharing this engine sets it.
- **Commit:** `8: shedrecipe: read require_approved on the PlanValidate row`

### Card 9: Set require_approved on the Plan-Revalidate recipe row

- **Context:**
  - `internal/shedrecipe/entries_simple.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `config:` block to the `Plan-Revalidate` row carrying `require_approved: true`, with a comment stating that this row runs after the review segment settles and is the row that confirms the approval flag is present, catching both a fixer-introduced regression and a failed approval write.
  Leave the `Plan-Validate` row with no `config:` block at all: its absent key is what puts it in format-only mode, which is the half of the deadlock this task removes, and the absence is load-bearing enough to say so in a comment on that row.
  Extend the existing comment on the `Plan-Revalidate` row that explains the shared-engine choice so it also names the mode difference as the second thing the recipe now distinguishes between the two rows, alongside their already-differing `on_done` targets.
  Change nothing else in the file — the `Plan-Bouncer` row's `approve_seam` key and the `Plan-Burler` row's instructions prose belong to a later batch.
- **Commit:** `9: loom-recipe: require_approved on Plan-Revalidate`

### Card 10: Move the loomshed PlanValidate tests to a mode table

- **Context:**
  - `internal/loomshed/planvalidate.go`
- **Edits:**
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/cancellation_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rework `TestPlanValidate_Call` into a table with a mode dimension: for each of the two `requireApproved` values, assert an unapproved-but-format-clean plan is `shedengine.Stuck` in the true mode and `shedengine.Done` in the false mode;
  a format-invalid plan is `Stuck` in both;
  a clean approved plan is `Done` in both;
  and a plan whose overview does not parse returns an error in both, never `Stuck`.
  Keep the file's local `seedPlanValidateFixture(t, anchorPath, approved bool)` helper and its existing fixture content as they are — the mode dimension is what is new, not the fixture.
  Update every `NewPlanValidate` construction in the file for the new fourth argument.
  In `cancellation_test.go`, update the single `NewPlanValidate` construction in the real-producers table for the new argument, passing `true` so the cancellation case keeps exercising the same full-check path it exercises today;
  change nothing else in that file, whose subject is cancellation rather than validation mode.
- **Commit:** `10: loomshed: table the PlanValidate modes`

### Card 11: Re-point the single-finding gate test off the approval flag

- **Context:**
  - `internal/loomshed/planvalidate.go`
  - `internal/planparser/validate.go`
- **Edits:**
  - `internal/loomshed/gatefindings_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestPlanValidate_StuckSurfacesItsFindings` currently builds a plan whose only validation failure is `approved: false` and asserts the producer's warn line carries `plan-unapproved`.
  Its subject is that exactly one finding reaches that log line — plumbing, not mode behaviour — so re-point the fixture rather than re-keying the test by mode: seed a format-clean, approved plan and additionally write one unindexed `.md` file into the plan directory that no Card Index entry names, so the plan's single finding is `index-file-mismatch`, and assert the warn line carries that ID instead.
  Keep the producer construction in the default mode by passing `false` as `NewPlanValidate`'s new fourth argument — the mode table in `planvalidate_test.go` is where `requireApproved` belongs, and this test must not become the one place it is covered.
  Leave `TestDiscussionValidate_StuckSurfacesItsFindings` and `TestLoomPreflight_StuckSurfacesItsFailures` in the same file untouched.
- **Commit:** `11: loomshed: re-point the plan gate-findings fixture to index-file-mismatch`

### Card 12: Cover the require_approved config key

- **Context:**
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/config.go`
- **Edits:**
  - `internal/shedrecipe/entries_simple_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add cases for the `PlanValidate` registry entry's new `require_approved` key, following the shape the file's existing `TestSimpleEntries_HappyPath` and `TestSimpleEntries_RejectsUnrecognisedConfigKey` cases already use: the key absent builds successfully;
  `require_approved: true` builds successfully;
  `require_approved: false` builds successfully;
  a non-bool value is an error naming the key;
  and an unrecognised key on this same entry — for instance a hyphenated `require-approved` typo — is still rejected, proving the allowlist widened by exactly one name rather than opening up.
- **Commit:** `12: shedrecipe: cover the require_approved key`

### Card 13: Mechanical NewPlanValidate call-site updates outside loomshed

- **Context:**
  - `internal/loomshed/planvalidate.go`
- **Edits:**
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomcli/parity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update every remaining `loomshed.NewPlanValidate` construction in the repo for the new fourth argument.
  In `shape_test.go` there are two, both inside `reflect.TypeOf` calls in the expected-producer table — one for the `Plan-Validate` row and one for the `Plan-Revalidate` row.
  Both exist only to name the concrete type the registry must return, so the argument value is immaterial there;
  pass `false` in both and change nothing else about the table.
  In `parity_test.go` there is one, in `TestGateParity_PlanValidate`'s per-case producer construction;
  pass `true` for now so this batch leaves the test's existing `Stuck_Unapproved` expectation correct — batch 5 is what re-keys the whole test by mode.
  Do not let the build enumeration stop at these three: grep the tree for the constructor name and fix any construction this card's list does not already name.
- **Commit:** `13: loomshed: update NewPlanValidate call sites for the mode argument`

## Batch Tests

`verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/loomrecipe/... ./internal/loomcli/...` names all four packages the signature change reaches, which is the batch's own edit surface plus the two packages whose tests only need mechanical call-site repair.

`internal/loomshed` covers cards 7, 10, and 11 directly.
`internal/shedrecipe` covers cards 8 and 12.
`internal/loomrecipe` is what proves card 9's recipe edit is coherent end to end: `revalidate_test.go` scripts a fixer round that clears the approval flag and asserts `Plan-Revalidate` bounces to `Plan-Write`, which passes only because that row is now in the approval-enforcing mode, and `sequence_test.go`'s nineteen-row order must stay unchanged.
`internal/loomcli` covers card 13's parity-test repair.

This is deliberately wider than a single package and deliberately narrower than the repo: the four named packages are exactly the ones the signature change touches, and the repo-wide sweep is already `pipeline.done_gate`'s job at the end of the run.
