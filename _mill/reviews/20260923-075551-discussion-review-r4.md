MILL_REVIEW_BEGIN
# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog

```yaml
duration_s: 102.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Stale-comment grep misses hits it must catch
**Section:** Scope → In, doc updates ("The grep, not this list, is the completeness check").
**Issue:** The grep pattern `pane-liveness probe\|awaitDriverPane\|driverPaneAttempts` does not match `startLLMDriverArm`'s comment, where `start.go:63-64` wraps "pane-liveness" / "probe" across two lines.
It also misses the step-6 block comment ("probe the just-launched driver strand's pane for liveness"), the `Long` text ("pane coming alive"), and the log line "driver strand pane is live".
The list covers these, so the grep is not a complete check, even though the discussion names the grep as the authoritative one.
**Fix:** Make the list authoritative, or widen the grep (for example `pane-liveness|awaitDriverPane|driverPane(Attempts|PollInterval)|pane (is live|coming alive|for liveness)`) and drop the claim that the grep alone is complete.

### [NIT:consistency] Residual attributed to awaitDriverPane is misquoted
**Section:** Decisions → Readiness signal, Rationale.
**Issue:** `awaitDriverPane`'s doc comment names a different residual: "a provider that boots, takes the pane, and then never reads its prompt", not "binary booted into a shell that shows no TUI".
`StartupReady` does not close that residual either.
Deleting the function also deletes the only note that records it.
**Fix:** Correct the attribution, and state whether the never-reads-its-prompt residual should be carried into `AwaitStarted`'s doc comment.

### [NIT:design] `startup_timeout_s: 0` now refuses every llm bootstrap
**Section:** Decisions → Readiness signal.
**Issue:** `config.go` documents that 0 "fast-fails as died".
Under the new signal, an operator config with 0 makes every llm-arm `start` refuse on its first tick, where today it gets the 5s pane probe.
**Fix:** Say explicitly whether this is accepted, or add a floor.

## Verdict

REQUEST_CHANGES
The mechanism is sound; the stale-comment check must match its own stated inventory.
MILL_REVIEW_END
