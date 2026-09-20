# Batch: loomshed-gates

```yaml
task: 'Producer gates: mechanical gates before session release'
batch: 'loomshed-gates'
number: 3
cards: 5
verify: go test ./internal/loomshed/...
depends-on: [1]
```

## Batch Scope

This batch builds the two gate closures and teaches the two writer rows' commit decorators the new pointer rule.
`internal/loomshed` is the right home for the closures because it is already the only package importing both validators, and it already owns everything they need: `formatDiscussionFindings`, `formatPlanFindings`, `hasBlockingFinding`, and the `logger.Warn` lines `gatefindings_test.go` pins.
The alternative placement, building the closures inline in `internal/shedrecipe`, costs two import-allowlist entries against this placement's one, and would strand that warn-line guard in a package whose producers no longer exist.

Nothing is deleted here: `discussionvalidate.go` and `planvalidate.go` are still wired into the recipe and stay untouched, so the shared helpers the closures call keep living in them until batch 5 moves them.
The external interface batch 4 consumes is `NewDiscussionGate` and `NewPlanGate`.

Batch-local decision: the closures are built here but the `logger.Warn` lines are duplicated rather than shared with the producers that still exist alongside them for two more batches.
Each site's warn line names its own producer or gate, and batch 5 removes the producer half with its file, so a shared helper would exist for exactly two batches and then have one caller.

## Cards

### Card 17: The discussion gate closure

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/loomshed/discussionvalidate.go`
  - `internal/discussionparser/validate.go`
  - `internal/loomshed/doc.go`
- **Edits:**
  - `internal/loomshed/seam_enforcement_test.go`
- **Creates:**
  - `internal/loomshed/gates.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func NewDiscussionGate(decisionRecordPath, supportLogPath string) shuttleengine.Gate` to the new file, returning a closure that calls `discussionparser.Validate(decisionRecordPath, supportLogPath)` once.
  A non-nil error is returned verbatim alongside the zero `shuttleengine.GateResult`: a gate that could not read the artifact has found no defect to re-prompt over, it is an infrastructure fault, and it must never burn an attempt or reach the LLM.
  A non-empty findings slice produces `GateResult{Passed: false, Findings: formatDiscussionFindings(findings)}` and a nil error, preceded by a `logger.Warn` carrying the gate name, the decision record path, and the same formatted findings — that warn line is the only durable record of why an artifact was refused, because the findings file itself lives in the ephemeral run directory `finalize` deletes on the Done cleanup.
  An empty slice produces `GateResult{Passed: true}` and a nil error.
  Both paths are told rather than derived, because `loomengine`'s own accessors for them take a `*lyxcwd.Location` this package may not import.
  Add the file's own doc comment stating that both closure constructors here are the gate half of the Gate Self-Check Parity Invariant and call the identical package function their CLI self-check verb does.
  In the same card, add `github.com/Knatte18/loomyard/internal/shuttleengine` to `loomshedAllowedImports`, carrying its reasoning as a comment in the style the `planglyph` and `logger` entries already set: it is imported for two type names and no behaviour, and `internal/shedadapters`, already on that list, imports it transitively anyway, so the geometry footprint does not widen.
  The allowlist edit belongs in this card rather than a later one because the new import trips `TestToldGeometryInvariant_AllowlistOnly` the moment the file lands.
- **Commit:** `feat(loomshed): add NewDiscussionGate, the discussion rows' gate closure`

