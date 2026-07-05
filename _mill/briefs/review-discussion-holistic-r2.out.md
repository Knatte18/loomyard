MILL_REVIEW_BEGIN
# Review: Build internal/shuttle: one LLM agent via a swappable engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-05
```

## Findings

### [GAP] AskUserQuestion steer contradicts `asking` classification
**Section:** Decisions › Guardrails; Outcome classification
**Issue:** The guardrail steers the agent to "write the question to **the output file** and end the turn — which shuttle surfaces as `asking`", but `asking` is defined as "Stop received, **output missing**"; if the agent writes an expected `OutputFiles` path, "all files exist" is true and it classifies as `done`, and the question would not ride `LastAssistantMessage`.
**Fix:** State that on AskUserQuestion-deny the agent ends its turn **without** writing `spec.OutputFiles` (question rides in `last_assistant_message`), or define a distinct question channel separate from the result files; reconcile the steer text with the missing-output rule.

### [NOTE] mux `claude:` removal ripples beyond template.yaml
**Section:** Scope › mux extensions; Config decision
**Issue:** Scope says only "Remove the unused `claude:` key from `template.yaml`", but the key is also backed by `Config.Claude` (internal/muxengine/config.go:24) and asserted in config_test.go:53-54.
**Fix:** Note that the removal must also drop the `Config.Claude` struct field and its config-test assertion, not just the template line.

### [NOTE] `KeepPane` override appears only in Q&A, not in scope/decisions
**Section:** Q&A log ("Strand at run end?") vs Run directory / classification decisions
**Issue:** A `KeepPane` spec override is mentioned once in the Q&A but is absent from the `spec` field list and the cleanup decision, leaving its existence/semantics for v1 ambiguous.
**Fix:** Either fold `KeepPane` into the spec/cleanup decision explicitly or drop the Q&A mention.

## Verdict
GAPS_FOUND
One contract-level contradiction (guardrail steer vs `asking`) must be resolved before planning.
MILL_REVIEW_END
