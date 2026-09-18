# Review: Launch ly-supervise and orchestrator via lyx reed add

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:design] dependsOrder: sequence may not actually stop on failure
**Section:** Decision `sequenced-tasks-not-shell-operators` / `fail-loud-on-no-hub`
**Issue:** The rationale claims `dependsOrder: sequence` gives "failure-stops-the-chain semantics from VS Code itself." VS Code's task runner has historically *not* aborted a dependent-task sequence when an earlier task exits non-zero — it runs the next task regardless. If that still holds, `lyx reed add --if-absent` and `lyx reed attach` would still fire after a failed `lyx reed up`.
**Suggested fix:** No behavior change needed — `add`'s `requireSessionLocked` and `attach` both fail safely with no session, so the end state (no untracked strand, no bare `claude`) holds either way. Worth a one-line note in the plan or a quick manual check at implementation time rather than asserting the VS Code mechanism causes the abort; the actual safety comes from each downstream command's own guard, not from VS Code stopping the chain.

## Verdict

APPROVE
Every code-location, line-number, and function-behavior claim I spot-checked against the actual source was accurate; scope, constraints, decisions, and testing are all concretely grounded with only a cosmetic rationale nit.