### Card 18: The plan gate closure and its ParsePlan carve-out

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/loomshed/planvalidate.go`
  - `internal/planparser/parse.go`
  - `internal/planglyph/planglyph.go`
  - `internal/planglyph/repo.go`
- **Edits:**
  - `internal/loomshed/gates.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func NewPlanGate(anchorPath, worktreeRoot string) shuttleengine.Gate`, returning a closure that parses through `planparser.ParsePlan(planparser.PlanDir(anchorPath))` and then runs `planglyph.ValidateFormat(plan, worktreeRoot)`.
  The two path parameters are separate because `planparser.PlanDir` takes the anchor path while `planglyph.ValidateFormat` takes the worktree root, and they are not the same value.
  `ValidateFormat`, never `planglyph.Validate`: both plan gate sites run strictly before the Plan-Review segment's approve seam writes the approval flag, so demanding it would fail every single fix round.
  The `ParsePlan` error set is split by shape rather than treated whole.
  An error satisfying `errors.As(err, new(*fs.PathError))` is a returned error: `ParsePlan` has exactly two `%w`-wrapped `os.ReadFile` faults, the overview read and the per-card read reached through `parseCardFile`, and this test matches both and only those, because every other error it returns is a plain `fmt.Errorf` value.
  Every other `ParsePlan` error produces `GateResult{Passed: false, Findings: err.Error()}` and a nil error: the not-exist branch and every structural or format error are plain `fmt.Errorf` values by construction, and they describe the bytes the agent wrote, which is the single most LLM-fixable defect class there is.
  This is a reasoned reversal of the disposition the standing plan-validate producer takes, and the closure's doc comment must record why rather than leaving it looking like an oversight: that producer's rationale — a plan that will not parse is not a plan the bounce target can be asked to improve — was about a *cold respawn* that knows nothing of the complaint, whereas the gate's bounce target is the live session that just wrote the file, holding its full context, so the premise no longer holds.
  Record in the same comment that this makes both gates behave identically on a missing-or-malformed artifact, since `discussionparser.Validate` already reports a missing file as a finding and only a non-not-exist read failure as an error.
  Every `planglyph` error stays a returned error in full and is explicitly **not** part of the carve-out: a resolve or quarry failure means the gate could not read the code, not that it found a defect, and three absurd re-prompts over a missing quarry binary would hide the real fault.
  A findings slice carrying no entry that fails `hasBlockingFinding` is a **pass**: it is logged as a `logger.Warn` naming it informational and returns `GateResult{Passed: true}`, because a create-new-unit finding on a brand-new package is not something the writer can fix and re-prompting on it would burn the whole budget on a condition that was never wrong.
  A slice with at least one blocking entry produces `GateResult{Passed: false, Findings: formatPlanFindings(findings)}` after a `logger.Warn` carrying the same formatted text.
  `hasBlockingFinding` is called, never re-derived: it encodes a crucible-round finding that `planglyph.Severity` is an open string type, so testing not-informational rather than equals-blocking is what keeps an unrecognized or zero-valued severity from silently passing.
- **Commit:** `feat(loomshed): add NewPlanGate with the ParsePlan findings carve-out`

### Card 19: The writer rows' commit decorators commit on a non-empty pointer

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedadapters/singlellm.go`
- **Edits:**
  - `internal/loomshed/discussionwrite.go`
  - `internal/loomshed/planwrite.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In both `discussionWrite.Call` and `planWrite.Call`, replace the early return condition `err != nil || outcome != shedengine.Done` with `err != nil || pointer.Path == ""`, so the commit seam fires whenever the wrapped producer reports a non-empty output pointer with a nil error, and the returned triple stays `(outcome, pointer, nil)` rather than being forced to `Done`.
  Left alone, a gate-failed `Stuck` would skip the commit and halt the run with the invalid artifact sitting uncommitted in a dirty weft — exactly the state the decorators' own recorded rationale exists to prevent.
  That rationale is preserved rather than superseded, and each doc comment must say so in those terms: the commit keeps the working tree clean and the artifact durable, it does not certify it, so the human the run just halted for finds the artifact committed and diagnosable.
  Record in the same comments that `shuttleengine.OutcomeAsking` keeps returning `Stuck` with an empty pointer and is therefore still not committed, correctly, because an asking run has not satisfied its file contract and there is nothing to commit; and that the only two outcomes the wrapped `SingleLLMProducer` can report with a non-empty pointer are `Done` and a gate-failed `Stuck`.
  The nil-commit-seam guard and the commit-error-to-returned-error mapping are unchanged on both decorators, and a commit failure on the gate-failed path maps to a returned error exactly as it does on the `Done` path.
- **Commit:** `feat(loomshed): commit a writer row's artifact whenever its pointer is non-empty`

