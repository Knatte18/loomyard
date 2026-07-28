MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewed_file: plan/
date: 2026-07-28
```

## Findings

### [BLOCKING] Token cache path has no test-isolation seam
**Location:** batch 6 / card 26 (and card 30)
**Issue:** Card 26 places the cache at `%LOCALAPPDATA%\lyx\credentials.json` / `$XDG_CONFIG_HOME`/`$HOME` with no stated override mechanism, yet card 30 requires the tests to be "fully hermetic" and to "never touch the operator's real cache." Card 25's `gh auth token` shell-out gets an explicit function-variable seam for exactly this reason ("so a test can inject a hanging fake"); card 26 states no analogous mechanism for the cache file path, so as written an implementer could satisfy card 26 literally and still have card 30's tests read/write the operator's real credential cache.
**Fix:** State in card 26 (or card 30) that the cache-directory resolution reads the standard env vars so tests redirect it via `t.Setenv` to a temp dir (or add an explicit override seam), matching the rigor already applied to the shell-out seam in the same batch.

### [NIT] hasUnpushed "three new states" miscounts against the existing spike
**Location:** batch 2 / card 8
**Issue:** Card 8 says hasUnpushed's five listed states are "three of which the spike never covered." `internal/gitnativepoc/read_test.go`'s existing `TestHasUnpushed` already has subtests covering 3 of the 5 (`AheadOfUpstream`, `Behind` = strictly-behind, `NoUpstreamConfigured`); only "configured but never fetched" and the failure-swallowing path are genuinely new relative to the spike.
**Fix:** Reword to "two of which the spike never covered" (or clarify the claim means "never compared against a live CLI oracle," if that's the intent). No implementation impact — all five states are still explicitly named for card 8 to add.

### [NIT] ACL requirement lacks API specificity and in-repo precedent
**Location:** batch 6 / card 26
**Issue:** "Use `golang.org/x/sys` for the ACL work" names the package but no specific API, and Context omits `internal/fslink/fslink_windows.go` — the repo's only existing example of a direct Windows syscall via `golang.org/x/sys/windows` (junction creation via reparse points). Without it in Context, the implementer has no in-repo model to follow for this fiddlier, security-relevant piece.
**Fix:** Add `internal/fslink/fslink_windows.go` to card 26's Context, or name the target ACL API surface (e.g. `SetNamedSecurityInfo`/`SetKernelObjectSecurity`) directly.

## Verdict

REQUEST_CHANGES
Exceptionally well evidence-grounded plan (both probe reports and every cross-checked file confirm the plan's claims); close the token-cache test-hermeticity gap before implementation.
MILL_REVIEW_END
