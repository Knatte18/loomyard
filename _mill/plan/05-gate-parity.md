# Batch: gate-parity

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
batch: "gate-parity"
number: 5
cards: 3
verify: go test ./internal/loomshed/ ./internal/loomcli/ ./internal/webstercli/
depends-on: [4]
```

## Batch Scope

This batch moves the two plan gates from `planparser`'s entry points to `planglyph`'s, on both sides of each parity pair at once, and edits the `CONSTRAINTS.md` bullet that names them.
It is one batch because the Gate Self-Check Parity Invariant is precisely the rule that the producer row and its CLI verb call the same package function: moving one side without the other is the violation, so the two sides plus the invariant text are one indivisible change.
The external interface batch 6 and batch 7 consume is unchanged — no new row, no new verb, no new flag — which is itself the point: `Plan-Revalidate` ↔ `validate-plan --require-approved` stays the only pair, because the two webster boundaries already have verbs and have no rows.

Batch-local decisions, beyond `## Shared Decisions`:

- **Severity decides the verdict, not finding count.** Today's `if len(findings) > 0` gate predates a severity axis; with one, an informational-only set is a pass. This is the same rule card 32 applies to the identical `[]planglyph.Finding` type at `begin-batch`, and the two must not disagree about what a severity means.
- **The infrastructure error is a producer error, never a `Stuck`.** `planValidate.Call` already draws that line for a `ParsePlan` failure — a plan that will not parse is not a plan the `Plan-Write` bounce target can be asked to improve — and a quarry outage is the same class: `Stuck` persists blocked and bounces, a returned error persists failed and aborts the run.
- **`webstercli validate` moves too**, even though it is not half of a parity pair, because it runs the identical gate `websterengine.Run` runs before forking an implementer, and leaving it on `planparser` would make one of the three plan-checking surfaces answer a different question from the other two.

## Cards

### Card 23: move the Plan-Validate/Plan-Revalidate producer onto planglyph

- **Context:**
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/repo.go`
  - `internal/planparser/validate.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/planvalidate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `planValidate.Call` to run `planglyph.Validate` when `p.requireApproved` is set and `planglyph.ValidateFormat` when it is not, in place of the two `planparser` functions it calls today, keeping every other line of the producer's behaviour identical.
  The producer stays a thin wrap and nothing more: it parses with `planparser.ParsePlan(planparser.PlanDir(p.anchorPath))` exactly as now, and passes `p.worktreeRoot` through unchanged.
  The two path fields stay separate for the same reason they are separate today — `planparser.PlanDir` takes the anchor path while the validation entry points take the worktree root, and they are not the same value.
  Handle card 21's new second return: a non-nil error satisfying `errors.Is(err, planglyph.ErrQuarryUnavailable)` maps to a **returned error**, never to `shedengine.Stuck`, matching how this file already treats a `ParsePlan` failure and for the same reason — the two dispositions differ materially, since `Stuck` persists blocked and a returned error persists failed and aborts the run, and a gate that could not read the code has not found a plan defect to bounce.
  Adapt `formatPlanFindings` to take `[]planglyph.Finding`, keeping its semicolon-separated rendering so the `logger.Warn` line and the CLI envelope still describe a violation identically, and include each finding's severity in the rendered string so an informational `create-new-unit` is distinguishable from a blocking `glyph-not-found` in the one place that record exists.
  Map to `shedengine.Stuck` only when the findings slice carries **at least one** finding whose severity is blocking; a set that is empty, or that holds informational findings only, maps to `shedengine.Done` reporting the plan directory.
  Replace the existing `if len(findings) > 0` predicate accordingly — it predates a severity axis and, kept as-is, would bounce every plan that adds a brand-new package on card 18's informational `create-new-unit` finding, a condition `Plan-Write` cannot fix, so the row would resubmit an unchanged plan until the bounce budget escalated to a human.
  Emit the `logger.Warn` line on any non-empty set, blocking or not, so an informational finding stays visible even on the pass path; word it so the two cases are distinguishable rather than reusing the failure phrasing for both.
  This is the identical rule card 32 applies to the same `[]planglyph.Finding` type at `begin-batch`, and the two boundaries must not disagree about what a severity means.
  Update the file's package comment, which today names `planparser.ValidateFormat` and `planparser.Validate` for the two modes.
  Extend the existing tests with a case proving a quarry-unavailable error produces a returned error rather than `Stuck`; one proving an informational-only findings set produces `Done` and still logs; and one proving a set mixing one blocking and one informational finding produces `Stuck`.
- **Commit:** `23: refactor(loomshed): run the plan gate through planglyph's resolve-backed entry points`

### Card 24: move validate-plan and webster validate onto planglyph