### Card 20: Gate closure tests

- **Context:**
  - `internal/shuttleengine/gate.go`
  - `internal/loomshed/gates.go`
  - `internal/loomshed/gatefindings_test.go`
  - `internal/loomshed/planvalidate_test.go`
  - `internal/loomshed/discussionvalidate_test.go`
  - `internal/planparser/parse.go`
  - `internal/planglyph/repo.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/gates_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Cover `NewDiscussionGate` with a pass case, a findings case, and an error case proving a validator error is returned rather than reported as a failed gate — a decision record that is a directory is the error fixture the existing validate tests already use.
  Cover that a failing discussion gate's `logger.Warn` carries the formatted findings, reusing the output-capture pattern `gatefindings_test.go` already establishes; this is the property that makes the warn line the only durable record of a refusal, and it must be asserted against the closure, not only against the producer that is about to be deleted.
  Cover `NewPlanGate` with the same three shapes, and additionally with the `ParsePlan` split, which is the subtlest rule in the task: a malformed overview, an unparseable card index, and an absent `00-overview.md` each produce `Passed` false with the error's own text as findings, while **both** of `ParsePlan`'s read faults produce a returned error — the overview read and the per-card read reached through `parseCardFile` — since a test covering only the first would leave the card-file path unguarded.
  Cover that a `planglyph` resolve failure stays a returned error in every case, using a worktree root that is not a repository under a `language: go` plan, which is the shape the existing plan-validate tests already use for it.
  Cover the fail-closed severity predicate's table against the gate: an unrecognized severity and a zero-value severity each fail the gate, and an informational-only set passes it while still logging a warn.
  All of it is untagged and offline, per the Test Tier Purity Invariant; the quarry-unavailable case asserts on the error path rather than requiring a resolvable fixture, which is what keeps the Quarry CGO Requirement Invariant from making this tier depend on a resolvable repository.
- **Commit:** `test(loomshed): cover both gate closures, the ParsePlan split, and the warn lines`

### Card 21: Commit-decorator tests for the new pointer rule

- **Context:**
  - `internal/loomshed/discussionwrite.go`
  - `internal/loomshed/planwrite.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/loomshed/discussionwrite_test.go`
  - `internal/loomshed/planwrite_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add, to each decorator's tests, one case per branch of the new rule: `shedengine.Done` with a pointer commits and returns `Done`, unchanged from today; `shedengine.Stuck` with a non-empty pointer commits and returns `Stuck` with that same pointer, which is the gate-failed writer row's shape; `shedengine.Stuck` with an empty pointer does not commit, which is the asking row's shape; and a commit error on the non-empty-pointer `Stuck` path maps to a returned error exactly as it already does on the `Done` path.
  The existing nil-commit-seam case must still hold and must now be reachable from the `Stuck`-with-pointer branch as well.
- **Commit:** `test(loomshed): cover commit-on-non-empty-pointer in both writer decorators`

## Batch Tests

`verify: go test ./internal/loomshed/...` runs the whole package, which is the correct scope: every card edits or creates a file in it, and the package is small enough that a narrower command would buy nothing.
The new `gates_test.go` carries the closures' coverage, including the `ParsePlan` split and the fail-closed severity table; `discussionwrite_test.go` and `planwrite_test.go` carry the decorator rule.
`gatefindings_test.go`, `discussionvalidate_test.go`, and `planvalidate_test.go` are the untouched-behaviour guards here — the two producers are still wired and must keep behaving exactly as they do today until batch 5 removes them.
No tagged test is edited by this batch.
