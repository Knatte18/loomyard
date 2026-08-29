MILL_REVIEW_BEGIN
# Review: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5
reviewed_file: plan/
date: 2026-08-29
```

## Findings

### [NIT:scope] Usage-error ">2 args" branch untested
**Location:** batch 1, card 4 (Test 16); batch 2, card 5 script Requirements **Issue:** the script's argument-count check rejects both "<1" and ">2" args, but Test 16 only exercises the 0-arg case (and a 1-arg regex failure) — no test invokes with 3+ args. **Fix:** add a 21st assertion (or extend Test 16) invoking with three arguments and asserting exit 2 and byte-empty stdout.

### [NIT:scope] Stub's "unsupported invocation" rejection path never directly exercised
**Location:** batch 1, card 1 (exit-98 branch); batch 1, card 4 (20 tests) **Issue:** card 1 specifies the stub must reject any non-`api <endpoint> --jq <expr>` shape with exit 98, framed as the regression guard against a re-added preflight call, but none of the 20 harness tests invoke the stub with a malformed shape to confirm this branch itself works — the existing "exactly one call log line" assertions catch a regression by count alone, not by exercising this mechanism. **Fix:** add one direct-invocation assertion of the stub (bypassing github-tree.sh) confirming exit 98 and the `unsupported invocation` message on a bad shape.

## Verdict

APPROVE
Plan is internally consistent, decisions are faithfully implemented, and claims verified against source files hold; only minor coverage nits remain.
MILL_REVIEW_END
