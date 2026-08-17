MILL_REVIEW_BEGIN
# Review: planparser owns the plan-directory path

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model; exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [NIT:decision] `planparser.Validate`'s `worktreeRoot` param undispositioned
**Section:** Technical context → Import direction; CONSTRAINTS reword
**Issue:** `internal/planparser/validate.go:56` is `func Validate(plan *Plan, worktreeRoot string)` yet is fed `layout.AnchorPath()` (`webstercli/validate.go:73`); the discussion cites this call as the "established shape … including the anchor-always argument" the new functions imitate, but never says whether the misleading parameter name stays, and the new sole-declarer bullet ("caller supplies the anchor path, `AnchorPath()`, never `WorktreePath()`") lands beside a package symbol whose own parameter says `worktreeRoot`.
**Fix:** State a disposition — rename to `anchorPath` in this commit, or explicitly leave it out of scope with a one-line reason.

### [NIT:scope] Three items left as "verify at implementation" are answerable now
**Section:** Testing → `internal/webstercli`; Technical context → import churn
**Issue:** `cli_test.go:134,152`'s layouts ("check before changing them"), `notransients_test.go`'s `loomengine` import ("verify rather than assume"), and `constructoranchoring_test.go`'s header ("check whether it needs a touch-up") are all resolvable from source today: those two layouts back only `fabricSync` tests and never consume `planDir` (no flip needed); `notransients_test.go` keeps `loomengine` for its `DiscussionDir`/`LoomStatusFile`/`LoomStatusLock` rows; and that header's "every constructor batch 5 relocated out of `internal/lyxcwd`" line becomes inexact once the plan rows name `planparser`.
**Fix:** Record the three answers as decisions rather than leaving them as implementation-time checks.

## Verdict

APPROVE
Claims verified against source; scope, decisions, and anchoring coverage are sound.
MILL_REVIEW_END
