Now I have read all the files. Let me produce the review.

MILL_REVIEW_BEGIN
# Review: CLI help & error ergonomics from sandbox run — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-28
```

## Findings

### [NIT] muxpoc unknown-flag test missing JSON envelope assertion
**Location:** `C:\Code\loomyard\wts\cli-help-ergonomics\internal\muxpoc\cli_test.go:58-67`
**Issue:** `TestRunCLIUnknownFlagFails` checks for the "unknown flag" substring but does not assert that the output parses as a valid JSON envelope with `ok:false`; card 10 requires adding that assertion to every per-module unknown-command/flag test.
**Fix:** Add a `json.Unmarshal` + `ok == false` check after the string-contains assertion, matching the pattern used in `TestRunCLIUnknownSubcommandFails` in the same file.

### [NIT] Inaccurate ordering comment in weft unknown-subcommand test
**Location:** `C:\Code\loomyard\wts\cli-help-ergonomics\internal\weft\cli_test.go:39-41`
**Issue:** The comment "GroupRunE fires before PersistentPreRunE reaches layout resolution" has the cobra execution order backwards — `PersistentPreRunE` runs before `RunE`; the guard (not ordering) is why resolution is skipped.
**Fix:** Correct the comment to state that the `PersistentPreRunE` guard (`cmd.Name() == "weft"`) returns nil early, preventing resolution before GroupRunE runs.

## Verdict

APPROVE
All 22 cards are correctly implemented; two test-quality NITs only.
MILL_REVIEW_END
