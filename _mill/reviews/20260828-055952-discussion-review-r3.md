MILL_REVIEW_BEGIN
# Review: Producer-agnostic final-summary artifact + wire Finalize

```yaml
duration_s: 161.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model; exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:consistency] Weft-conclude "accepted consequence" is false
**Demoted-from:** BLOCKING
**Section:** Decisions › message-set-unconditionally **Issue:** The discussion states the composed message "lands on *both* sides of the pair" because `concludeMergeSides` passes one `effectiveMsg` to warp and weft alike, but `merge.go:449-460` writes `WeftOutcome: mergeOutcomeAlreadyUpToDate` at the first checkpoint, and `mergelifecycle.go:75` skips the weft arm on exactly that value — `mergelifecycle.go:17-23` says so verbatim ("not because a weft conclude can still be produced"). **Fix:** Restate the consequence as "warp-side conclude commit only; the weft arm is unreachable for records this binary produces", or drop it — and make sure the accepted-consequence text does not migrate into `final-summary-spec.md` as written.

### [NIT:scope] Finalize's unconditional parse breaks existing fixtures
**Section:** Testing **Issue:** The parse at the top of `Call` plus the `NewFinalize`/`NewPublish` empty-`FinalSummaryPath` rejection makes every existing `Finalize` path require an artifact on disk; `finalize_test.go` builds `&Finalize{...}` directly for eight tests with no summary file, and `landingshed.Deps` fixtures in `internal/shedrecipe/entries_simple_test.go`, `internal/shedbuild/fixture_test.go`, and `internal/loomrecipe/fixture_test.go` construct through the rejecting constructors. **Fix:** Name those three fixture packages plus the existing `finalize_test.go` cases in Testing, so the plan carries the fixture update rather than discovering it at `go test` time.

### [NIT:scope] docs/overview.md edit left unspecified
**Section:** Scope **Issue:** "docs/overview.md ... updated" names no target; the file has both a module list (`:303-304`) needing a `summaryparser` row and a kept-specs enumeration (`:98`, `loom-status-spec.md`, `webster-spec.md`, `llm-model-spec.md`) that must gain `final-summary-spec.md`. **Fix:** Name both sites explicitly.

## Verdict

APPROVE
One decision's accepted-consequence rests on a behaviour the cited source contradicts.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
