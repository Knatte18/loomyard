MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion

```yaml
duration_s: 95.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, as invoked
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:consistency] no-origin exit code differs cleanup vs remove
**Demoted-from:** BLOCKING
**Section:** `### no-origin-is-resolved-once-per-verb` (:177, :185) vs `### remote-failure-is-a-cli-failure` (:99)
**Issue:** The no-origin pre-check writes the reason into `RemoveResult.RemoteBranchError`, and `runRemoveWithFlag` is specified to take `errWithRecordFields` "whenever `RemoteBranchError` is non-empty" — so the identical configuration state exits 0 on `cleanup` (:185 explicitly exempts it) and non-zero on `remove`, which the discussion never states or justifies.
**Fix:** State the intended `remove` exit code for a missing `origin` and pick a carrier for its reason that does not collide with the per-entry failure key the CLI verdict switches on.

### [NIT:consistency] contradictory instruction on editing CONSTRAINTS.md
**Demoted-from:** BLOCKING
**Section:** `## Scope` (:32) vs `## Constraints` (:239)
**Issue:** Scope says the Fabric Destruction Chokepoint Invariant "enumerates no primitives and needs no count change… leave it untouched"; the Constraints section says "the invariant's own wording in `CONSTRAINTS.md` must move in the same commit". The shipped invariant text names no executors and no primitive count, so the Constraints-section statement is false.
**Fix:** Delete or correct :239 so exactly one disposition for `CONSTRAINTS.md` survives.

### [BLOCKING:design] remote gate's dirtiness check is vacuous as sequenced
**Section:** `### gate-runs-unchanged-for-remote` (:133-137) vs `### local-first-then-remote` (:78) and Technical context (:199)
**Issue:** `checkBranchDirtiness` refuses only when `listWeftBranches` reports the branch checked out at a worktree; because the remote executor runs only after `git branch -D` succeeded, the branch is already absent locally, so the probe can never refuse at either real call site — the stated rationale ("a branch checked out locally must not have its remote copy deleted") is delivered by local-first ordering, not by this check. The Testing bullet "a branch the ownership predicate rejects is refused before any push is attempted" (:280) is likewise unreachable from the real call sites.
**Fix:** Say plainly that the reused gate is a defence-in-depth/shape check at the executor and that protection at the call sites comes from local-first, and mark the corresponding gate tests as direct-call tests rather than end-to-end guarantees.

### [NIT:decision] carrier for the once-per-verb no-origin reason left open
**Section:** `### no-origin-is-resolved-once-per-verb` (:184)
**Issue:** Whether the reason becomes a new `CleanupResult`/`RemoveResult` field or rides the verb-level error path is explicitly deferred, and it changes a JSON envelope the Mutation Record Invariant constrains.
**Fix:** Acceptable to defer given the two stated guardrails, but naming the preferred carrier would remove a plan-time fork.

## Verdict

REQUEST_CHANGES
Two internal contradictions and one false premise in the gate rationale need settling.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
