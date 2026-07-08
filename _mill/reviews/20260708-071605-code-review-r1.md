MILL_REVIEW_BEGIN
# Review: Build burler - the review+fix round worker — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-08
```

## Findings

### [NIT] `burler run` can emit two JSON error envelopes on one invocation
**Location:** `internal/burlercli/run.go:189` (`MarkFlagRequired("profile")`) combined with `internal/burlercli/cli.go:62-112` (`PersistentPreRunE`)
**Issue:** Unlike `shuttlecli`'s `run` verb (which validates required flags manually inside `RunE`, after checking `clihelp.ShouldAbort`), `burlercli`'s `run` uses cobra's `MarkFlagRequired`, which cobra validates *after* `PersistentPreRunE` but *before* `RunE`/`ShouldAbort` is ever consulted. Running `lyx burler run` with no `--profile` outside an initialized git repo therefore writes one `output.Err` envelope from the `PersistentPreRunE` abort ("not a git repository") and a second from cobra's own required-flag error surfaced through `clihelp.RunRoot` — two JSON lines for one invocation, verified by tracing `TestRunCLI_Run_MissingProfile` (which still passes since it only substring-checks for "profile" and exit code 1, not line count).
**Fix:** Either drop `MarkFlagRequired` and validate `--profile` manually inside `RunE` behind the `ShouldAbort` guard (matching shuttlecli's `run.go` pattern exactly), or accept the double envelope as intentional and note it in the doc comment.

## Verdict

APPROVE
Implementation matches the plan and discussion precisely across all 5 batches; only one NIT-level UX quirk found.
MILL_REVIEW_END