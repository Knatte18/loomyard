MILL_REVIEW_BEGIN
# Review: Producer-agnostic final-summary artifact + wire Finalize

```yaml
duration_s: 180.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus-class model (Anthropic); exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:decision] landingshed import allowlist has no disposition
**Demoted-from:** BLOCKING
**Section:** Scope / Technical context / Testing
**Issue:** `internal/landingshed/seam_enforcement_test.go:28-40` is a positive membership allowlist that today permits `internal/websterengine`; the discussion never states whether that entry is removed and `internal/summaryparser` added. Leaving it is not a compile error, so the file silently keeps authorising the exact producer import this task exists to eliminate.
**Fix:** State in Scope that the allowlist drops `websterengine` and gains `summaryparser`, and that the dropped entry is the task's own enforcement of producer-agnosticism.

### [NIT:consistency] New invariant not scoped to production code
**Section:** Constraints — "Summaryparser Sole-Parser Invariant"
**Issue:** The wording "sole declarer of the summary artifact's filename" carries no production-only scope, unlike Lyxdirs/Discussionparser; `"summary.md"` literals already exist in `internal/websterengine/recordbatch_test.go:156`, `internal/webstercli/smoke_test.go:340`, and many `runlevel_test.go`/`integration_test.go` sites, and Testing adds no declarer scan.
**Fix:** Scope the invariant text to production files and say explicitly that no declarer-scan test is added (the leaf import test covers imports only).

### [NIT:scope] Message lands on both pair sides, not just the parent commit
**Section:** Decisions — message-set-unconditionally
**Issue:** `fabricengine/mergelifecycle.go:46-70` applies the effective message via `concludeMergeSides` to warp and weft alike, so the weft conclude commit also gains the composed title+body; the discussion frames `Message` only as "the landing commit".
**Fix:** Note in the decision that both sides of the pair inherit the composed message, and that this is accepted.

### [NIT:consistency] "matching Publish" overstates the parity
**Section:** Decisions — finalize-parse-fails-loud
**Issue:** Publish reaches its parse only when the parent is in `RequirePRToBase` and no PR exists (`publish.go:98`, `publish.go:168`); Finalize's unconditional top-of-`Call` parse is strictly stricter, making the artifact mandatory on task-to-task landings where it is never read today.
**Fix:** Restate the rationale as a deliberate tightening rather than parity with Publish.

### [NIT:decision] Told-Geometry bound-package list membership unstated
**Section:** Constraints
**Issue:** `CONSTRAINTS.md`'s Told-Geometry bound-package list includes `planparser` but not `discussionparser`; the discussion says only that `summaryparser` must not import `lyxcwd`, leaving the list edit to plan-writer judgement.
**Fix:** State whether `summaryparser` is added to that list.

## Verdict

APPROVE
Import-allowlist disposition missing; everything else checks out against source.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
