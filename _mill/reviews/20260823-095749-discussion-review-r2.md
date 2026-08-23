MILL_REVIEW_BEGIN
# Review: loom: self-checkable mechanical gates

```yaml
duration_s: 172.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-23
```

## Findings

### [NIT:consistency] "Forced" registration test is a superset check
**Demoted-from:** BLOCKING
**Section:** `## Technical context` → "Registration tests — only two files actually move" and `## Testing` → "Registered-verb assertions"
**Issue:** Both places claim `internal/loomcli/cli_test.go`'s `TestCommand_AllFourVerbsRegistered` "asserts the exact registered-verb set and will fail until it is extended to six" — the source (cli_test.go:35-48) builds `want := map[string]bool{"run","drive","status","pause"}`, marks only names it already knows, and asserts each of those four is present; two extra verbs pass unchanged, so nothing in the repo forces any test update for this task.
**Fix:** Correct the claim — state that zero tests are forced, that both the `cli_test.go` and `helptree_test.go` updates are deliberate coverage, and decide explicitly whether `TestCommand_AllFourVerbsRegistered` should be tightened to a real exact-set assertion (otherwise the task ships two new verbs with no mechanical registration guard at all).

### [NIT:design] Retained tests need the required-heading list they can no longer see
**Section:** `### required-sections-move-unexported` / `## Testing`
**Issue:** The rejection of exporting the list rests on "no caller", but `internal/loomshed/discussionvalidate_test.go:113` iterates `requiredDiscussionSections` today, and the retained producer test plus both parity-test fixture builders still need a decision record carrying all seven headings from outside the new package.
**Fix:** Say how those fixtures obtain a passing decision record (hardcoded fixture text, a test-only fixture helper, or an exported list) rather than leaving it implicit.

### [NIT:scope] Non-not-exist I/O case is only reachable on one of the two files
**Section:** `## Testing` → `internal/discussionparser` case list
**Issue:** "an unreadable path that is not a not-exist (e.g. a directory where a file is expected)" is stated generically, but the support log is only `os.Stat`ed — a directory there passes — so only the decision record's `os.ReadFile` can produce that case.
**Fix:** Name the decision record as the file that case applies to, and state the expected outcome when the support log path is a directory.

## Verdict

APPROVE
One source-contradicted claim about which test forces the verb registration update.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
