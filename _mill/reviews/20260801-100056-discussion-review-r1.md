MILL_REVIEW_BEGIN
# Review: Audit and overhaul engine test suites

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: /home/knatte/Code/loomyard/wts/test-suite-overhaul/_mill/discussion.md
date: 2026-08-01
```

## Findings

### [NOTE] Stale docstring on the await-batch test being changed
**Section:** Scope → In (webstercli `--wait 1ns` fix) / Technical context
**Issue:** `TestAwaitBatchCmd_ReportPresenceEnvelope`'s own docstring (verbs_test.go:400-404) claims "the bounded wait (PollWaitS = 1s in this fixture) elapses", which is already false — the `wait==0` path uses `DefaultAwaitWaitS` (~30s), not 1s — and adding `--wait 1ns` makes it more misleading; the discussion does not mention correcting it.
**Fix:** Add an explicit note in Scope to update that docstring to reflect the actual window (`--wait 1ns`) in the same edit.

### [NOTE] New benchmark block collides with existing 2026-08-01 heading
**Section:** Decisions → Benchmark doc update
**Issue:** Today is 2026-08-01, so the new "newest-first" block would carry the same `### 2026-08-01` date as the existing `### 2026-08-01 — githubclient + webstercli now the floor` block, yielding two identically-dated headings.
**Fix:** Specify a distinct descriptive suffix for the new heading (e.g. `### 2026-08-01 — timeout/window seams shrunk`), mirroring the existing block's suffix convention.

## Verdict

APPROVE
Scope, decisions, and technical claims are sound and verified against source; only minor doc-accuracy NOTEs.
MILL_REVIEW_END
