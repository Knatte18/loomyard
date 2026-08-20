MILL_REVIEW_BEGIN
# Review: preflight: split into two Shed rows -- a generic one, and loom's own

```yaml
duration_s: 159.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [NIT:consistency] NewPreflight signature stated two ways
**Demoted-from:** BLOCKING
**Section:** Scope (l.28), `row-1-home` (l.56), `preflightshed-takes-a-told-name` (l.213), Technical context (l.250)
**Issue:** Three sites specify a one-argument constructor (`NewPreflight(cwd string)`, "taking `cwd` and nothing else", `preflightshed.NewPreflight(cwd)`) while the `preflightshed-takes-a-told-name` decision specifies `NewPreflight(name, cwd string)` and its whole rationale rests on the name being told.
**Fix:** Pick the two-argument form (the decision's) and correct Scope l.28, `row-1-home` l.56, and the wiring line at l.250 to `preflightshed.NewPreflight(NamePreflight, cwd)`.

### [NIT:decision] Five re-exported tier-1/2 CheckID aliases lose every caller
**Demoted-from:** BLOCKING
**Section:** `delete-the-composite`, Technical context "Files that change"
**Issue:** `internal/loomengine/report.go:28-43` re-exports `CheckGeometry`/`CheckWorktreeClean`/`CheckFabricReady`/`CheckFabricSync`/`CheckJunction` explicitly "so existing callers of these names keep compiling"; after `runCheck4` is deleted, `preflight_integration_test.go` is retired, and `smoke_test.go:641` is repointed at `preflight.Check`, grep shows zero remaining users — yet the discussion states a disposition only for `CheckSeedUnreadable`'s doc (l.51-55). `report.go`'s own header comment (l.1-7) also describes the deleted `Preflight`.
**Fix:** State whether the five aliases are kept (with a stated reason, e.g. Hardener/future callers) or deleted, and add `report.go`'s header comment to the doc list.

### [NIT:scope] Fourth production comment naming the deleted symbol missed
**Demoted-from:** BLOCKING
**Section:** Technical context, "Docs that change" (l.284-286)
**Issue:** The list names three comments (`doc.go:484`, `warpclean.go:2`, `warpclean.go:17`), but `internal/fabricengine/drift.go:17` also names `loomengine.Preflight` ("letting a caller like loomengine.Preflight classify the failure"); the enumeration is presented as complete and is not.
**Fix:** Add `drift.go:17` and state that the enumeration was produced by a repo-wide grep on the symbol, so the list is closed rather than sampled.

### [NIT:consistency] The "known risk" fixture question is already answerable
**Section:** Testing, "Known risk the planner must confirm" (l.349-351)
**Issue:** `internal/loomshed/fixture_test.go:106` already calls `loomshed.Seed(...)`, so `buildSequenceFixture` writes a coherent fresh seed that a real row 2 passes; only `testDeps` writes no status file, and no test using it runs the list.
**Fix:** Replace the open risk with the verified fact, keeping only the `testDeps` half as a note.

### [NIT:scope] coherence.go's prose still names "Preflight" and "row 1"
**Section:** Technical context (l.243)
**Issue:** The disposition covers only `coherence.go:41` and `:91`'s literals, but the file header (l.1-5) and the two explanatory comments at l.38-40 and l.84-89 assert "current_producer must always name [the first producer]" and "a Stuck at row 1", both false after the split.
**Fix:** Name those comment blocks in the same file's disposition.

### [NIT:ambiguity] Tier of preflightshed's cancel-during case unstated
**Section:** Testing, TDD candidate 4 (l.334-337)
**Issue:** "Tier 1 for the contract shape ... outcome-mapping coverage is Tier 2" leaves the "cancelled during → error on the non-success path" case untiered; it needs `preflight.Check` to complete with a non-OK report, which spawns git and needs a hub, while the existing model (`resume_test.go:311-341`) covers entry-cancellation only.
**Fix:** Assign the cancel-during case explicitly to Tier 2 alongside the outcome-mapping tests.

## Verdict

APPROVE
Contradictory constructor signature, undisposed check-ID aliases, and one missed stale comment site.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
