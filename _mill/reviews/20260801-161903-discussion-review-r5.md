MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] HasUnpushed baseline conflates rebased-away commits
**Section:** warp-refresh-primitives / safe-vs-unsafe-reconcile
**Issue:** The safety gate reuses the existing `hasUnpushed` (push.go:233), which computes `git rev-list --count @{u}..HEAD`; after the fetch-without-merge updates `origin/warp` to the rebased tip, a purely-pushed local branch's old pre-rebase commits are no longer ancestors of the new upstream, so `@{u}..HEAD` is non-zero and `HasUnpushed()` returns true even with zero genuine local work — making auto-reconcile abort in exactly the clean case it exists to serve.
**Fix:** Specify that the unpushed check measures HEAD against the *pre-fetch* warp remote-tracking ref (captured before the fetch primitive runs), not the post-fetch `@{u}`, so rebased-away commits do not register as local unpushed work; note `hasUnpushed`'s "no upstream ⇒ true" default is inert here only because divergence detection already established an upstream.

### [NOTE] Q&A log entry contradicts finalized weft-first ordering
**Section:** Q&A log (final entry) vs. pull-partial-failure-contract
**Issue:** The last Q&A answers "if weft's pull fails after warp already succeeded/reconciled," but the finalized ordering (`unified-pull-dispatch`, r3) is weft-first — weft cannot fail after warp succeeds; the authoritative Decision at pull-partial-failure-contract already states the correct (warp-fails-after-weft-succeeds) contract.
**Fix:** Update the stale Q&A entry to match the weft-first ordering so a plan writer reading the log isn't misled about which side is the already-completed one.

## Verdict

GAPS_FOUND
One gap: the reused unpushed-check baseline would make auto-reconcile misfire; one non-blocking log inconsistency.
MILL_REVIEW_END