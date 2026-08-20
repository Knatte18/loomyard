MILL_REVIEW_BEGIN
# Review: preflight: split into two Shed rows -- a generic one, and loom's own

```yaml
duration_s: 199.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [NIT:decision] wiring_test.go's pinned type string is undisposed
**Demoted-from:** BLOCKING
**Section:** Technical context → Tests that change
**Issue:** The entry reads "check whether it names the row-1 constructor" — it does, and harder than a name: `internal/loomcli/wiring_test.go:118` compares `fmt.Sprintf("%T", c.deps.Preflight)` against the literal `"*loomshed.preflightProducer"`, which becomes a guaranteed failure once the type moves to `internal/preflightshed`.
**Fix:** State the disposition — new expected literal (`*preflightshed.preflightProducer`, or whatever the type is named there) plus the line-98-99 comment naming `loomshed.NewPreflightProducer`.

### [NIT:scope] Closed grep covers only one of the two moved symbols
**Demoted-from:** BLOCKING
**Section:** Technical context → Docs that change
**Issue:** The enumeration is declared "closed, not sampled" for `loomengine.Preflight`, but `NewPreflightProducer` also leaves `internal/loomshed` and has no equivalent sweep; `internal/loomcli/cli.go:32-33` names it in a production comment ("loomshed.NewPreflightProducer reads it -- Preflight is the one row that spawns git") and appears nowhere in the change list.
**Fix:** Extend the closed-grep statement to both moved symbols and add the `cli.go` comment to the change list.

### [NIT:consistency] Verify command contradicts itself
**Demoted-from:** BLOCKING
**Section:** Testing vs Q&A log
**Issue:** Testing pins three invocations and argues the third (`go vet -tags smoke ./internal/loomcli`) "is not optional padding", while the Q&A answer to "What is the verify command?" names only the two `go test` runs — a plan writer copying the Q&A would ship the gate that misses the smoke-tag compile break this task creates.
**Fix:** Update the Q&A answer to the three-invocation form.

### [NIT:consistency] CheckSeedMissing's stale doc clause is not disposed of
**Demoted-from:** BLOCKING
**Section:** Decisions → drop-check3blockseed / checkseedmissing-stays…
**Issue:** `report.go:48-49` reads "fails when _lyx/loom/status.json does not exist **and fabric is otherwise ready and healthy**"; removing `check3BlocksSeed` makes not-exist unconditional, so that clause goes stale — but the enumerated report.go edits ("three separate edits") only *add* the unreachable-through-Shed note to it and narrow `CheckSeedUnreadable`.
**Fix:** Name the removal of that clause alongside the `CheckSeedUnreadable` narrowing, same Documentation-Lifecycle argument.

### [NIT:design] drop-check3blockseed rationale is not true on resume
**Section:** Decisions → drop-check3blockseed; Testing → scenarios that must not regress
**Issue:** "Shed never advances to row 2" and "a broken fabric blocks at row 1 and never reaches row 2" hold only within one `Run` call; a run blocked at row 2 resumes by re-calling `current_producer: Loom-Preflight` directly (`run.go:101-111`), with row 1 never re-run. The conclusion still stands because step 1's read gate hard-errors first (`run.go:77-83`), but that is not the reason given.
**Fix:** State the step-1 read gate as the second half of the argument, and reword the regression scenario for the resume path.

## Verdict

APPROVE
Two enumeration/disposition gaps, one verify-command contradiction, one stale doc clause.
_Note: 4 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
