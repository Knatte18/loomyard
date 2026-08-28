MILL_REVIEW_BEGIN
# Review: Producer-agnostic final-summary artifact + wire Finalize — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (per harness-provided identity; no independent way to verify further)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:consistency] Batch-2 verify in 00-overview.md drops the integration-tagged vet pass
**Location:** 00-overview.md `batches:` block, batch 2 vs 02-retarget-callers.md's own header/Batch Tests
**Issue:** The overview's `batches:` yaml (stated as the "authoritative DAG mill-go reads to schedule batches") gives batch 2's verify as `go vet ./... && go test ...`, omitting `go vet -tags integration ./...` that 02-retarget-callers.md's own header carries and whose own Batch Tests section says exists specifically to catch a missed caller in `publish_integration_test.go`/`integration_test.go`, both edited by this batch.
**Fix:** Add `go vet -tags integration ./...` to batch 2's verify line in 00-overview.md so the two mirrored copies match.

### [BLOCKING:design] Card 12 says "Remove" the roadmap item; the file's own convention says "Move to Done"
**Location:** 04-docs-and-specs.md, Card 12
**Issue:** Requirements instruct "Remove the ... entry from ... `## Planned` section," but `manifest/roadmap.md`'s own Maintenance note (which the card defers to) states a shipped item is moved to `## Done` with a link to its module doc, never merely deleted. Card 12's Context conspicuously includes `contracts/specs/final-summary-spec.md` — the natural Done-entry link target — yet Requirements never instructs adding a Done entry, so a literal reading drops the shipped feature from the roadmap's own history.
**Fix:** State explicitly that the entry moves to `## Done`, linking `contracts/specs/final-summary-spec.md` (or the relevant package doc), not merely removed from Planned.

### [BLOCKING:consistency] Card 10 fixes one Finalize/Publish mislabel in webster-spec.md but leaves an identical one two lines above
**Location:** 04-docs-and-specs.md, Card 10; source `contracts/specs/webster-spec.md`
**Issue:** Card 10 correctly flags and fixes "because Finalize dumps `summary.md` verbatim into the PR body" (should be Publish). The retained sentence immediately above it — "It is Finalize's PR-text source, because a long-lived Master session is the only party with full oversight of what actually shipped" — carries the identical Finalize/Publish mislabel, and Requirements only instruct keeping the Master-oversight clause, never flagging the "Finalize's PR-text source" phrase for correction.
**Fix:** Extend Card 10's Requirements to also correct "It is Finalize's PR-text source" (Publish's, per the new `final-summary-spec.md`) when retaining the Master-oversight sentence.

### [BLOCKING:scope] docs/overview.md's "Other docs" entry for webster-spec.md goes stale, untouched by Card 11
**Location:** 04-docs-and-specs.md, Card 11; source `docs/overview.md` line ~445 ("Other docs" section)
**Issue:** That entry describes `webster-spec.md` as covering "the `_lyx/webster/` boundary, `outcome.yaml`, and the `summary.md` artifact Finalize consumes." After Card 10 shrinks that section to a pointer at `final-summary-spec.md`, this description is stale (webster-spec.md no longer directly describes the artifact, and the entry never names Publish). Card 11's Requirements name only two edit sites in `docs/overview.md` (the Documentation-lifecycle enumeration and the module list); this third stale site is never mentioned.
**Fix:** Add a third Card 11 edit site updating the "Other docs" webster-spec.md entry to reflect the pointer and both consumers, or add a matching `final-summary-spec.md` entry there.

### [NIT:consistency] Card 6 names the wrong RecordBatch deps type
**Location:** 02-retarget-callers.md, Card 6
**Issue:** Requirements twice name `websterengine.RecordBatchDeps.SummaryPath`; the actual type (`internal/websterengine/recordbatch.go:46`) is `RecordDeps`, not `RecordBatchDeps`. The same wrong name appears in `_mill/discussion.md`. `recordbatch.go` is in Card 6's own Context, so an implementer will likely self-correct, but the plan text itself is wrong.
**Fix:** Correct both occurrences to `websterengine.RecordDeps`.

### [NIT:scope] Card 6 undercounts the `wantPath` assertion sites in webster_test.go
**Location:** 02-retarget-callers.md, Card 6; source `internal/shedadapters/webster_test.go`
**Issue:** Requirements describe "the `wantPath := websterengine.SummaryPath(dir)` assertion" (singular); it actually occurs twice — `TestWebsterProducer_OutcomeDone` (line 51) and `TestWebsterProducer_CancelledDuringRun_OutcomeDoneStillSucceeds` (line 232). `go vet` will force both to compile-fix since `SummaryPath` is deleted, so this is self-correcting but the plan text undercounts the edit.
**Fix:** Say "each occurrence" rather than "the ... assertion."

### [NIT:consistency] Card 5's claim that the "fourteen fields"/"fifteenth field" comments "stay accurate" is false today
**Location:** 02-retarget-callers.md, Card 5; sources `internal/shedrecipe/recipe.go`, `internal/loomcli/landingdeps_test.go`, `internal/landingshed/deps.go`
**Issue:** `landingshed.Deps` already has 15 fields today (verified by counting `deps.go`), not 14 — `recipe.go`'s comment ("fourteen fields") and `landingdeps_test.go`'s comment ("a fifteenth field added later") are both already stale, independent of this task. Card 5's claim "both stay accurate" leaving them untouched is a false premise, even though the field-count-unchanged fact itself is true.
**Fix:** Note the pre-existing drift rather than asserting the comments are accurate; leaving them unfixed is acceptable as out-of-scope, but the rationale should not claim they're correct.

## Verdict

REQUEST_CHANGES
Four blocking consistency/scope/design gaps across the DAG verify mirror, the roadmap convention, and doc staleness need fixing.
MILL_REVIEW_END