- **Context:**
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/repo.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/loomcli/validate.go`
  - `internal/loomcli/validate_test.go`
  - `internal/webstercli/validate.go`
  - `internal/webstercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move both standalone plan-checking verbs onto the same two functions card 23 moved the producer onto, so each parity pair calls one function in each mode.
  In `internal/loomcli/validate.go`, change `validatePlanCmd`'s `RunE` to call `planglyph.Validate(plan, c.env.WorktreeRoot)` under `--require-approved` and `planglyph.ValidateFormat(plan, c.env.WorktreeRoot)` without it, leaving the `planparser.PlanDir(c.env.AnchorPath)` parse and the envelope-and-exit contract untouched.
  `renderFindings` is generic over any type with an `Error() string` method, so give `planglyph.Finding` such a method rendering `check[/card]: detail` exactly as `planparser.ValidationError.Error` does, plus its severity — then `renderFindings` needs no change at all and the two verbs keep rendering findings identically.
  Apply card 23's severity rule on this side too, since parity binds the two: emit the findings error envelope only when at least one blocking finding is present, and emit the success envelope otherwise — carrying any informational findings under their own envelope key so they stay visible on the pass path rather than being dropped.
  Map a quarry-unavailable error onto `output.Err` with a message naming quarry rather than the plan, so an operator reading the envelope is never told the plan is invalid when quarry simply could not answer; this is the CLI-side half of card 23's producer-side rule and the two must agree, since parity binds them.
  Update the command's `Long` text, which today names `planparser.ValidateFormat` and `planparser.Validate` for the two modes.
  In `internal/webstercli/validate.go`, make the same substitution for `planglyph.Validate(plan, c.geom.WorktreeRoot)`, adapt `findingsEnvelope` to take `[]planglyph.Finding` and add the severity to each entry's map alongside `check`, `card` and `detail`, apply the same blocking-only gate to which envelope it emits, and update the command's `Long` text, which today pins a 17-check count that is no longer accurate.
  Extend both packages' tests with a clean case; a blocking-findings case asserting the severity is present in the error envelope; an informational-only case asserting a success envelope that still carries the findings; and a quarry-unavailable case asserting the error envelope names quarry.
- **Commit:** `24: refactor(cli): run validate-plan and webster validate through planglyph`

### Card 25: update the parity invariant and its test

- **Context:**
  - `internal/loomshed/planvalidate.go`
  - `internal/loomcli/validate.go`
  - `internal/planglyph/planglyph.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/loomcli/parity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Bring the invariant text and the test that mirrors it onto the functions cards 23 and 24 moved to.
  In `CONSTRAINTS.md`, edit the `## Gate Self-Check Parity Invariant` section's first bullet, which today spells `planparser.ValidateFormat` for the `Plan-Validate` ↔ `validate-plan` pair and `planparser.Validate` for the `Plan-Revalidate` ↔ `validate-plan --require-approved` pair, so both name the `planglyph` equivalents instead.
  Leave the `Discussion-Validate` ↔ `validate-discussion` pair naming `discussionparser.Validate`, which this task does not touch, and leave the second bullet about adding a gate meaning adding its verb and its parity check unchanged, since this task adds no gate.
  In `internal/loomcli/parity_test.go`, `TestGateParity_PlanValidate` already drives both sides over four fixtures crossed with both modes and asserts the mapped verdicts agree in each cell; keep every existing cell's expectation, because the moved functions compose the same pure checks and must therefore produce the same verdicts.
  The `Unapproved` fixture's flag-absent cell stays `verdictDone` — that cell is the load-bearing one, proving the format-only mode never runs the `plan-unapproved` check — and its `--require-approved` cell stays `verdictStuck`.
  Add a fifth fixture whose plan carries a glyph that does not resolve, expecting `verdictStuck` in both modes, so the test covers the resolve-backed half the move introduces rather than only re-proving the pure half.
  Add a sixth fixture whose plan is clean apart from one informational `create-new-unit` finding, expecting `verdictDone` in both modes: that cell is what proves both sides read severity the same way, and it is the cell that would have caught the bounce loop had it existed before the move.
  Add a seventh case pointing both sides at a worktree root that is not a repository, expecting `verdictError` in both modes, which is what proves the infrastructure-error disposition is symmetric across the pair and not just implemented twice.
  Update the test's own doc comment, which names the two `planparser` functions.
- **Commit:** `25: docs(constraints): move the plan parity pairs onto planglyph and cover the resolve half`

## Batch Tests

`verify: go test ./internal/loomshed/ ./internal/loomcli/ ./internal/webstercli/` covers exactly the three packages this batch edits, and the parity test in `internal/loomcli` is the load-bearing one: it drives the producer and the verb over the same fixtures in both modes and fails if either side moved without the other.
All three packages' untagged tests stay tier1-pure — the two new parity fixtures build a small repository under `t.TempDir()` and reach `quarry.Resolve`, which reads files rather than spawning, and the non-repository fixture never opens anything at all.
`internal/webstercli`'s own `cli_integration_test.go` and `smoke_test.go` carry build tags and are out of this untagged run by construction; they exercise no plan-validation path this batch changes.
The scope excludes `internal/planglyph` deliberately — batch 4's verify already covers it and nothing here changes it — while the overview's module-wide `go build ./...` catches any other caller of the two moved `planparser` functions that this batch missed, at this batch's own boundary.
</content>
