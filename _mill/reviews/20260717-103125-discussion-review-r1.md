MILL_REVIEW_BEGIN
# Review: Master Builder: new, parallel fork-based implementation module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Model de-escalation leaks on recovery/no_report paths
**Section:** oversized-model-escalation + fork-failure-ladder
**Issue:** Escalation is injected by `begin-batch` (oversized) but de-escalation only by `record-batch`; the ladder's `stuck → recover-batch` and `no_report → recover-batch` branches never call `record-batch`, so an oversized batch that fails leaves Master's pane pinned at `master_oversized`, and every later non-oversized batch forks at the escalated model.
**Fix:** Assign de-escalation to the recovery paths (or make `begin-batch` idempotently set the correct model per-batch, escalate-or-de-escalate), and state a run-exit model reset.

### [NOTE] no_report vs zero-transcript precedence unspecified
**Section:** fork-audit-policy + fork-failure-ladder
**Issue:** `record-batch` both returns a distinct `no_report` classification (fork ran, wrote no file → re-fork once) and hard-errors on "zero new transcripts = batch never forked"; the precedence when a begun batch yields zero transcripts (Agent errored before running) is undefined — the two outcomes route to different ladder branches.
**Fix:** State the check order (report presence vs transcript count) and which branch a begun-but-transcript-less fork takes.

### [NOTE] Single Master-spawn timeout covers the whole sequential plan
**Section:** config-webster-yaml / run-verb-shape
**Issue:** Unlike builder's per-batch implementer timeouts, `master_timeout_min` is one shuttle timeout spanning the entire sequential run; a large plan risks a mid-run kill and there is no per-batch watchdog.
**Fix:** Note how the whole-run timeout is sized (or that it is intentionally generous) so a plan writer does not assume per-batch semantics.

### [NOTE] /model pane injection while a bracket-verb subprocess runs
**Section:** oversized-model-escalation
**Issue:** The sandbox scenario validates whether `/model` takes effect mid-turn, but not the specific mechanic that `begin-batch` (a foreground Bash tool subprocess occupying Master's pane) is running when mux sends the `/model` keys — whether those keys reach Claude's TUI input rather than the subprocess is the load-bearing uncertainty.
**Fix:** Have the validation scenario explicitly inject while a bracket-verb subprocess holds the pane, not just assert mid-turn API effect.

## Verdict

GAPS_FOUND
One unhandled state transition (model de-escalation on the failure/recovery path) must be resolved before planning.
MILL_REVIEW_END