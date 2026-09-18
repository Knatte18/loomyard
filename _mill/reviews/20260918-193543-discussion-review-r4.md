MILL_REVIEW_BEGIN
# Review: Worktree spawn/teardown as Shed producers

```yaml
duration_s: 107.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4-class model (self-assessment; exact version uncertain)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:scope] shedbuild's every-engine fixture unaccounted for
**Demoted-from:** BLOCKING
**Section:** Scope / Testing ("The registry", `internal/shedrecipe` testing)
**Issue:** `internal/shedbuild/build_engines_test.go:46` drives `TestBuild_EveryRegisteredEngineBuilds` off `shedrecipe.Names()`, and `fixture_test.go:129-145` states in its own doc comment that a new registry entry with a new required `Env` seam fails that test until `newTestEnv` covers it — the three new entries require `Slug`, `CreateWorktree`, `LoomRun`, `Teardown`, none of which that fixture fills, yet `internal/shedbuild` appears nowhere in Scope, Testing, or the Q&A log.
**Fix:** Name `internal/shedbuild`'s `newTestEnv`/`TestBuild_EveryRegisteredEngineBuilds` as an in-scope edit in the same commit, in the same way the `loomrecipe` coverage-guard collision is already named.

### [NIT:consistency] Non-prime refusal test placed in Tier 1, but it spawns git
**Demoted-from:** BLOCKING
**Section:** Testing (`internal/lifecyclecli`) vs. Constraints (Test Tier Purity)
**Issue:** The non-prime refusal is described as a Tier-1 `RunCLIIn` case, but the check itself is `fabricengine.PrimeName` → `List` → `gitexec.Run` (`internal/fabricengine/worktreelist.go:29,147-148`), and reaching it at all requires `lyxcwd.Resolve`, which runs `git rev-parse --show-toplevel` (`internal/lyxcwd/lyxcwd.go:104-113`) — both barred from untagged files by Test Tier Purity.
**Fix:** State which tier that case lives in (integration/smoke-tagged, or a seam that makes the prime comparison injectable) rather than leaving it in the Tier-1 paragraph.

### [NIT:decision] `PrimeName` error disposition unstated
**Section:** `lifecycle-driver-runs-from-prime`
**Issue:** The decision reads as `location.WorktreeName` vs. `fabricengine.PrimeName(location)`, but `PrimeName` returns `(string, error)` and fails when the worktree list cannot be read or has no main entry; no disposition is given for that third outcome on either verb.
**Fix:** Say whether a `PrimeName` error is a refusal envelope or a hard error, on both `run` and `status`.

## Verdict

APPROVE
Two blocking gaps: an unnamed shedbuild fixture collision and a mistiered refusal test.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
