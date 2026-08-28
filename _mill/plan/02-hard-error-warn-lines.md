# Batch: hard-error-warn-lines

```yaml
task: "Audit internal/logger coverage across spawn/hard-error paths"
batch: "hard-error-warn-lines"
number: 2
cards: 3
verify: go test ./internal/shedadapters/ ./internal/mergeresolve/ ./internal/websterengine/
depends-on: [1]
```

## Batch Scope

This batch implements every `add` verdict from the audit's hard-error-return table: the three production sites whose non-`Done` shuttle-outcome branch terminates an orchestration unit and today leaves nothing in the trace file.
It is one batch because all three are the same shape — a `logger.Warn` inserted immediately before an existing error return, carrying the outcome-identifying fields the site already holds — and because two of the three are directly testable through the same captured-output seam, so an implementer holds one idiom in context rather than three.
The `mergeresolve` card additionally edits that package's import allowlist, which must land in the same commit as the import or the package does not build green.

Batches 3, 4 and 5 consume nothing from this batch; it is parallel to them under the DAG root.

Batch-local decision differing from `## Shared Decisions`: `internal/websterengine/runlevel.go`'s four `Warn` lines land untested, deliberately (card 5). Every other new line in this batch is covered.

## Cards

### Card 3: Warn on SingleLLMProducer's Died/Timeout and default outcome branches

- **Context:**
  - `internal/shedadapters/burler.go`
  - `internal/shedadapters/ctx_test.go`
  - `internal/loomshed/gatefindings_test.go`
  - `internal/logger/logger.go`
  - `internal/shuttleengine/`
