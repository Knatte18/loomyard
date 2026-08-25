# Review: Fix prowler: Reddit adapter blocked

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [NIT:scope] Credential-provisioning step has no named owner or step, despite gating "done"
**Section:** Decisions → `oauth-credential-shape`'s "Open risk" paragraph, and Testing → "Live credentialed smoke test"
**Issue:** The document is explicit that "no Reddit app credentials exist yet" and that "the task is not done until [the live test] has been run for real against a live Reddit app and observed to pass" — but nowhere does it say who registers the Reddit "script" app and sets `PROWLER_REDDIT_CLIENT_ID`/`PROWLER_REDDIT_CLIENT_SECRET`, or at what point in the task this manual, non-automatable step happens. Every other loom-side task in this backlog names its manual operator prerequisites explicitly (e.g. plugin installs); this one only surfaces the gap as an "open risk" without assigning it a step.
**Suggested fix:** Add one line to Scope or Testing naming this as an explicit manual prerequisite the plan must schedule before the live integration test can run — who creates the Reddit app and how the two env vars reach the implementer's/CI's environment.

## Verdict

APPROVE
Problem, scope, and all eight decisions are grounded in live-probed evidence (not speculation) with concrete reproduction commands and observed results; failure modes (IP-escalation, silent-garbage-as-success, concurrent token acquisition) are identified and tested for. The one gap is a process/ownership detail, not a design ambiguity, and does not block plan writing.
