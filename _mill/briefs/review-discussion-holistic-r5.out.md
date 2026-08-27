MILL_REVIEW_BEGIN
# Review: Add a local-only file category to weft

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: /home/knatte/Code/loomyard/wts/weft-local-only-files/_mill/discussion.md
date: 2026-08-27
```

## Findings

### [BLOCKING:consistency] MergeStateActive: weft-only vs both-sides
**Section:** `### skip-while-mid-merge` vs `## Testing` vs `## Q&A log`
**Issue:** The decision states the probe consults `MergeHeadPresent()`/`ConflictedFiles()` "on the weft alone — not the two-sided form", with a dedicated argument against inheriting `foreignMergeStatePresent`'s shape (verified: `mergestate.go:257-276` probes both sides); but the Testing bullet says it "reports true for foreign merge state on either side" and the Q&A log answer says "on both sides".
**Fix:** Rewrite the Testing bullet and the Q&A answer to the weft-only form so all three agree, since a plan writer following Testing would build the rejected two-sided probe.

### [BLOCKING:design] Push under the status write lock is unspecified
**Section:** `### commit-hook-lives-in-persist` / `### commit-and-push-every-transition`
**Issue:** `persist` is a single `return state.UpdateJSON(...)` (`shedengine/run.go:344-359`) and `state.UpdateJSON` holds `AcquireWriteLock(lockPath)` for the whole call (`state/state.go:110-136`); "called by `persist` after every successful write" does not say whether the closure fires inside the mutate callback or after `UpdateJSON` returns, and the two differ materially — a synchronous network push inside the callback blocks every `ReadJSON`/`ReadJSONStrict` reader (`lyx loom status --watch`) on `StatusLockPath` for the push's duration.
**Fix:** State explicitly that `CommitStatus` is invoked after `UpdateJSON` returns (outside the lock), and name the consequence of the resulting read-then-commit window.

### [NIT:scope] fabriccli's `weft_pulled` envelope semantics undisposed
**Section:** `## Scope` / `### pull-does-not-stall-on-weft`
**Issue:** Scope names `PartialPullError` but not `PullResult.WeftPulled`, whose doc comment asserts "Every field below is only ever populated once this is true" (`pull.go:30-37`) — false once warp pulls with the weft unpulled — nor `fabriccli/weft_verbs.go:364`'s `"weft_pulled"` envelope key, which can now report `false` inside a success envelope.
**Fix:** Add `PullResult`'s doc comment to the same-commit doc list and state whether the CLI envelope's `weft_pulled=false`-on-success is an accepted observable change.

### [NIT:decision] Per-transition commit message unspecified
**Section:** `### commit-and-push-every-transition`
**Issue:** The closure mirrors `landingdeps.go:66`'s `CommitAnchoredPaths(..., msg, ...)` but no commit-message form is decided, and this now fires once per transition rather than once per landing.
**Fix:** Name the message shape (fixed string, or one carrying producer/state) so the plan does not invent one.

## Verdict

REQUEST_CHANGES
Probe scope contradicts itself; lock-vs-push ordering for the persist hook is undecided.
MILL_REVIEW_END