- **Edits:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/singlellm_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/shedadapters/singlellm.go`, inside `(*SingleLLMProducer).mapOutcome`, add one `logger.Warn` call to each of two branches, in both cases placed AFTER the existing `cancelErr` guard and immediately BEFORE the existing `fmt.Errorf` return, so a cancelled context still returns the cancellation error without emitting a log line:

  - the `case shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout:` branch;
  - the `default:` branch.

  Each call carries exactly six fields and no others — `producer` (`p.name`), `engine` (`singleLLMEngineLabel`), `sessionID` (`result.SessionID`), `strandGUID` (`result.StrandGUID`), `runDir` (`result.RunDir`), `outcome` (`result.Outcome`).
  Do not carry `lastAssistantMessage`: on `Died`/`Timeout` the process is gone and on `default` the outcome is unrecognized, so the value is stale or absent and carrying it would invite reading it as a cause.
  Message strings are package-prefixed and distinguish the two branches — the `Died`/`Timeout` line names a died-or-timed-out shuttle run, the `default` line names an unrecognized shuttle outcome.
  `internal/shedadapters/burler.go`'s own `logger.Warn` on its `OutcomeDied, OutcomeTimeout` branch is the in-package precedent for shape and level.
  The file already imports `github.com/Knatte18/loomyard/internal/logger`; no import change is needed.

  In `internal/shedadapters/singlellm_test.go`, extend the existing `TestSingleLLMProducer_OutcomeDiedAndTimeout` (or add table cases beside it) to assert, for each of `OutcomeDied`, `OutcomeTimeout`, and one unrecognized outcome string, that the captured log output contains the level token `WARN` and each of the four result-derived field keys `sessionID`, `strandGUID`, `runDir`, `outcome`.
  Add one further case asserting that when the context is already cancelled the branch returns the cancellation error and the captured buffer stays empty — the log line must not be emitted on the cancellation path.
  Use the inline capture pattern from the `test-log-capture-pattern` shared decision (`internal/loomshed/gatefindings_test.go` is the model); `Warn` is the default threshold, so no `logger.SetVerbosity` call is needed here.
  Assert on field keys and the level token, never on an exact rendered line.
- **Commit:** `feat(shedadapters): warn on singlellm died/timeout and unrecognized outcomes`

### Card 4: Warn before mergeresolve's abortAndStuck, and admit logger to its seam allowlist

- **Context:**
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/ctx.go`
  - `internal/loomshed/gatefindings_test.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/mergeresolve/mergeresolve_test.go`
  - `internal/mergeresolve/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/mergeresolve/mergeresolve.go`, add the import `github.com/Knatte18/loomyard/internal/logger`, and inside `(*Resolver).Resolve` add one `logger.Warn` call on the `if runResult.Outcome != shuttleengine.OutcomeDone` branch, placed immediately BEFORE the existing `return r.abortAndStuck(...)` call so the diagnostic survives even when `MergeAbort` itself fails.
  The call carries the fields `outcome` (`runResult.Outcome`), `attempt` (the loop's `attempt` variable), `sessionID` (`runResult.SessionID`), and `runDir` (`runResult.RunDir`), with a package-prefixed message naming a non-done conflict-session outcome.

  In `internal/mergeresolve/seam_enforcement_test.go`, add `"github.com/Knatte18/loomyard/internal/logger": true` to the `mergeresolveAllowedImports` map, and extend that map's doc comment with one clause giving the reason: `internal/logger` carries no geometry and opens no seam, so admitting it leaves the Told-Geometry Invariant's actual property intact — the same call CONSTRAINTS.md's Treadle Runner-Seam Invariant allowlist already makes.
  This edit and the import must land in the same commit as each other; the package does not build green otherwise.

  In `internal/mergeresolve/mergeresolve_test.go`, extend `TestResolve_ShuttleOutcomes_MapToStuckNoConclude` (or add a sibling test beside it) to assert that the captured log output contains the `WARN` level token and the field keys `outcome`, `attempt`, `sessionID`, `runDir`.
  Add one further case in which `MergeAbort` itself returns an error, asserting the `Warn` line is still present in the captured buffer — that ordering is the thing the placement guarantees.
  Use the inline capture pattern from the `test-log-capture-pattern` shared decision.
- **Commit:** `feat(mergeresolve): warn on non-done conflict-session outcome before abort`

### Card 5: Warn on webster's four non-Done Master outcome branches

- **Context:**
  - `internal/websterengine/integration.go`
  - `internal/logger/logger.go`
  - `internal/shedadapters/singlellm.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/websterengine/runlevel.go`, add the import `github.com/Knatte18/loomyard/internal/logger` if it is not already present in that file, and add one `logger.Warn` call to each of the four non-`Done` branches of `Run`'s `switch result.Outcome` over the Master shuttle result, in each case placed immediately BEFORE the branch's existing `return RunResult{}, …` statement:

  - `case shuttleengine.OutcomeAsking:` — before the `&MasterAskingError{…}` return;
  - `case shuttleengine.OutcomeDied:` — before the `&MasterDiedError{…}` return;
  - `case shuttleengine.OutcomeTimeout:` — before the `&MasterTimeoutError{…}` return;
  - `default:` — before the unrecognized-outcome `fmt.Errorf` return.

  Each call carries the fields `outcome` (`result.Outcome`), `sessionID` (`result.SessionID`), and `runDir` (`result.RunDir`), with a package-prefixed message naming which branch it is.
  The `Asking` branch's line additionally carries `lastAssistantMessage` (`result.LastAssistantMessage`), matching `internal/shedadapters/singlellm.go`'s own `OutcomeAsking` line, where that field is the question being asked and is the point of the line; the other three branches must not carry it.
  Change nothing else in the switch — the returned error values, their types, and the `OutcomeDone` branch are untouched.
  These four lines land without a direct test; card 1's audit document records them as untested and why.
- **Commit:** `feat(websterengine): warn on non-done master shuttle outcomes`

## Batch Tests

`verify: go test ./internal/shedadapters/ ./internal/mergeresolve/ ./internal/websterengine/` runs the three affected packages' untagged suites.
The scope is exactly the packages this batch's `Edits:` touch — no wider — per the per-batch scoping expectation.

What each package's run covers:

- `./internal/shedadapters/` runs the extended `internal/shedadapters/singlellm_test.go` cases from card 3 (three outcome cases asserting level and field keys, plus the already-cancelled case asserting the buffer stays empty) alongside the package's existing suite, which is what catches a regression in `mapOutcome`'s untouched `Done`/`Asking` branches.
- `./internal/mergeresolve/` runs card 4's extended `TestResolve_ShuttleOutcomes_MapToStuckNoConclude` cases and, critically, `TestToldGeometryInvariant_AllowlistOnly` in `internal/mergeresolve/seam_enforcement_test.go` — the test that fails if the `logger` import lands without the allowlist entry, or the allowlist entry without the import being genuinely needed.
- `./internal/websterengine/` covers card 5 only structurally: the four new `Warn` lines have no direct test (see card 5 and the audit document's Untested log lines section), so this run proves the package still compiles and its existing suite still passes with the new import, nothing more.

Card 3 and card 4 are TDD candidates and should be implemented test-first: both branches are reachable by constructing a `shuttleengine.Result` with the outcome under test and driving the existing seam, so the assertions can be written before the log call exists and observed to fail.
Card 5 is not — it has no seam to drive.

The module-wide `verify:` in the overview (`go build ./... && GOOS=windows go build ./...`) runs at the batch boundary and catches any cross-package fallout from the two new `logger` imports.
