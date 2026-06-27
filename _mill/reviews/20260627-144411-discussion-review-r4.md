MILL_REVIEW_BEGIN
# Review: Built-in CLI help: lyx self-documents modules & commands

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-27
```

## Findings

### [GAP] warp test omitted from regression inventory
**Section:** Testing → "Existing tests (regression)" / Per-module scenarios (warp)
**Issue:** `internal/warp/warp.go:96` emits the unknown/no-arg usage as a JSON envelope via `output.Err(out, …)`, and `internal/warp/warp_test.go:75-85` (`UnknownSubcommand`) `json.Unmarshal`s that buffer and asserts `ok=false`; under cobra the seam merges plain "unknown command" text (non-JSON) into `out`, so `decodeResult` `t.Fatalf`s — yet warp is not in the five-file edit list/budget and is asserted to "still pass."
**Fix:** Add `internal/warp/warp_test.go` to the regression-edit inventory (assert on the `unknown command` substring + exit code, not a decoded JSON envelope) and bump the assertion budget accordingly.

### [NOTE] JSON-help interception mechanism unspecified
**Section:** Decisions → json-help-form
**Issue:** The schema is fixed, but how `--json` intercepts a help path is not stated — cobra handles `--help`/no-Run help internally, so emitting JSON requires overriding `SetHelpFunc` (and handling `--help --json`) rather than a normal `RunE`.
**Fix:** Name the interception seam (e.g. a custom `HelpFunc` on the root that checks the persistent `--json` flag) so the plan writer does not rediscover it.

## Verdict
GAPS_FOUND
warp's unknown-subcommand JSON test breaks under cobra but is absent from the regression inventory.
MILL_REVIEW_END